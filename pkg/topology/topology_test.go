// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package topology

import (
	"reflect"
	"testing"
)

func TestInferTiersAssignsExpectedHostLeafSpineCoreLevels(t *testing.T) {
	graph := BuildTopologyGraph(testHosts(), testSwitches())

	InferTiers(graph)

	wantTiers := map[string]int{
		"node-a":  0,
		"node-b":  0,
		"leaf-a":  1,
		"leaf-b":  1,
		"spine-a": 2,
		"spine-b": 2,
		"core-a":  3,
	}

	for name, wantTier := range wantTiers {
		device, ok := graph.Devices[name]
		if !ok {
			t.Fatalf("expected device %q to exist in graph", name)
		}
		if device.Tier != wantTier {
			t.Fatalf("expected device %q tier %d, got %d", name, wantTier, device.Tier)
		}
	}
}

func TestDiscoverTopologyBuildsDeterministicConcatGroups(t *testing.T) {
	forward := DiscoverTopology(testHosts(), testSwitches(), WithConcatGroupName())
	reordered := DiscoverTopology(testHostsReordered(), testSwitchesReordered(), WithConcatGroupName())

	if !reflect.DeepEqual(forward.GroupsByTier, reordered.GroupsByTier) {
		t.Fatalf("expected deterministic groups across input order changes, got %#v and %#v", forward.GroupsByTier, reordered.GroupsByTier)
	}
	if !reflect.DeepEqual(forward.ParentIndex, reordered.ParentIndex) {
		t.Fatalf("expected deterministic parent index across input order changes, got %#v and %#v", forward.ParentIndex, reordered.ParentIndex)
	}

	wantTierOne := []TopologyGroup{{
		Name:           "leaf-a-leaf-b-group",
		Tier:           1,
		Members:        []string{"leaf-a", "leaf-b"},
		LowerTierNodes: []string{"node-a", "node-b"},
		UpperTierNodes: []string{"spine-a", "spine-b"},
	}}
	if !reflect.DeepEqual(forward.GroupsByTier[1], wantTierOne) {
		t.Fatalf("unexpected tier 1 groups: %#v", forward.GroupsByTier[1])
	}

	wantTierTwo := []TopologyGroup{{
		Name:           "spine-a-spine-b-group",
		Tier:           2,
		Members:        []string{"spine-a", "spine-b"},
		LowerTierNodes: []string{"leaf-a", "leaf-b"},
		UpperTierNodes: []string{"core-a"},
	}}
	if !reflect.DeepEqual(forward.GroupsByTier[2], wantTierTwo) {
		t.Fatalf("unexpected tier 2 groups: %#v", forward.GroupsByTier[2])
	}

	wantParentChain := []string{"leaf-a-leaf-b-group", "spine-a-spine-b-group", "core-a"}
	if got := forward.QueryParentChain("node-a"); !reflect.DeepEqual(got, wantParentChain) {
		t.Fatalf("expected node-a parent chain %#v, got %#v", wantParentChain, got)
	}
}

func TestDiscoverTopologyUsesHashGroupNames(t *testing.T) {
	topology := DiscoverTopology(testHosts(), testSwitches(), WithHashGroupName(), WithHashLength(10))

	wantLeafGroupName := shortSHA("leaf-a-leaf-b", 10)
	wantSpineGroupName := shortSHA("spine-a-spine-b", 10)

	if len(topology.GroupsByTier[1]) != 1 {
		t.Fatalf("expected 1 tier 1 group, got %#v", topology.GroupsByTier[1])
	}
	if topology.GroupsByTier[1][0].Name != wantLeafGroupName {
		t.Fatalf("expected tier 1 hash group name %q, got %q", wantLeafGroupName, topology.GroupsByTier[1][0].Name)
	}
	if len(topology.GroupsByTier[2]) != 1 {
		t.Fatalf("expected 1 tier 2 group, got %#v", topology.GroupsByTier[2])
	}
	if topology.GroupsByTier[2][0].Name != wantSpineGroupName {
		t.Fatalf("expected tier 2 hash group name %q, got %q", wantSpineGroupName, topology.GroupsByTier[2][0].Name)
	}
	if len(topology.GroupsByTier[1][0].Name) != 10 || len(topology.GroupsByTier[2][0].Name) != 10 {
		t.Fatalf("expected hash group names to use requested length 10, got %#v", topology.GroupsByTier)
	}

	wantParentChain := []string{wantLeafGroupName, wantSpineGroupName, "core-a"}
	if got := topology.QueryParentChain("node-a"); !reflect.DeepEqual(got, wantParentChain) {
		t.Fatalf("expected node-a parent chain %#v, got %#v", wantParentChain, got)
	}
}

