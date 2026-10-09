// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cilium/ebpf"
	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	endpointManager  = "unifabric-agent"
	endpointResource = "rdmaendpoints"

	// hostNetnsPath is the host network namespace, reachable through hostPID.
	hostNetnsPath = "/proc/1/ns/net"
	// hostSysfsPath is the host's own sysfs mount. Its namespace tag makes
	// every RDMA device visible under class/infiniband, and without hostPID
	// it degrades to the container's own /sys.
	hostSysfsPath = "/proc/1/root/sys"

	// endpointResend bounds how long an unchanged object goes without a
	// rewrite, so readers can tell a live agent from a stale object.
	endpointResend = 5 * time.Minute
	// endpointStale is the age after which readers ignore an object.
	endpointStale = 15 * time.Minute
)

// portInfo is what the sysfs scan learns about one port of an RDMA device.
type portInfo struct {
	num          int
	linkLayer    v1beta1.LinkLayer
	lid          uint16
	subnetPrefix string
	gids         []v1beta1.RDMAGid
}

// deviceInfo is what the sysfs scan learns about one RDMA device.
type deviceInfo struct {
	netdev string
	ports  []portInfo
}

// endpointPublisher builds one RDMAEndpoint per Pod that uses RDMA on
// this node and applies it through the API. Objects are named after the
// Pod and owned by it, so Kubernetes garbage collects them with the Pod.
type endpointPublisher struct {
	ctx    context.Context
	client client.Client
	node   string
	sysfs  string

	// minInterval spaces event driven runs, see delayUntilAllowed.
	minInterval time.Duration
	// allPods includes Pods with their own network namespace. Their GIDs
	// resolve through the Pod IP index, so by default only hostNetwork
	// Pods and Pods not yet in the index are published.
	allPods bool

	// published remembers the fingerprint and last write of every object
	// this agent owns, keyed by Pod UID.
	published map[string]publishedObject
	lastRun   time.Time
	// synced is set once the objects left by a previous agent instance have
	// been reconciled against the live Pods.
	synced bool
}

type publishedObject struct {
	namespace   string
	name        string
	fingerprint string
	sentAt      time.Time
}

// newEndpointPublisher returns nil when publishing is not possible, which
// is logged once so the rest of the agent keeps working.
func newEndpointPublisher(res *resolver, node string, minInterval time.Duration, allPods bool) *endpointPublisher {
	if res.client == nil {
		logger.Warn("rdma_endpoint", "status", "disabled", "reason", "kubernetes_api_unavailable")
		return nil
	}
	if node == "" {
		logger.Warn("rdma_endpoint", "status", "disabled", "reason", "NODE_NAME_env_missing")
		return nil
	}
	logger.Debug("rdma_endpoint_enabled",
		"node", node, "sysfs", hostSysfsPath, "all_pods", allPods,
		"resource", fmt.Sprintf("%s.%s", endpointResource, v1beta1.GroupVersion.Group))
	return &endpointPublisher{
		ctx:         res.ctx,
		client:      res.client,
		node:        node,
		sysfs:       hostSysfsPath,
		minInterval: minInterval,
		allPods:     allPods,
		published:   map[string]publishedObject{},
	}
}

// delayUntilAllowed returns how long an event driven run must wait so that
// runs stay at least minInterval apart, or 0 when it may run now.
func (n *endpointPublisher) delayUntilAllowed(now time.Time) time.Duration {
	if n == nil || n.lastRun.IsZero() {
		return 0
	}
	remaining := n.minInterval - now.Sub(n.lastRun)
	if remaining <= 0 {
		return 0
	}
	return remaining
}

// hostView runs fn in the host network namespace, falling back to the
// agent's own namespace when switching is not possible.
func hostView(fn func()) {
	err := inHostNetns(fn)
	if err != nil {
		logDedup("host_netns_error", slog.LevelWarn, "enter_host_netns", "path", hostNetnsPath, "status", "using_own_netns",

			// run collects the RDMAEndpoints of this node, applies the ones whose
			// content changed or whose resend period elapsed, and deletes the ones
			// whose Pod disappeared without waiting for garbage collection.
			"reason", err)
		fn()
	}
}

