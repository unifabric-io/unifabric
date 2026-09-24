// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"regexp"
	"strings"
	"unicode"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	networkStatusAnnotation = "k8s.v1.cni.cncf.io/network-status"
	calicoPodIPAnnotation   = "cni.projectcalico.org/podIP"
	calicoPodIPsAnnotation  = "cni.projectcalico.org/podIPs"
)

var podUIDPattern = regexp.MustCompile(
	`pod([0-9a-f]{8}[-_][0-9a-f]{4}[-_][0-9a-f]{4}[-_][0-9a-f]{4}[-_][0-9a-f]{12})`)

// peer is one end of a flow. Empty Pod fields mean the end was not resolved
// to a Pod, and label then carries the log fallback text such as
// pid:<n>(<comm>) or the raw IP.
type peer struct {
	Namespace string
	Name      string
	Owner     ownerInfo
	label     string
	// hostNetwork marks Pods whose RDMA GIDs belong to the node NIC and
	// can only be attributed through the node report.
	hostNetwork bool
}

func (p peer) isPod() bool {
	return p.Name != ""
}

// String returns the log text, namespace/name for Pods and the fallback
// label otherwise.
func (p peer) String() string {
	if p.isPod() {
		return p.Namespace + "/" + p.Name
	}
	if p.label == "" {
		return "unknown"
	}
	return p.label
}

// resolver maps tgid -> src pod (via cgroup pod UID) and peer IP -> dst pod
// (via the Kubernetes API). Without a client it degrades to printing raw pod
// UIDs and IPs.
type resolver struct {
	ctx context.Context
	// client reads RDMAEndpoint objects through the manager cache.
	client client.Client
	// podReader lists Pods of the whole cluster, backed by a dedicated cache.
	podReader client.Reader
	owners    *ownerResolver
	byUID     map[string]peer
	byIP      map[string]peer
	byMAC     map[string]peer
	uidByTgid map[uint32]string

	// gidPods and gidNodes come from RDMAEndpoint objects, see endpoint.go.
	// Keys are GIDs in IP form and "<subnetPrefix>/<lid>" strings.
	gidPods  map[string][]reportedPod
	gidNodes map[string][]string
	// localPorts maps "<device>/<port>" of this node to its subnet prefix.
	localPorts map[string]string
}

// newResolver builds a resolver. A nil client disables every API lookup,
// which unit tests and non cluster runs rely on.
func newResolver(ctx context.Context, c client.Client, apiReader, podReader client.Reader) *resolver {
	r := &resolver{
		ctx:       ctx,
		client:    c,
		podReader: podReader,
		byUID:     map[string]peer{},
		byIP:      map[string]peer{},
		byMAC:     map[string]peer{},
		uidByTgid: map[uint32]string{},
	}
	if ctx == nil {
		r.ctx = context.Background()
	}
	if c == nil {
		logger.Warn("kubernetes_api", "status", "unavailable", "fallback", "raw_pod_uid_and_peer_ip")
		return r
	}
	r.owners = newOwnerResolver(apiReader)
	return r
}

func (r *resolver) refresh() {
	if r.client == nil || r.podReader == nil {
		return
	}
	var pods corev1.PodList
	if err := r.podReader.List(r.ctx, &pods); err != nil {
		logDedup("pod_list_error", slog.LevelError, "get_pod_list", "reason", err)
		return
	}
	byUID := make(map[string]peer, len(pods.Items))
	byIP := make(map[string]peer, len(pods.Items))
	byMAC := make(map[string]peer, len(pods.Items))
	for i := range pods.Items {
		item := &pods.Items[i]
		p := peer{
			Namespace:   item.Namespace,
			Name:        item.Name,
			Owner:       r.owners.topOwner(r.ctx, item.Namespace, item.Name, item.OwnerReferences),
			hostNetwork: item.Spec.HostNetwork,
		}
		byUID[strings.ToLower(string(item.UID))] = p
		// hostNetwork Pods share the node IP, so indexing them by IP would
		// attribute node traffic to an arbitrary Pod. They are resolved
		// through the node GID reports instead.
		if item.Spec.HostNetwork {
			continue
		}
		addPodIP(byIP, item.Status.PodIP, p)
		for _, podIP := range item.Status.PodIPs {
			addPodIP(byIP, podIP.IP, p)
		}
		for _, ip := range podAnnotationIPs(item.Annotations) {
			addPodIP(byIP, ip, p)
		}
		for _, mac := range podAnnotationMACs(item.Annotations) {
			byMAC[mac] = p
		}
	}
	r.byUID = byUID
	r.byIP = byIP
	r.byMAC = byMAC
	logDedup("pod_index_refresh_complete", slog.LevelDebug, "pod_index_refresh_complete",
		"pod_count", len(pods.Items), "uid_count", len(byUID), "ip_count", len(byIP), "mac_count", len(byMAC))
	r.refreshEndpoints()
}