func testHosts() []Host {
	return []Host{
		{Name: "node-a"},
		{Name: "node-b"},
	}
}

func testHostsReordered() []Host {
	return []Host{
		{Name: "node-b"},
		{Name: "node-a"},
	}
}

func testSwitches() []Switch {
	return []Switch{
		{
			Name: "leaf-a",
			Neighbors: []LLDPNeighbor{
				{LocalDeviceName: "leaf-a", LocalPort: "Ethernet1", RemoteSystemName: "node-a", RemotePortID: "eth0"},
				{LocalDeviceName: "leaf-a", LocalPort: "Ethernet2", RemoteSystemName: "node-b", RemotePortID: "eth0"},
				{LocalDeviceName: "leaf-a", LocalPort: "Ethernet49", RemoteSystemName: "spine-a", RemotePortID: "Ethernet1"},
				{LocalDeviceName: "leaf-a", LocalPort: "Ethernet50", RemoteSystemName: "spine-b", RemotePortID: "Ethernet1"},
			},
		},
		{
			Name: "leaf-b",
			Neighbors: []LLDPNeighbor{
				{LocalDeviceName: "leaf-b", LocalPort: "Ethernet1", RemoteSystemName: "node-a", RemotePortID: "eth1"},
				{LocalDeviceName: "leaf-b", LocalPort: "Ethernet2", RemoteSystemName: "node-b", RemotePortID: "eth1"},
				{LocalDeviceName: "leaf-b", LocalPort: "Ethernet49", RemoteSystemName: "spine-a", RemotePortID: "Ethernet2"},
				{LocalDeviceName: "leaf-b", LocalPort: "Ethernet50", RemoteSystemName: "spine-b", RemotePortID: "Ethernet2"},
			},
		},
		{
			Name: "spine-a",
			Neighbors: []LLDPNeighbor{
				{LocalDeviceName: "spine-a", LocalPort: "Ethernet49", RemoteSystemName: "core-a", RemotePortID: "Ethernet1"},
			},
		},
		{
			Name: "spine-b",
			Neighbors: []LLDPNeighbor{
				{LocalDeviceName: "spine-b", LocalPort: "Ethernet49", RemoteSystemName: "core-a", RemotePortID: "Ethernet2"},
			},
		},
		{Name: "core-a"},
	}
}

func testSwitchesReordered() []Switch {
	return []Switch{
		{Name: "core-a"},
		{
			Name: "spine-b",
			Neighbors: []LLDPNeighbor{
				{LocalDeviceName: "spine-b", LocalPort: "Ethernet49", RemoteSystemName: "core-a", RemotePortID: "Ethernet2"},
			},
		},
		{
			Name: "spine-a",
			Neighbors: []LLDPNeighbor{
				{LocalDeviceName: "spine-a", LocalPort: "Ethernet49", RemoteSystemName: "core-a", RemotePortID: "Ethernet1"},
			},
		},
		{
			Name: "leaf-b",
			Neighbors: []LLDPNeighbor{
				{LocalDeviceName: "leaf-b", LocalPort: "Ethernet50", RemoteSystemName: "spine-b", RemotePortID: "Ethernet2"},
				{LocalDeviceName: "leaf-b", LocalPort: "Ethernet49", RemoteSystemName: "spine-a", RemotePortID: "Ethernet2"},
				{LocalDeviceName: "leaf-b", LocalPort: "Ethernet2", RemoteSystemName: "node-b", RemotePortID: "eth1"},
				{LocalDeviceName: "leaf-b", LocalPort: "Ethernet1", RemoteSystemName: "node-a", RemotePortID: "eth1"},
			},
		},
		{
			Name: "leaf-a",
			Neighbors: []LLDPNeighbor{
				{LocalDeviceName: "leaf-a", LocalPort: "Ethernet50", RemoteSystemName: "spine-b", RemotePortID: "Ethernet1"},
				{LocalDeviceName: "leaf-a", LocalPort: "Ethernet49", RemoteSystemName: "spine-a", RemotePortID: "Ethernet1"},
				{LocalDeviceName: "leaf-a", LocalPort: "Ethernet2", RemoteSystemName: "node-b", RemotePortID: "eth0"},
				{LocalDeviceName: "leaf-a", LocalPort: "Ethernet1", RemoteSystemName: "node-a", RemotePortID: "eth0"},
			},
		},
	}
}
