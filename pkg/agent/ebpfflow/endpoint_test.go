// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
)

func writeSysfsFile(t *testing.T, path, content string) {
	t.Helper()
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(path, []byte(content+"\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseSysfsGID(t *testing.T) {
	cases := map[string]string{
		"0000:0000:0000:0000:0000:ffff:0a10:0102": "10.16.1.2",
		"fe80:0000:0000:0000:0c42:a1ff:fe8f:1234": "fe80::c42:a1ff:fe8f:1234",
		"0000:0000:0000:0000:0000:0000:0000:0000": "",
		"not-a-gid": "",
	}
	for raw, want := range cases {
		got := parseSysfsGID(raw)
		if got != want {
			t.Errorf("parseSysfsGID(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestDeviceTableReadsPortsWithGIDIndexes(t *testing.T) {
	sysfs := t.TempDir()
	port := filepath.Join(sysfs, "class/infiniband/mlx5_0/ports/1/gids")
	writeSysfsFile(t, filepath.Join(port, "0"), "fe80:0000:0000:0000:0c42:a1ff:fe8f:1234")
	writeSysfsFile(t, filepath.Join(port, "1"), "fe80:0000:0000:0000:0c42:a1ff:fe8f:1234")
	writeSysfsFile(t, filepath.Join(port, "2"), "0000:0000:0000:0000:0000:ffff:0a10:0102")
	writeSysfsFile(t, filepath.Join(port, "3"), "0000:0000:0000:0000:0000:ffff:0a10:0102")
	writeSysfsFile(t, filepath.Join(port, "4"), "0000:0000:0000:0000:0000:0000:0000:0000")
	writeSysfsFile(t, filepath.Join(sysfs, "class/infiniband/mlx5_0/ports/1/lid"), "0x0")
	writeSysfsFile(t, filepath.Join(sysfs, "class/infiniband/mlx5_0/ports/1/link_layer"), "Ethernet")
	writeSysfsFile(t, filepath.Join(sysfs, "class/infiniband/mlx5_0/device/net/eth1/.keep"), "")
	// An InfiniBand port with an SM assigned LID, a non default subnet and
	// the prefix only placeholder that IB GID tables carry.
	writeSysfsFile(t, filepath.Join(sysfs, "class/infiniband/mlx5_1/ports/1/gids/0"),
		"fe81:0000:0000:0001:0c42:a103:0068:1a2b")
	writeSysfsFile(t, filepath.Join(sysfs, "class/infiniband/mlx5_1/ports/1/gids/1"),
		"fe81:0000:0000:0001:0000:0000:0000:0000")
	writeSysfsFile(t, filepath.Join(sysfs, "class/infiniband/mlx5_1/ports/1/lid"), "0x2a")
	writeSysfsFile(t, filepath.Join(sysfs, "class/infiniband/mlx5_1/ports/1/link_layer"), "InfiniBand")

	got := deviceTable(sysfs)
	if len(got) != 2 {
		t.Fatalf("devices = %v, want 2", got)
	}
	roce := got["mlx5_0"]
	if roce.netdev != "eth1" {
		t.Fatalf("mlx5_0 netdev = %q, want eth1", roce.netdev)
	}
	if len(roce.ports) != 1 {
		t.Fatalf("mlx5_0 ports = %+v, want 1", roce.ports)
	}
	rocePort := roce.ports[0]
	if rocePort.linkLayer != v1beta1.LinkLayerEthernet || rocePort.lid != 0 || rocePort.subnetPrefix != "fe80000000000000" {
		t.Fatalf("mlx5_0 port = %+v", rocePort)
	}
	// RoCE v1 and v2 entries are both kept with their indexes, zero GID is dropped.
	if len(rocePort.gids) != 4 || rocePort.gids[0].Index != 0 || rocePort.gids[3].Index != 3 {
		t.Fatalf("mlx5_0 gids = %+v, want indexes 0-3", rocePort.gids)
	}
	if lidKey(rocePort.subnetPrefix, rocePort.lid) != "" {
		t.Fatal("RoCE port must not produce a LID key")
	}
	ib := got["mlx5_1"]
	if ib.netdev != "" {
		t.Fatalf("mlx5_1 netdev = %q, want empty", ib.netdev)
	}
	ibPort := ib.ports[0]
	if ibPort.num != 1 || ibPort.lid != 42 || ibPort.linkLayer != v1beta1.LinkLayerInfiniBand {
		t.Fatalf("mlx5_1 port = %+v, want port 1 lid 42 InfiniBand", ibPort)
	}
	if ibPort.subnetPrefix != "fe81000000000001" {
		t.Fatalf("mlx5_1 subnet prefix = %q", ibPort.subnetPrefix)
	}
	if len(ibPort.gids) != 1 || ibPort.gids[0].Gid != "fe81:0000:0000:0001:0c42:a103:0068:1a2b" {
		t.Fatalf("mlx5_1 gids = %+v, want the port GID only", ibPort.gids)
	}
	if lidKey(ibPort.subnetPrefix, ibPort.lid) != "fe81000000000001/42" {
		t.Fatalf("lid key = %q", lidKey(ibPort.subnetPrefix, ibPort.lid))
	}
}

func TestReadSysfsLIDAndSubnetPrefix(t *testing.T) {
	dir := t.TempDir()
	writeSysfsFile(t, filepath.Join(dir, "lid"), "0xffff")
	lid := readSysfsLID(filepath.Join(dir, "lid"))
	if lid != 0xffff {
		t.Fatalf("lid = %d, want 65535", lid)
	}
	missing := readSysfsLID(filepath.Join(dir, "missing"))
	if missing != 0 {
		t.Fatalf("missing lid = %d, want 0", missing)
	}
	mapped := gidSubnetPrefix("0000:0000:0000:0000:0000:ffff:0a10:0102")
	if mapped != "" {
		t.Fatalf("IPv4 mapped GID has no subnet prefix, got %q", mapped)
	}
	linkLocal := gidSubnetPrefix("fe80:0000:0000:0000:0c42:a1ff:fe8f:1234")
	if linkLocal != "fe80000000000000" {
		t.Fatalf("subnet prefix = %q", linkLocal)
	}
}

func TestUverbsDevice(t *testing.T) {
	sysfs := t.TempDir()
	writeSysfsFile(t, filepath.Join(sysfs, "class/infiniband_verbs/uverbs3/ibdev"), "mlx5_3")
	got := uverbsDevice(sysfs, "uverbs3")
	if got != "mlx5_3" {
		t.Fatalf("uverbsDevice = %q, want mlx5_3", got)
	}
	missing := uverbsDevice(sysfs, "uverbs9")
	if missing != "" {
		t.Fatalf("missing uverbs resolved to %q", missing)
	}
}

func testEndpoint(node, uid, namespace, name string, updatedAt time.Time, interfaces ...v1beta1.RDMAInterface) v1beta1.RDMAEndpoint {
	return v1beta1.RDMAEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "v1", Kind: "Pod", Name: name, UID: types.UID(uid),
			}},
		},
		Status: v1beta1.RDMAEndpointStatus{
			NodeName:       node,
			LastUpdateTime: metav1.NewTime(updatedAt),
			Interfaces:     interfaces,
		},
	}
}

func TestBuildEndpointIndexDropsStaleObjects(t *testing.T) {
	now := time.Now()
	fresh := testEndpoint("node-a", "uid-a", "tenant-a", "vllm-0", now.Add(-time.Minute),
		v1beta1.RDMAInterface{RdmaDevice: "mlx5_0", Ports: []v1beta1.RDMAPort{{
			Port: 1, LinkLayer: v1beta1.LinkLayerEthernet,
			Gids: []v1beta1.RDMAGid{
				{Index: 0, Gid: "fe80:0000:0000:0000:0000:0000:0000:0001"},
				{Index: 2, Gid: "0000:0000:0000:0000:0000:ffff:0a10:0102"},
				{Index: 3, Gid: "0000:0000:0000:0000:0000:ffff:0a10:0102"},
			},
			QueuePairs: []v1beta1.RDMAQueuePair{{Qpn: 100, Pid: 1}, {Qpn: 101, Pid: 1}},
		}}},
		v1beta1.RDMAInterface{RdmaDevice: "mlx5_1", Ports: []v1beta1.RDMAPort{{
			Port: 1, LinkLayer: v1beta1.LinkLayerInfiniBand, Lid: 7, SubnetPrefix: "fe80000000000000",
			Gids:       []v1beta1.RDMAGid{{Index: 0, Gid: "fe80:0000:0000:0000:0000:0000:0000:0001"}},
			QueuePairs: []v1beta1.RDMAQueuePair{{Qpn: 200, Pid: 1}},
		}}},
	)
	// A second Pod on the same node sharing fe80::1 must not duplicate the node.
	sibling := testEndpoint("node-a", "uid-s", "tenant-a", "monitor", now.Add(-time.Minute),
		v1beta1.RDMAInterface{RdmaDevice: "mlx5_0", Ports: []v1beta1.RDMAPort{{
			Port: 1, Gids: []v1beta1.RDMAGid{{Index: 0, Gid: "fe80:0000:0000:0000:0000:0000:0000:0001"}},
		}}},
	)
	stale := testEndpoint("node-b", "uid-b", "tenant-b", "old", now.Add(-endpointStale-time.Minute),
		v1beta1.RDMAInterface{RdmaDevice: "mlx5_0", Ports: []v1beta1.RDMAPort{{
			Port: 1, Gids: []v1beta1.RDMAGid{{Index: 0, Gid: "0000:0000:0000:0000:0000:ffff:0a10:0109"}},
		}}},
	)
	index := buildEndpointIndex([]v1beta1.RDMAEndpoint{fresh, sibling, stale}, now)
	if index.stale != 1 {
		t.Fatalf("stale = %d, want 1", index.stale)
	}
	// Both RoCE versions of one address collapse into one IP key.
	if len(index.nodes["10.16.1.2"]) != 1 || index.nodes["10.16.1.2"][0] != "node-a" {
		t.Fatalf("nodes for 10.16.1.2 = %v", index.nodes["10.16.1.2"])
	}
	if len(index.nodes["fe80::1"]) != 1 {
		t.Fatalf("nodes for fe80::1 = %v, want node-a once", index.nodes["fe80::1"])
	}
	_, present := index.nodes["10.16.1.9"]
	if present {
		t.Fatal("stale object GID still indexed")
	}
	// fe80::1 is carried by two devices of uid-a, so that Pod appears once
	// with the QPNs of both devices merged, next to the sibling Pod.
	var shared []reportedPod
	for _, pod := range index.pods["fe80::1"] {
		if pod.uid == "uid-a" {
			shared = append(shared, pod)
		}
	}
	if len(index.pods["fe80::1"]) != 2 || len(shared) != 1 || shared[0].node != "node-a" {
		t.Fatalf("pods for fe80::1 = %+v", index.pods["fe80::1"])
	}
	if !shared[0].hasQPN(100) || !shared[0].hasQPN(200) || shared[0].hasQPN(300) {
		t.Fatalf("qpns for fe80::1 = %v", shared[0].qpns)
	}
	if shared[0].deviceFor(200) != "mlx5_1" || shared[0].deviceFor(100) != "mlx5_0" {
		t.Fatalf("device lookup by qpn = %v", shared[0].qpns)
	}
	if shared[0].deviceFor(0) != "" {
		t.Fatalf("two devices carry fe80::1, deviceFor(0) = %q, want empty", shared[0].deviceFor(0))
	}
	only := index.pods["10.16.1.2"]
	if len(only) != 1 || only[0].hasQPN(200) || only[0].deviceFor(0) != "mlx5_0" {
		t.Fatalf("10.16.1.2 must only carry mlx5_0, got %+v", only)
	}
	byLID := index.pods["fe80000000000000/7"]
	if len(byLID) != 1 || !byLID[0].hasQPN(200) || byLID[0].hasQPN(100) {
		t.Fatalf("LID key must index mlx5_1 only, got %+v", byLID)
	}
	lidNodes := index.nodes["fe80000000000000/7"]
	if len(lidNodes) != 1 || lidNodes[0] != "node-a" {
		t.Fatalf("nodes for LID key = %v", lidNodes)
	}
}

func TestPodForLIDScopesByLocalSubnetPrefix(t *testing.T) {
	worker := peer{Namespace: "train", Name: "worker-1"}
	r := &resolver{
		byUID:    map[string]peer{"uid-w": worker},
		byIP:     map[string]peer{},
		byMAC:    map[string]peer{},
		gidNodes: map[string][]string{"fe80000000000000/42": {"node-b"}},
		gidPods: map[string][]reportedPod{
			"fe80000000000000/42": {reported("uid-w", "node-b", "mlx5_0", 5000)},
		},
		localPorts: map[string]string{"mlx5_2/1": "fe80000000000000", "mlx5_3/1": "fe81000000000001"},
	}

	got, device := r.podForLID("mlx5_2", 1, 42, 5000)
	if got != worker || device != "mlx5_0" {
		t.Fatalf("podForLID = %+v device=%q, want %+v mlx5_0", got, device, worker)
	}
	// The same LID on another rail belongs to another subnet and must not match.
	other, _ := r.podForLID("mlx5_3", 1, 42, 5000)
	if other.isPod() || other.String() != "lid:42" {
		t.Fatalf("other subnet = %+v, want unresolved lid label", other)
	}
	unknownPort, _ := r.podForLID("mlx5_9", 1, 42, 5000)
	if unknownPort.isPod() {
		t.Fatalf("unknown local port must stay unresolved, got %+v", unknownPort)
	}
}

func reported(uid, node, device string, qpns ...uint32) reportedPod {
	set := map[uint32]string{}
	for _, qpn := range qpns {
		set[qpn] = device
	}
	return reportedPod{uid: uid, node: node, devices: []string{device}, qpns: set}
}

func TestPodForPeerFallsBackToEndpoints(t *testing.T) {
	hostPod := peer{
		Namespace: "tenant-a", Name: "vllm-host-0",
		Owner: ownerInfo{Kind: "StatefulSet", Namespace: "tenant-a", Name: "vllm-host"},
	}
	r := &resolver{
		byUID:    map[string]peer{"uid-a": hostPod},
		byIP:     map[string]peer{},
		byMAC:    map[string]peer{},
		gidNodes: map[string][]string{"10.0.0.5": {"node-a"}, "10.0.0.6": {"node-b"}, "10.0.0.7": {"node-c"}},
		gidPods: map[string][]reportedPod{
			"10.0.0.5": {reported("uid-a", "node-a", "mlx5_0")},
			"10.0.0.6": {reported("uid-x", "node-b", "mlx5_0"), reported("uid-y", "node-b", "mlx5_0")},
		},
	}

	got, device := r.podForPeer("10.0.0.5", 0)
	if got != hostPod || device != "mlx5_0" {
		t.Fatalf("unique GID holder = %+v device=%q, want %+v mlx5_0", got, device, hostPod)
	}
	shared, _ := r.podForPeer("10.0.0.6", 0)
	if shared.isPod() || shared.String() != "node:node-b" {
		t.Fatalf("shared GID = %+v, want unresolved node label", shared)
	}
	nodeOnly, _ := r.podForPeer("10.0.0.7", 0)
	if nodeOnly.isPod() || nodeOnly.String() != "node:node-c" {
		t.Fatalf("node only GID = %+v, want unresolved node label", nodeOnly)
	}
	unknown, _ := r.podForPeer("10.0.0.8", 0)
	if unknown.isPod() || unknown.String() != "10.0.0.8" {
		t.Fatalf("unknown GID = %+v, want raw ip", unknown)
	}
}

func TestPodForPeerUsesDestinationQPNToDisambiguate(t *testing.T) {
	prefill := peer{Namespace: "dc", Name: "prefill-0"}
	monitor := peer{Namespace: "unifabric", Name: "unifabric-agent-1"}
	r := &resolver{
		byUID:    map[string]peer{"uid-prefill": prefill, "uid-monitor": monitor},
		byIP:     map[string]peer{},
		byMAC:    map[string]peer{},
		gidNodes: map[string][]string{"172.17.3.103": {"g1-03"}},
		gidPods: map[string][]reportedPod{
			"172.17.3.103": {
				reported("uid-monitor", "g1-03", "mlx5_2"),
				reported("uid-prefill", "g1-03", "mlx5_2", 15246, 15247),
			},
		},
	}

	got, device := r.podForPeer("172.17.3.103", 15246)
	if got != prefill || device != "mlx5_2" {
		t.Fatalf("qpn match = %+v device=%q, want %+v mlx5_2", got, device, prefill)
	}
	miss, _ := r.podForPeer("172.17.3.103", 999)
	if miss.isPod() || miss.String() != "node:g1-03" {
		t.Fatalf("shared GID without qpn match = %+v, want unresolved", miss)
	}
	zero, _ := r.podForPeer("172.17.3.103", 0)
	if zero.isPod() {
		t.Fatalf("shared GID without dest qpn = %+v, want unresolved", zero)
	}
}

func TestPodForPeerPrefersPodIPOverEndpoints(t *testing.T) {
	ipPod := peer{Namespace: "tenant-a", Name: "sriov-0"}
	reportPod := peer{Namespace: "tenant-b", Name: "host-0"}
	r := &resolver{
		byUID:    map[string]peer{"uid-b": reportPod},
		byIP:     map[string]peer{"10.16.1.2": ipPod},
		byMAC:    map[string]peer{},
		gidNodes: map[string][]string{"10.16.1.2": {"node-a"}},
		gidPods:  map[string][]reportedPod{"10.16.1.2": {reported("uid-b", "node-a", "mlx5_0", 7)}},
	}
	got, device := r.podForPeer("10.16.1.2", 7)
	if got != ipPod || device != "" {
		t.Fatalf("podForPeer = %+v device=%q, want IP index match %+v with no device", got, device, ipPod)
	}
}

func TestCollectEndpointsBuildsOwnedObjects(t *testing.T) {
	sysfs := t.TempDir()
	writeSysfsFile(t, filepath.Join(sysfs, "class/infiniband/mlx5_0/ports/1/gids/0"),
		"0000:0000:0000:0000:0000:ffff:ac11:0367")
	writeSysfsFile(t, filepath.Join(sysfs, "class/infiniband/mlx5_1/ports/1/gids/0"),
		"0000:0000:0000:0000:0000:ffff:ac11:0467")
	writeSysfsFile(t, filepath.Join(sysfs, "class/infiniband/mlx5_1/ports/1/lid"), "0x1c")
	writeSysfsFile(t, filepath.Join(sysfs, "class/infiniband/mlx5_1/device/net/ib1/.keep"), "")
	res := &resolver{byUID: map[string]peer{
		"uid-a": {Namespace: "dc", Name: "prefill-0", hostNetwork: true},
		"uid-b": {Namespace: "dc", Name: "sriov-0"},
	}}
	qps := map[string]map[string][]qpRecord{
		"uid-a":       {"mlx5_1": {{qpn: 15247, pid: 400, port: 1}, {qpn: 15246, pid: 400, port: 1}}},
		"uid-b":       {"mlx5_0": {{qpn: 77, pid: 500, port: 1}}},
		"uid-unknown": {"mlx5_0": {{qpn: 88, pid: 600, port: 0}}},
	}

	objects := collectEndpoints(sysfs, "g1-03", res, qps, false)
	if len(objects) != 2 {
		t.Fatalf("objects = %+v, want hostNetwork and unknown Pods only", objects)
	}
	for _, object := range objects {
		if podUIDOf(object) == "uid-b" {
			t.Fatalf("Pod with its own network namespace must not be published: %+v", object)
		}
		if podUIDOf(object) == "uid-unknown" && (object.Name != "" || object.Namespace != "") {
			t.Fatalf("unknown Pod must stay unnamed until indexed: %+v", object.ObjectMeta)
		}
	}

	objects = collectEndpoints(sysfs, "g1-03", res, qps, true)
	if len(objects) != 3 {
		t.Fatalf("all_pods objects = %+v, want 3", objects)
	}

	res.byUID = map[string]peer{"uid-a": {Namespace: "dc", Name: "prefill-0", hostNetwork: true}}
	qps = map[string]map[string][]qpRecord{"uid-a": {"mlx5_1": {{qpn: 15247, pid: 400, port: 1}, {qpn: 15246, pid: 400, port: 1}}}}
	objects = collectEndpoints(sysfs, "g1-03", res, qps, false)
	if len(objects) != 1 {
		t.Fatalf("objects = %+v, want 1", objects)
	}
	object := objects[0]
	if object.APIVersion != "unifabric.io/v1beta1" || object.Kind != "RDMAEndpoint" {
		t.Fatalf("type meta = %+v", object.TypeMeta)
	}
	if object.Name != "prefill-0" || object.Namespace != "dc" {
		t.Fatalf("object must be named after the Pod, got %s/%s", object.Namespace, object.Name)
	}
	if object.Labels[v1beta1.RDMAEndpointLabelNode] != "g1-03" {
		t.Fatalf("labels = %v", object.Labels)
	}
	if len(object.OwnerReferences) != 1 {
		t.Fatalf("ownerReferences = %+v", object.OwnerReferences)
	}
	owner := object.OwnerReferences[0]
	if owner.Kind != "Pod" || owner.Name != "prefill-0" || string(owner.UID) != "uid-a" ||
		owner.Controller == nil || !*owner.Controller || owner.BlockOwnerDeletion == nil || !*owner.BlockOwnerDeletion {
		t.Fatalf("owner = %+v", owner)
	}
	if object.Status.NodeName != "g1-03" || !object.Status.HostNetwork {
		t.Fatalf("status = %+v", object.Status)
	}
	if len(object.Status.Interfaces) != 1 {
		t.Fatalf("interfaces = %+v, want mlx5_1 from qp_infos", object.Status.Interfaces)
	}
	iface := object.Status.Interfaces[0]
	if iface.RdmaDevice != "mlx5_1" || iface.Name != "ib1" {
		t.Fatalf("interface = %+v", iface)
	}
	if len(iface.Ports) != 1 || iface.Ports[0].Port != 1 || iface.Ports[0].Lid != 28 {
		t.Fatalf("ports = %+v, want port 1 lid 28", iface.Ports)
	}
	if len(iface.Ports[0].Gids) != 1 || iface.Ports[0].Gids[0].Gid != "0000:0000:0000:0000:0000:ffff:ac11:0467" {
		t.Fatalf("gids = %+v", iface.Ports[0].Gids)
	}
	qpsOut := iface.Ports[0].QueuePairs
	if len(qpsOut) != 2 || qpsOut[0].Qpn != 15246 || qpsOut[1].Qpn != 15247 || qpsOut[0].Pid != 400 {
		t.Fatalf("queue pairs = %+v, want sorted with pid", qpsOut)
	}
	if qpCount(objects) != 2 {
		t.Fatalf("qpCount = %d, want 2", qpCount(objects))
	}
}

func TestBuildInterfaceKeepsQPsWhenSysfsUnreadable(t *testing.T) {
	iface := buildInterface("mlx5_9", deviceInfo{}, []qpRecord{{qpn: 5, pid: 1, port: 0}, {qpn: 3, pid: 1, port: 1}})
	if len(iface.Ports) != 1 || len(iface.Ports[0].QueuePairs) != 2 || iface.Ports[0].QueuePairs[0].Qpn != 3 {
		t.Fatalf("interface = %+v, want one synthetic port carrying both QPs", iface)
	}
}

func TestEndpointFingerprintIgnoresTimestamp(t *testing.T) {
	a := testEndpoint("n", "u", "ns", "p", time.Now(),
		v1beta1.RDMAInterface{RdmaDevice: "mlx5_0", Ports: []v1beta1.RDMAPort{{Port: 1, Lid: 3}}})
	b := a
	b.Status.LastUpdateTime = metav1.NewTime(a.Status.LastUpdateTime.Add(time.Hour))
	if endpointFingerprint(a) != endpointFingerprint(b) {
		t.Fatal("fingerprint changed with timestamp only")
	}
	b.Status.Interfaces = []v1beta1.RDMAInterface{{RdmaDevice: "mlx5_0", Ports: []v1beta1.RDMAPort{{Port: 1, Lid: 4}}}}
	if endpointFingerprint(a) == endpointFingerprint(b) {
		t.Fatal("fingerprint ignored a LID change")
	}
}

func TestParseSysfsGIDDropsPrefixOnlyEntries(t *testing.T) {
	prefixOnly := parseSysfsGID("fe80:0000:0000:0000:0000:0000:0000:0000")
	if prefixOnly != "" {
		t.Fatalf("prefix only GID must be dropped, got %q", prefixOnly)
	}
	portGID := parseSysfsGID("fe80:0000:0000:0000:a088:c203:0088:6a44")
	if portGID != "fe80::a088:c203:88:6a44" {
		t.Fatalf("port GID = %q", portGID)
	}
}

func TestDelayUntilAllowedSpacesEventDrivenRuns(t *testing.T) {
	n := &endpointPublisher{minInterval: 3 * time.Second}
	now := time.Now()
	first := n.delayUntilAllowed(now)
	if first != 0 {
		t.Fatalf("first run delay = %v, want 0", first)
	}
	n.lastRun = now
	early := n.delayUntilAllowed(now.Add(time.Second))
	if early != 2*time.Second {
		t.Fatalf("delay after 1s = %v, want 2s", early)
	}
	late := n.delayUntilAllowed(now.Add(5 * time.Second))
	if late != 0 {
		t.Fatalf("delay after 5s = %v, want 0", late)
	}
	var disabled *endpointPublisher
	nilDelay := disabled.delayUntilAllowed(now)
	if nilDelay != 0 {
		t.Fatalf("nil reporter delay = %v, want 0", nilDelay)
	}
}