// podUIDForTgid returns the Pod UID owning a process, cached per tgid,
// or "" for processes outside Pods.
func (r *resolver) podUIDForTgid(tgid uint32) string {
	uid, ok := r.uidByTgid[tgid]
	if !ok {
		uid = cgroupPodUID(tgid)
		if uid != "" {
			r.uidByTgid[tgid] = uid
		}
	}
	return uid
}

func (r *resolver) podForTgid(tgid uint32, comm string) peer {
	uid := r.podUIDForTgid(tgid)
	label := fmt.Sprintf("pid:%d", tgid)
	if comm != "" {
		label = fmt.Sprintf("pid:%d(%s)", tgid, comm)
	}
	if uid == "" {
		return peer{label: label}
	}
	if p, ok := r.byUID[uid]; ok {
		return p
	}
	return peer{label: "pod-uid:" + uid}
}

// podForLID resolves an InfiniBand peer addressed by LID without a GRH.
// The local device and port scope the LID to a subnet through the node's
// own port table, then the RDMAEndpoints are searched like for a GID.
func (r *resolver) podForLID(device string, port uint8, dlid uint16, destQPN uint32) (peer, string) {
	label := fmt.Sprintf("lid:%d", dlid)
	key := r.lidKeyFor(device, port, dlid)
	if key == "" {
		logDedup("destination_resolution:"+label+":"+device, slog.LevelDebug, "destination_resolution_failed",
			"destination_lid", dlid, "device", device, "port", port, "reason", "local_subnet_prefix_unknown")
		return peer{label: label}, ""
	}
	if p, dstDevice, ok := r.podForReportedAddress(key, destQPN); ok {
		return p, dstDevice
	}
	logDedup("destination_resolution:"+key, slog.LevelDebug, "destination_resolution_failed",
		"destination_lid", dlid, "subnet_prefix", key, "reason", "lid_not_in_rdma_endpoints")
	return peer{label: label}, ""
}

// podForPeer resolves a destination from its GID in IP form and, when the
// sender recorded it, the destination QPN. The second result is the RDMA
// device on the destination when the node report identifies it.
func (r *resolver) podForPeer(ip string, destQPN uint32) (peer, string) {
	if p, ok := r.byIP[ip]; ok {
		logDedup("destination_resolution:"+ip, slog.LevelDebug, "destination_resolution_complete",
			"destination_ip", ip, "method", "ip_match", "destination_pod", p.String())
		return p, ""
	}
	// RoCEv2 often uses link-local GIDs whose low 64 bits are the NIC MAC
	// in EUI-64 form.
	mac := eui64MAC(ip)
	if mac != "" {
		if p, ok := r.byMAC[mac]; ok {
			logDedup("destination_resolution:"+ip, slog.LevelDebug, "destination_resolution_complete",
				"destination_ip", ip, "method", "eui64_match", "mac", mac, "destination_pod", p.String())
			return p, ""
		}
	}
	// RDMAEndpoints cover hostNetwork Pods, whose GIDs belong to the node NIC.
	if p, device, ok := r.podForReportedAddress(ip, destQPN); ok {
		return p, device
	}
	if mac != "" {
		logDedup("destination_resolution:"+ip, slog.LevelDebug, "destination_resolution_failed",
			"destination_ip", ip, "derived_mac", mac, "reason", "mac_not_in_pod_index")
		return peer{label: ip}, ""
	}
	logDedup("destination_resolution:"+ip, slog.LevelDebug, "destination_resolution_failed",
		"destination_ip", ip, "reason", "ip_not_in_pod_index")
	return peer{label: ip}, ""
}