func (n *endpointPublisher) run(res *resolver, infos *ebpf.Map) {
	if n == nil {
		return
	}
	n.lastRun = time.Now()
	if !n.synced {
		n.adoptExisting()
	}
	objects := collectEndpoints(n.sysfs, n.node, res, podQPs(res, infos), n.allPods)
	logDedup("rdma_endpoint_collected", slog.LevelDebug, "rdma_endpoint_collected",
		"sysfs", n.sysfs, "object_count", len(objects), "qp_count", qpCount(objects))
	live := map[string]bool{}
	for _, object := range objects {
		uid := podUIDOf(object)
		live[uid] = true
		if object.Namespace == "" || object.Name == "" {
			// The Pod is not in the index yet, so the object cannot be named
			// or owned. The next run after the index refresh publishes it.
			logDedup("rdma_endpoint_unnamed:"+uid, slog.LevelDebug, "rdma_endpoint_deferred",
				"pod_uid", uid, "reason", "pod_not_in_index")
			continue
		}
		fingerprint := endpointFingerprint(object)
		previous, known := n.published[uid]
		unchanged := known && previous.fingerprint == fingerprint
		if unchanged && time.Since(previous.sentAt) < endpointResend {
			continue
		}
		err := n.apply(object)
		if err != nil {
			logDedup("rdma_endpoint_apply_error:"+uid, slog.LevelError, "apply_rdma_endpoint",
				"object", fmt.Sprintf("%s/%s", object.Namespace, object.Name), "reason", err)
			continue
		}
		n.published[uid] = publishedObject{
			namespace: object.Namespace, name: object.Name,
			fingerprint: fingerprint, sentAt: time.Now(),
		}
		if !unchanged {
			logger.Info("rdma_endpoint_published",
				"object", fmt.Sprintf("%s/%s", object.Namespace, object.Name),
				"interface_count", len(object.Status.Interfaces),
				"qp_count", qpCount([]v1beta1.RDMAEndpoint{object}))
		}
	}
	for uid, previous := range n.published {
		if live[uid] {
			continue
		}
		err := n.delete(previous.namespace, previous.name)
		if err != nil {
			logDedup("rdma_endpoint_delete_error:"+uid, slog.LevelError, "delete_rdma_endpoint",
				"object", fmt.Sprintf("%s/%s", previous.namespace, previous.name), "reason", err)
			continue
		}
		delete(n.published, uid)
		logger.Info("rdma_endpoint_deleted", "object", fmt.Sprintf("%s/%s", previous.namespace, previous.name), "reason",

			// podUIDOf returns the Pod UID an object describes, taken from its owner
			// reference. Unnamed objects carry the UID in the same place.
			"no_rdma_holder")
	}
}

func podUIDOf(object v1beta1.RDMAEndpoint) string {
	for _, owner := range object.OwnerReferences {
		if owner.Kind == "Pod" {
			return string(owner.UID)
		}
	}
	return ""
}

// adoptExisting lists the objects a previous agent instance left for this
// node so they are refreshed or deleted instead of leaking. Failure is not
// fatal, the next run retries.
func (n *endpointPublisher) adoptExisting() {
	items, err := n.list(map[string]string{v1beta1.RDMAEndpointLabelNode: n.node})
	if err != nil {
		logDedup("rdma_endpoint_adopt_error", slog.LevelWarn, "list_own_rdma_endpoints", "reason", err)
		return
	}
	for _, item := range items {
		uid := podUIDOf(item)
		if uid == "" {
			continue
		}
		_, known := n.published[uid]
		if !known {
			// An empty fingerprint forces a rewrite on the first run.
			n.published[uid] = publishedObject{namespace: item.Namespace, name: item.Name}
		}
	}
	n.synced = true
	if len(items) > 0 {
		logger.Info("rdma_endpoint_adopted", "count", len(items), "node", n.node)
	}
}

func (n *endpointPublisher) list(labels map[string]string) ([]v1beta1.RDMAEndpoint, error) {
	return listEndpoints(n.ctx, n.client, labels)
}

// listEndpoints lists RDMAEndpoints across all namespaces through the
// manager cache, optionally filtered by labels.
func listEndpoints(ctx context.Context, c client.Client, labels map[string]string) ([]v1beta1.RDMAEndpoint, error) {
	var out v1beta1.RDMAEndpointList
	opts := []client.ListOption{}
	if len(labels) > 0 {
		opts = append(opts, client.MatchingLabels(labels))
	}
	if err := c.List(ctx, &out, opts...); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func qpCount(objects []v1beta1.RDMAEndpoint) int {
	total := 0
	for _, object := range objects {
		for _, iface := range object.Status.Interfaces {
			for _, port := range iface.Ports {
				total += len(port.QueuePairs)
			}
		}
	}
	return total
}

// apply uses server-side apply so a single request creates or updates the
// object. Because the CRD has a status subresource, metadata and status are
// applied in two requests: the main resource carries labels and the owner
// reference, the status subresource carries the observed state.
func (n *endpointPublisher) apply(object v1beta1.RDMAEndpoint) error {
	metaOnly := &v1beta1.RDMAEndpoint{TypeMeta: object.TypeMeta, ObjectMeta: object.ObjectMeta}
	err := n.client.Patch(n.ctx, metaOnly, client.Apply,
		client.FieldOwner(endpointManager), client.ForceOwnership)
	if err != nil {
		return fmt.Errorf("apply metadata: %w", err)
	}
	full := object.DeepCopy()
	err = n.client.Status().Patch(n.ctx, full, client.Apply,
		client.FieldOwner(endpointManager), client.ForceOwnership)
	if err != nil {
		return fmt.Errorf("apply status: %w", err)
	}
	return nil
}

// delete removes one object. A missing object is not an error.
func (n *endpointPublisher) delete(namespace, name string) error {
	object := &v1beta1.RDMAEndpoint{}
	object.Namespace = namespace
	object.Name = name
	err := n.client.Delete(n.ctx, object)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// endpointFingerprint identifies the object content without its timestamp.
func endpointFingerprint(object v1beta1.RDMAEndpoint) string {
	object.Status.LastUpdateTime = metav1.Time{}
	data, _ := json.Marshal(object)
	return string(data)
}

// qpRecord is one QP of a Pod as read from qp_infos.
type qpRecord struct {
	qpn  uint32
	pid  uint32
	port int
}

// podQPs groups the QPs in qp_infos by owning Pod UID and device.
// Entries without a device name cannot be matched by peers and are skipped.
func podQPs(res *resolver, infos *ebpf.Map) map[string]map[string][]qpRecord {
	result := map[string]map[string][]qpRecord{}
	if infos == nil {
		return result
	}
	var key qpKey
	var info qpInfo
	it := infos.Iterate()
	for it.Next(&key, &info) {
		device := info.device()
		if device == "" {
			continue
		}
		uid := res.podUIDForTgid(key.Tgid)
		if uid == "" {
			continue
		}
		if result[uid] == nil {
			result[uid] = map[string][]qpRecord{}
		}
		result[uid][device] = append(result[uid][device],
			qpRecord{qpn: key.QPN, pid: key.Tgid, port: int(info.PortNum)})
	}
	err := it.Err()
	if err != nil {
		logDedup("rdma_endpoint_qp_iterate_error", slog.LevelWarn, "iterate_qp_infos",
			"purpose", "rdma_endpoint", "reason", err)
	}
	return result
}

// collectEndpoints builds one object per Pod that holds RDMA devices or
// has QPs in qp_infos. Unless allPods is set, Pods known to have their own
// network namespace are left out because peers resolve them through the
// Pod IP index. Objects for Pods missing from the index carry only the
// owner UID and are published once the index knows the Pod.
func collectEndpoints(sysfs, node string, res *resolver,
	qps map[string]map[string][]qpRecord, allPods bool) []v1beta1.RDMAEndpoint {
	var hostDevices map[string]deviceInfo
	hostView(func() {
		hostDevices = deviceTable(sysfs)
	})
	now := metav1.NewTime(time.Now().UTC())

	holders := podDeviceHolders()
	for uid, byDevice := range qps {
		// A QP proves the device is in use even if the fd scan raced with it.
		holder := holders[uid]
		if holder == nil {
			holder = &podHolder{devices: map[string]bool{}}
			holders[uid] = holder
		}
		for device := range byDevice {
			holder.devices[device] = true
		}
	}
	objects := make([]v1beta1.RDMAEndpoint, 0, len(holders))
	for uid, holder := range holders {
		p, known := res.byUID[uid]
		if known && !allPods && !p.hostNetwork {
			continue
		}
		controller := true
		object := v1beta1.RDMAEndpoint{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1beta1.GroupVersion.String(),
				Kind:       "RDMAEndpoint",
			},
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "unifabric-agent",
					v1beta1.RDMAEndpointLabelNode:  node,
				},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion:         "v1",
					Kind:               "Pod",
					UID:                types.UID(uid),
					Controller:         &controller,
					BlockOwnerDeletion: &controller,
				}},
			},
			Status: v1beta1.RDMAEndpointStatus{
				NodeName:       node,
				LastUpdateTime: now,
				Interfaces:     []v1beta1.RDMAInterface{},
			},
		}
		if known {
			object.Namespace = p.Namespace
			object.Name = p.Name
			object.OwnerReferences[0].Name = p.Name
			object.Status.HostNetwork = p.hostNetwork
		}
		// A Pod with its own network namespace sees its exclusively assigned
		// VFs only through its own sysfs, so read the table from inside it
		// and fall back to the host view for devices it does not list.
		podDevices := map[string]deviceInfo{}
		if holder.pid != "" && (!known || !p.hostNetwork) {
			podDevices = podDeviceTable(holder.pid)
		}
		for _, device := range sortedKeys(holder.devices) {
			info, ok := podDevices[device]
			if !ok {
				info = hostDevices[device]
			}
			object.Status.Interfaces = append(object.Status.Interfaces,
				buildInterface(device, info, qps[uid][device]))
		}
		objects = append(objects, object)
	}
	sort.Slice(objects, func(i, j int) bool {
		return podUIDOf(objects[i]) < podUIDOf(objects[j])
	})
	return objects
}