// eui64MAC recovers the EUI-64 encoded MAC from an fe80::/10 link-local
// address.
func eui64MAC(value string) string {
	ip := net.ParseIP(value)
	if ip == nil {
		return ""
	}
	ip16 := ip.To16()
	if ip16 == nil || ip.To4() != nil || !ip16.IsLinkLocalUnicast() {
		return ""
	}
	if ip16[11] != 0xff || ip16[12] != 0xfe {
		return ""
	}
	mac := net.HardwareAddr{ip16[8] ^ 0x02, ip16[9], ip16[10], ip16[13], ip16[14], ip16[15]}
	return mac.String()
}

func addPodIP(byIP map[string]peer, ip string, p peer) {
	if normalized := normalizeIP(ip); normalized != "" {
		byIP[normalized] = p
	}
}

func podAnnotationIPs(annotations map[string]string) []string {
	values := networkStatusIPs(annotations[networkStatusAnnotation])
	values = append(values, annotationIPs(annotations[calicoPodIPAnnotation])...)
	values = append(values, annotationIPs(annotations[calicoPodIPsAnnotation])...)

	seen := make(map[string]bool)
	ips := make([]string, 0, len(values))
	for _, value := range values {
		ip := normalizeIP(value)
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true
		ips = append(ips, ip)
	}
	return ips
}

func annotationIPs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		var value string
		if err := json.Unmarshal([]byte(raw), &value); err == nil {
			values = []string{value}
		} else {
			values = strings.FieldsFunc(raw, func(r rune) bool {
				return r == ',' || unicode.IsSpace(r)
			})
		}
	}

	seen := make(map[string]bool)
	ips := make([]string, 0, len(values))
	for _, value := range values {
		ip := normalizeIP(value)
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true
		ips = append(ips, ip)
	}
	return ips
}

func networkStatusIPs(raw string) []string {
	seen := make(map[string]bool)
	var ips []string
	for _, status := range parseNetworkStatus(raw) {
		for _, value := range status.IPs {
			ip := normalizeIP(value)
			if ip == "" || seen[ip] {
				continue
			}
			seen[ip] = true
			ips = append(ips, ip)
		}
	}
	return ips
}

func podAnnotationMACs(annotations map[string]string) []string {
	seen := make(map[string]bool)
	var macs []string
	for _, status := range parseNetworkStatus(annotations[networkStatusAnnotation]) {
		mac := normalizeMAC(status.MAC)
		if mac == "" || seen[mac] {
			continue
		}
		seen[mac] = true
		macs = append(macs, mac)
	}
	return macs
}

type networkStatus struct {
	IPs []string `json:"ips"`
	MAC string   `json:"mac"`
}

func parseNetworkStatus(raw string) []networkStatus {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var statuses []networkStatus
	if err := json.Unmarshal([]byte(raw), &statuses); err != nil {
		var status networkStatus
		if err := json.Unmarshal([]byte(raw), &status); err != nil {
			return nil
		}
		statuses = []networkStatus{status}
	}
	return statuses
}

func normalizeMAC(value string) string {
	mac, err := net.ParseMAC(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	return mac.String()
}

func normalizeIP(value string) string {
	value = strings.TrimSpace(value)
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}
	if ip, _, err := net.ParseCIDR(value); err == nil {
		return ip.String()
	}
	return ""
}

func cgroupPodUID(tgid uint32) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", tgid))
	if err != nil {
		return ""
	}
	m := podUIDPattern.FindSubmatch(data)
	if m == nil {
		return ""
	}
	return strings.ToLower(strings.ReplaceAll(string(m[1]), "_", "-"))
}