// buildInterface converts a scanned device and the Pod's QPs on it into the
// API shape. QPs are attached to the port they were created on. A QP whose
// port is unknown goes to the first port.
func buildInterface(device string, info deviceInfo, qps []qpRecord) v1beta1.RDMAInterface {
	iface := v1beta1.RDMAInterface{Name: info.netdev, RdmaDevice: device}
	byPort := map[int][]v1beta1.RDMAQueuePair{}
	for _, qp := range qps {
		byPort[qp.port] = append(byPort[qp.port], v1beta1.RDMAQueuePair{Qpn: qp.qpn, Pid: qp.pid})
	}
	for _, port := range info.ports {
		queuePairs := byPort[port.num]
		if port.num == firstPortNum(info.ports) {
			queuePairs = append(queuePairs, byPort[0]...)
		}
		sort.Slice(queuePairs, func(i, j int) bool {
			return queuePairs[i].Qpn < queuePairs[j].Qpn
		})
		iface.Ports = append(iface.Ports, v1beta1.RDMAPort{
			Port:         port.num,
			LinkLayer:    port.linkLayer,
			Lid:          port.lid,
			SubnetPrefix: port.subnetPrefix,
			Gids:         port.gids,
			QueuePairs:   queuePairs,
		})
	}
	// A device the Pod uses but whose sysfs table was unreadable still keeps
	// its QPs so peers can match dest_qp_num against them.
	if len(info.ports) == 0 && len(qps) > 0 {
		var queuePairs []v1beta1.RDMAQueuePair
		for _, list := range byPort {
			queuePairs = append(queuePairs, list...)
		}
		sort.Slice(queuePairs, func(i, j int) bool {
			return queuePairs[i].Qpn < queuePairs[j].Qpn
		})
		iface.Ports = append(iface.Ports, v1beta1.RDMAPort{
			Port:       1,
			LinkLayer:  v1beta1.LinkLayerEthernet,
			QueuePairs: queuePairs,
		})
	}
	return iface
}

func firstPortNum(ports []portInfo) int {
	if len(ports) == 0 {
		return 0
	}
	return ports[0].num
}

// lidKey scopes a LID to its subnet, or returns "" for ports without a LID.
func lidKey(subnetPrefix string, lid uint16) string {
	if lid == 0 || subnetPrefix == "" {
		return ""
	}
	return subnetPrefix + "/" + strconv.FormatUint(uint64(lid), 10)
}

// podDeviceTable reads the RDMA device table as one Pod process sees it,
// switching into the process network namespace so filtered GID tables are
// readable. Failures fall back to an empty table.
func podDeviceTable(pid string) map[string]deviceInfo {
	table := map[string]deviceInfo{}
	err := inNetnsOf(pid, func() {
		table = deviceTable(filepath.Join("/proc", pid, "root/sys"))
	})
	if err != nil {
		logDedup("pod_netns_error:"+pid, slog.LevelDebug, "enter_pod_netns", "process", pid, "status", "using_host_table",

			// deviceTable reads every RDMA device under sysfs with its netdev and, per
			// port, link layer, LID, subnet prefix and non zero GIDs with their index.
			"reason", err)
	}
	return table
}

func deviceTable(sysfs string) map[string]deviceInfo {
	devices := map[string]deviceInfo{}
	root := filepath.Join(sysfs, "class/infiniband")
	entries, err := os.ReadDir(root)
	if err != nil {
		logDedup("rdma_endpoint_sysfs_error:"+sysfs, slog.LevelWarn, "read_rdma_devices", "path", root, "reason", err)
		return devices
	}
	for _, entry := range entries {
		name := entry.Name()
		deviceDir := filepath.Join(root, name)
		portsDir := filepath.Join(deviceDir, "ports")
		ports, err := os.ReadDir(portsDir)
		if err != nil {
			logDedup("rdma_endpoint_device:"+name, slog.LevelWarn, "rdma_endpoint_device_unreadable",
				"device", name, "path", portsDir, "reason", err)
			devices[name] = deviceInfo{netdev: deviceNetdev(deviceDir)}
			continue
		}
		info := deviceInfo{netdev: deviceNetdev(deviceDir)}
		gidCount := 0
		entriesRead := 0
		var firstErr error
		for _, port := range ports {
			portNum, err := strconv.Atoi(port.Name())
			if err != nil {
				continue
			}
			portDir := filepath.Join(portsDir, port.Name())
			pi := portInfo{
				num:       portNum,
				linkLayer: readLinkLayer(filepath.Join(portDir, "link_layer")),
				lid:       readSysfsLID(filepath.Join(portDir, "lid")),
			}
			gidDir := filepath.Join(portDir, "gids")
			files, err := os.ReadDir(gidDir)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				info.ports = append(info.ports, pi)
				continue
			}
			for _, file := range files {
				index, err := strconv.Atoi(file.Name())
				if err != nil {
					continue
				}
				raw, err := os.ReadFile(filepath.Join(gidDir, file.Name()))
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
					continue
				}
				entriesRead++
				text := strings.TrimSpace(string(raw))
				if index == 0 {
					pi.subnetPrefix = gidSubnetPrefix(text)
				}
				if parseSysfsGID(text) != "" {
					pi.gids = append(pi.gids, v1beta1.RDMAGid{Index: index, Gid: text})
				}
			}
			sort.Slice(pi.gids, func(i, j int) bool {
				return pi.gids[i].Index < pi.gids[j].Index
			})
			gidCount += len(pi.gids)
			info.ports = append(info.ports, pi)
		}
		sort.Slice(info.ports, func(i, j int) bool {
			return info.ports[i].num < info.ports[j].num
		})
		devices[name] = info
		if gidCount == 0 {
			logDedup("rdma_endpoint_device:"+name, slog.LevelWarn, "rdma_endpoint_device_without_gids",
				"device", name, "path", portsDir, "port_count", len(ports), "entries_read", entriesRead,
				"first_error", firstErr)
		}
	}
	return devices
}

// deviceNetdev returns the netdev bound to an RDMA device, read from the
// device/net directory that sysfs exposes for ports with a netdev.
func deviceNetdev(deviceDir string) string {
	entries, err := os.ReadDir(filepath.Join(deviceDir, "device/net"))
	if err != nil || len(entries) == 0 {
		return ""
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names[0]
}

// readLinkLayer parses ports/<n>/link_layer, defaulting to Ethernet when
// the file is missing so RoCE devices without the attribute still validate.
func readLinkLayer(path string) v1beta1.LinkLayer {
	raw, err := os.ReadFile(path)
	if err != nil {
		return v1beta1.LinkLayerEthernet
	}
	if strings.TrimSpace(string(raw)) == string(v1beta1.LinkLayerInfiniBand) {
		return v1beta1.LinkLayerInfiniBand
	}
	return v1beta1.LinkLayerEthernet
}

// readSysfsLID parses the hex LID file of a port, 0 when absent or unset.
func readSysfsLID(path string) uint16 {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	text := strings.TrimPrefix(strings.TrimSpace(string(raw)), "0x")
	lid, err := strconv.ParseUint(text, 16, 16)
	if err != nil {
		return 0
	}
	return uint16(lid)
}

// gidSubnetPrefix returns the upper 64 bits of a sysfs GID as 16 hex digits,
// or "" for the zero GID. Every port of one InfiniBand subnet shares it.
func gidSubnetPrefix(value string) string {
	ip := net.ParseIP(value)
	if ip == nil {
		return ""
	}
	ip16 := ip.To16()
	if ip16 == nil {
		return ""
	}
	var zero [8]byte
	if bytes.Equal(ip16[:8], zero[:]) {
		return ""
	}
	return fmt.Sprintf("%02x%02x%02x%02x%02x%02x%02x%02x",
		ip16[0], ip16[1], ip16[2], ip16[3], ip16[4], ip16[5], ip16[6], ip16[7])
}

// parseSysfsGID converts the colon separated sysfs GID text into the
// canonical IP form used for destination lookups. It returns "" for the
// zero GID and for prefix only entries such as fe80::, which InfiniBand
// ports list alongside the real port GID but which no peer can address.
func parseSysfsGID(value string) string {
	ip := net.ParseIP(value)
	if ip == nil {
		return ""
	}
	ip16 := ip.To16()
	if ip16 == nil {
		return ""
	}
	var zero [8]byte
	if bytes.Equal(ip16[8:], zero[:]) {
		return ""
	}
	var gid [16]byte
	copy(gid[:], ip16)
	return gidToIP(gid)
}

// podHolder is one Pod's RDMA usage as seen from /proc: the devices its
// processes hold open and one live pid whose namespaces represent the Pod.
type podHolder struct {
	pid     string
	devices map[string]bool
}

// podDeviceHolders maps Pod UIDs to the RDMA devices their processes hold
// open, discovered through /dev/infiniband/uverbsN file descriptors.
// Processes outside Pods are skipped before their descriptors are listed.
// The uverbs to device mapping comes from the process's own sysfs because
// exclusively assigned VFs are only listed there.
func podDeviceHolders() map[string]*podHolder {
	result := map[string]*podHolder{}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return result
	}
	for _, entry := range entries {
		pid := entry.Name()
		if !isNumeric(pid) {
			continue
		}
		tgid, err := strconv.ParseUint(pid, 10, 32)
		if err != nil {
			continue
		}
		uid := cgroupPodUID(uint32(tgid))
		if uid == "" {
			continue
		}
		uverbsList := openUverbs(pid)
		if len(uverbsList) == 0 {
			continue
		}
		holder := result[uid]
		if holder == nil {
			holder = &podHolder{pid: pid, devices: map[string]bool{}}
			result[uid] = holder
		}
		procSysfs := filepath.Join("/proc", pid, "root/sys")
		for _, uverbs := range uverbsList {
			device := uverbsDevice(procSysfs, uverbs)
			if device == "" {
				device = uverbsDevice(hostSysfsPath, uverbs)
			}
			if device != "" {
				holder.devices[device] = true
			}
		}
	}
	return result
}

// openUverbs lists the uverbs device names a process has open.
func openUverbs(pid string) []string {
	fdDir := filepath.Join("/proc", pid, "fd")
	dir, err := os.Open(fdDir)
	if err != nil {
		return nil
	}
	defer dir.Close()
	names, err := dir.Readdirnames(-1)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var found []string
	for _, name := range names {
		target, err := os.Readlink(filepath.Join(fdDir, name))
		if err != nil {
			continue
		}
		if !strings.HasPrefix(target, "/dev/infiniband/uverbs") {
			continue
		}
		uverbs := strings.TrimPrefix(target, "/dev/infiniband/")
		if !seen[uverbs] {
			seen[uverbs] = true
			found = append(found, uverbs)
		}
	}
	return found
}

func uverbsDevice(sysfs, uverbs string) string {
	raw, err := os.ReadFile(filepath.Join(sysfs, "class/infiniband_verbs", uverbs, "ibdev"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// reportedPod is a Pod from an RDMAEndpoint as seen by an address lookup,
// with the QPNs of the devices carrying that address mapped to their device
// name. The address is a GID in IP form or a "<subnetPrefix>/<lid>" key.
type reportedPod struct {
	uid     string
	node    string
	devices []string
	qpns    map[uint32]string
}

// hasQPN reports whether the Pod brought destQPN to RTR on a device that
// carries the looked up address.
func (p reportedPod) hasQPN(destQPN uint32) bool {
	if destQPN == 0 {
		return false
	}
	_, ok := p.qpns[destQPN]
	return ok
}

// deviceFor names the device serving destQPN, or the only device carrying
// the address when the QPN is unknown, or "" when that is ambiguous.
func (p reportedPod) deviceFor(destQPN uint32) string {
	if p.hasQPN(destQPN) {
		return p.qpns[destQPN]
	}
	if len(p.devices) == 1 {
		return p.devices[0]
	}
	return ""
}

// endpointIndex is the reader side view over every RDMAEndpoint, keyed
// by GID in IP form and by "<subnetPrefix>/<lid>".
type endpointIndex struct {
	pods  map[string][]reportedPod
	nodes map[string][]string
	stale int
}

// portAddresses lists the lookup keys one port answers to.
func portAddresses(port v1beta1.RDMAPort) []string {
	var addresses []string
	seen := map[string]bool{}
	for _, gid := range port.Gids {
		ip := parseSysfsGID(gid.Gid)
		if ip != "" && !seen[ip] {
			seen[ip] = true
			addresses = append(addresses, ip)
		}
	}
	key := lidKey(port.SubnetPrefix, port.Lid)
	if key != "" {
		addresses = append(addresses, key)
	}
	return addresses
}

// buildEndpointIndex merges objects into address lookups, dropping
// objects older than endpointStale so removed nodes stop attracting
// traffic. A Pod appears once per address even when several of its devices
// carry it, and the node list per address is deduplicated.
func buildEndpointIndex(objects []v1beta1.RDMAEndpoint, now time.Time) endpointIndex {
	index := endpointIndex{
		pods:  map[string][]reportedPod{},
		nodes: map[string][]string{},
	}
	nodeSeen := map[string]map[string]bool{}
	for _, object := range objects {
		if now.Sub(object.Status.LastUpdateTime.Time) > endpointStale {
			index.stale++
			continue
		}
		node := object.Status.NodeName
		uid := podUIDOf(object)
		if uid == "" {
			continue
		}
		byAddress := map[string]*reportedPod{}
		for _, iface := range object.Status.Interfaces {
			for _, port := range iface.Ports {
				for _, address := range portAddresses(port) {
					entry := byAddress[address]
					if entry == nil {
						entry = &reportedPod{uid: uid, node: node, qpns: map[uint32]string{}}
						byAddress[address] = entry
					}
					entry.devices = append(entry.devices, iface.RdmaDevice)
					for _, qp := range port.QueuePairs {
						entry.qpns[qp.Qpn] = iface.RdmaDevice
					}
					if nodeSeen[address] == nil {
						nodeSeen[address] = map[string]bool{}
					}
					if !nodeSeen[address][node] {
						nodeSeen[address][node] = true
						index.nodes[address] = append(index.nodes[address], node)
					}
				}
			}
		}
		for address, entry := range byAddress {
			index.pods[address] = append(index.pods[address], *entry)
		}
	}
	return index
}

// refreshEndpoints lists every RDMAEndpoint and rebuilds the address
// lookups used by podForPeer and podForLID.
func (r *resolver) refreshEndpoints() {
	if r.client == nil {
		return
	}
	objects, err := listEndpoints(r.ctx, r.client, nil)
	if err != nil {
		logDedup("rdma_endpoint_list_error", slog.LevelError, "list_rdma_endpoints", "reason", err)
		return
	}
	index := buildEndpointIndex(objects, time.Now())
	r.gidPods = index.pods
	r.gidNodes = index.nodes
	r.localPorts = localPortTable(hostSysfsPath)
	logDedup("rdma_endpoint_refresh_complete", slog.LevelDebug, "rdma_endpoint_refresh_complete",
		"object_count", len(objects)-index.stale, "stale_count", index.stale,
		"address_count", len(index.pods), "local_port_count", len(r.localPorts))
}

// localPortTable maps "<device>/<port>" of this node to its subnet prefix,
// which the sender needs to scope a LID addressed peer. It is read through
// the host namespace like the objects themselves.
func localPortTable(sysfs string) map[string]string {
	table := map[string]string{}
	hostView(func() {
		for device, info := range deviceTable(sysfs) {
			for _, port := range info.ports {
				if port.subnetPrefix != "" {
					table[device+"/"+strconv.Itoa(port.num)] = port.subnetPrefix
				}
			}
		}
	})
	return table
}

// lidKeyFor builds the lookup key for a LID addressed peer from the local
// device and port the QP was created on. Both ends of a LID addressed QP
// share the subnet, so the local prefix scopes the remote LID. Empty when
// the local port is unknown.
func (r *resolver) lidKeyFor(device string, port uint8, dlid uint16) string {
	if device == "" || port == 0 || dlid == 0 {
		return ""
	}
	prefix, ok := r.localPorts[device+"/"+strconv.Itoa(int(port))]
	if !ok {
		return ""
	}
	return lidKey(prefix, dlid)
}

// podForReportedAddress attributes a destination through the objects.
// address is a GID in IP form or a "<subnetPrefix>/<lid>" key. Among the
// Pods holding it, one whose device carries destQPN wins. Without a QPN
// match an address held by exactly one Pod resolves to that Pod. An address
// held by several Pods without a QPN match stays unresolved, with the
// candidate nodes named in the log label. The device is returned when the
// object pins it down.
func (r *resolver) podForReportedAddress(address string, destQPN uint32) (peer, string, bool) {
	pods := r.gidPods[address]
	nodes := r.gidNodes[address]
	topic := fmt.Sprintf("destination_resolution:%s:%d", address, destQPN)
	var matched []reportedPod
	for _, pod := range pods {
		if pod.hasQPN(destQPN) {
			matched = append(matched, pod)
		}
	}
	method := "rdma_endpoint_qpn"
	if len(matched) != 1 {
		method = "rdma_endpoint_address"
		matched = nil
		if len(pods) == 1 {
			matched = pods
		}
	}
	if len(matched) == 1 {
		chosen := matched[0]
		device := chosen.deviceFor(destQPN)
		p, known := r.byUID[chosen.uid]
		if !known {
			logDedup(topic, slog.LevelDebug, "destination_resolution_failed",
				"destination", address, "destination_qpn", destQPN, "method", method,
				"reason", "pod_uid_not_in_index", "pod_uid", chosen.uid)
			return peer{label: "pod-uid:" + chosen.uid}, device, true
		}
		logDedup(topic, slog.LevelDebug, "destination_resolution_complete",
			"destination", address, "destination_qpn", destQPN, "method", method, "node", chosen.node,
			"destination_pod", p.String(), "destination_device", device)
		return p, device, true
	}
	if len(pods) == 0 && len(nodes) == 0 {
		return peer{}, "", false
	}
	// The address is known to belong to a node but cannot be pinned to one
	// Pod, either because several Pods share it without a QPN match or
	// because no Pod holds it. The node label makes the log useful while the
	// flow stays unresolved.
	reason := "address_shared_by_multiple_pods_without_qpn_match"
	if len(pods) == 0 {
		reason = "address_reported_by_node_without_pod"
	}
	logDedup(topic, slog.LevelDebug, "destination_resolution_failed",
		"destination", address, "destination_qpn", destQPN, "method", "rdma_endpoint",
		"node", strings.Join(nodes, ","), "reason", reason, "pod_count", len(pods))
	return peer{label: "node:" + strings.Join(nodes, ",")}, "", true
}
