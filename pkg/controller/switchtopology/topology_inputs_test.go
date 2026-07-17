// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchtopology

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
)

func TestBuildTopologyInputsForRoleIgnoresSwitchReportedHostNeighbors(t *testing.T) {
	inputs := mustBuildTopologyInputsForRole(t,
		v1beta1.SwitchRoleScaleOut,
		[]v1beta1.FabricNode{{ObjectMeta: metav1.ObjectMeta{Name: "node-gpu-1"}}},
		[]v1beta1.Switch{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "leaf1"},
				Spec:       v1beta1.SwitchSpec{Role: v1beta1.SwitchRoleScaleOut},
				Status: v1beta1.SwitchStatus{
					Healthy: true,
					LLDPNeighbors: []v1beta1.SwitchNeighbor{
						{
							RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeKubernetesNode,
							RemoteSystemName: "node-gpu-1",
						},
					},
				},
			},
		},
	)

	if len(inputs.hosts) != 0 {
		t.Fatalf("expected no participating hosts without FabricNode LLDP data, got %#v", inputs.hosts)
	}
	if len(inputs.switches) != 1 {
		t.Fatalf("expected one switch input, got %#v", inputs.switches)
	}
	if len(inputs.switches[0].Neighbors) != 0 {
		t.Fatalf("expected switch-reported host neighbors to be ignored, got %#v", inputs.switches[0].Neighbors)
	}
}

func TestBuildTopologyInputsUsesUniqueSwitchNeighbors(t *testing.T) {
	inputs := mustBuildTopologyInputsForRole(t,
		v1beta1.SwitchRoleScaleOut,
		nil,
		[]v1beta1.Switch{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "leaf1"},
				Status: v1beta1.SwitchStatus{
					Healthy: false,
					LLDPNeighbors: []v1beta1.SwitchNeighbor{
						{
							RemoteSystemName: "spine1",
							RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeSwitch,
						},
						{
							RemoteSystemName: "spine1",
							RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeSwitch,
						},
					},
				},
			},
		},
	)

	if len(inputs.switches) != 1 {
		t.Fatalf("expected 1 topology switch input, got %d", len(inputs.switches))
	}
	if len(inputs.switches[0].Neighbors) != 1 {
		t.Fatalf("expected 1 topology neighbor, got %#v", inputs.switches[0].Neighbors)
	}
	if inputs.switches[0].Neighbors[0].RemoteSystemName != "spine1" {
		t.Fatalf("unexpected topology neighbor: %#v", inputs.switches[0].Neighbors[0])
	}
}

func TestBuildTopologyInputsWithoutSwitchCRUsesFabricNodeLLDP(t *testing.T) {
	inputs := mustBuildTopologyInputsForRole(t,
		v1beta1.SwitchRoleScaleOut,
		[]v1beta1.FabricNode{
			{ObjectMeta: metav1.ObjectMeta{Name: "node1"}, Status: v1beta1.FabricNodeStatus{ScaleOutNics: []v1beta1.NicInfo{
				{State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "Leaf-01", Port: "Ethernet1"}},
			}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "node2"}, Status: v1beta1.FabricNodeStatus{ScaleOutNics: []v1beta1.NicInfo{
				{State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "leaf-01", Port: "Ethernet2"}},
			}}},
		},
		nil,
	)

	if !inputs.nodeOnly {
		t.Fatal("expected node-only topology inputs without Switch CRs")
	}
	if len(inputs.hosts) != 2 {
		t.Fatalf("expected 2 participating hosts, got %#v", inputs.hosts)
	}
	if len(inputs.switches) != 1 || inputs.switches[0].Name != "leaf-01" {
		t.Fatalf("expected one synthetic leaf input, got %#v", inputs.switches)
	}
	if len(inputs.switches[0].Neighbors) != 2 {
		t.Fatalf("expected both FabricNode LLDP links, got %#v", inputs.switches[0].Neighbors)
	}
}

func TestBuildTopologyInputsOtherRoleSwitchKeepsNodeOnlyMode(t *testing.T) {
	inputs := mustBuildTopologyInputsForRole(t,
		v1beta1.SwitchRoleScaleOut,
		[]v1beta1.FabricNode{{ObjectMeta: metav1.ObjectMeta{Name: "node1"}, Status: v1beta1.FabricNodeStatus{ScaleOutNics: []v1beta1.NicInfo{
			{State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "leaf1", Port: "Ethernet1"}},
		}}}},
		[]v1beta1.Switch{{ObjectMeta: metav1.ObjectMeta{Name: "storage1"}, Spec: v1beta1.SwitchSpec{Role: v1beta1.SwitchRoleStorage}}},
	)

	if !inputs.nodeOnly {
		t.Fatal("expected a Switch from another role to preserve node-only mode")
	}
	if len(inputs.hosts) != 1 || len(inputs.switches) != 1 || inputs.switches[0].Name != "leaf1" {
		t.Fatalf("unexpected node-only scale-out inputs: %#v", inputs)
	}
}

func TestBuildTopologyInputsUsesManualSwitchNeighbors(t *testing.T) {
	inputs := mustBuildTopologyInputsForRole(t,
		v1beta1.SwitchRoleScaleOut,
		[]v1beta1.FabricNode{{ObjectMeta: metav1.ObjectMeta{Name: "node1"}, Status: v1beta1.FabricNodeStatus{ScaleOutNics: []v1beta1.NicInfo{
			{State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "leaf1", Port: "Ethernet1"}},
		}}}},
		[]v1beta1.Switch{
			{ObjectMeta: metav1.ObjectMeta{
				Name:        "spine1",
				Annotations: map[string]string{v1beta1.SwitchNeighborsAnnotation: `["leaf1", "leaf1"]`},
			}, Spec: v1beta1.SwitchSpec{Role: v1beta1.SwitchRoleScaleOut}},
		},
	)

	if inputs.nodeOnly {
		t.Fatal("manual Switch resources unexpectedly produced node-only inputs")
	}
	if !inputs.syntheticSwitches["leaf1"] {
		t.Fatalf("node-discovered leaf was not marked synthetic: %#v", inputs.syntheticSwitches)
	}
	if len(inputs.hosts) != 1 || inputs.hosts[0].Name != "node1" {
		t.Fatalf("hosts = %#v", inputs.hosts)
	}
	if len(inputs.switches) != 2 {
		t.Fatalf("switches = %#v", inputs.switches)
	}
	if len(inputs.switches[0].Neighbors) != 1 || inputs.switches[0].Neighbors[0].RemoteSystemName != "node1" {
		t.Fatalf("leaf neighbors = %#v", inputs.switches[0].Neighbors)
	}
	if len(inputs.switches[1].Neighbors) != 1 || inputs.switches[1].Neighbors[0].RemoteSystemName != "leaf1" {
		t.Fatalf("spine neighbors = %#v", inputs.switches[1].Neighbors)
	}
}

func TestBuildTopologyInputsRejectsInvalidManualSwitchNeighbors(t *testing.T) {
	tests := []struct {
		name        string
		switchName  string
		annotation  string
		otherSwitch string
	}{
		{name: "invalid JSON", switchName: "leaf1", annotation: `leaf2`, otherSwitch: "leaf2"},
		{name: "unknown switch", switchName: "leaf1", annotation: `["missing"]`, otherSwitch: "leaf2"},
		{name: "self reference", switchName: "leaf1", annotation: `["leaf1"]`, otherSwitch: "leaf2"},
		{name: "empty name", switchName: "leaf1", annotation: `[""]`, otherSwitch: "leaf2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildTopologyInputsForRole(v1beta1.SwitchRoleScaleOut, nil, []v1beta1.Switch{
				{ObjectMeta: metav1.ObjectMeta{
					Name:        tt.switchName,
					Annotations: map[string]string{v1beta1.SwitchNeighborsAnnotation: tt.annotation},
				}},
				{ObjectMeta: metav1.ObjectMeta{
					Name:        tt.otherSwitch,
					Annotations: map[string]string{v1beta1.SwitchNeighborsAnnotation: `[]`},
				}},
			})
			if err == nil {
				t.Fatal("expected invalid manual neighbors to fail")
			}
		})
	}
}

func TestBuildTopologyInputsAutomaticModeIgnoresAllManualNeighbors(t *testing.T) {
	inputs := mustBuildTopologyInputsForRole(t,
		v1beta1.SwitchRoleScaleOut,
		[]v1beta1.FabricNode{{ObjectMeta: metav1.ObjectMeta{Name: "node1"}, Status: v1beta1.FabricNodeStatus{ScaleOutNics: []v1beta1.NicInfo{
			{State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "leaf1", Port: "Ethernet1"}},
		}}}},
		[]v1beta1.Switch{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "leaf1"},
				Status: v1beta1.SwitchStatus{LLDPNeighbors: []v1beta1.SwitchNeighbor{{
					RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeSwitch,
					RemoteSystemName: "spine1",
				}}},
			},
			{ObjectMeta: metav1.ObjectMeta{
				Name:        "spine1",
				Annotations: map[string]string{v1beta1.SwitchNeighborsAnnotation: `not-json`},
			}},
		},
	)

	if len(inputs.syntheticSwitches) != 0 {
		t.Fatalf("automatic mode created synthetic Switches: %#v", inputs.syntheticSwitches)
	}
	if len(inputs.switches) != 2 {
		t.Fatalf("switches = %#v", inputs.switches)
	}
	if len(inputs.switches[0].Neighbors) != 2 {
		t.Fatalf("automatic LLDP neighbors were not used: %#v", inputs.switches[0].Neighbors)
	}
}

func TestBuildTopologyInputsEmptyManualAnnotationEnablesSemiAutomaticMode(t *testing.T) {
	inputs := mustBuildTopologyInputsForRole(t,
		v1beta1.SwitchRoleScaleOut,
		[]v1beta1.FabricNode{{ObjectMeta: metav1.ObjectMeta{Name: "node1"}, Status: v1beta1.FabricNodeStatus{ScaleOutNics: []v1beta1.NicInfo{
			{State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "leaf1", Port: "Ethernet1"}},
		}}}},
		[]v1beta1.Switch{{ObjectMeta: metav1.ObjectMeta{
			Name:        "spine1",
			Annotations: map[string]string{v1beta1.SwitchNeighborsAnnotation: ""},
		}, Status: v1beta1.SwitchStatus{LLDPNeighbors: []v1beta1.SwitchNeighbor{{
			RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeSwitch,
			RemoteSystemName: "ignored",
		}}}}},
	)

	if !inputs.syntheticSwitches["leaf1"] {
		t.Fatalf("empty annotation key did not enable semi-automatic mode: %#v", inputs.syntheticSwitches)
	}
	for _, sw := range inputs.switches {
		if sw.Name == "spine1" && len(sw.Neighbors) != 0 {
			t.Fatalf("semi-automatic mode used Switch status neighbors: %#v", sw.Neighbors)
		}
	}
}

func TestBuildTopologyInputsStorageUsesNodeDiscoveredLeafInSemiAutomaticMode(t *testing.T) {
	inputs := mustBuildTopologyInputsForRole(t,
		v1beta1.SwitchRoleStorage,
		[]v1beta1.FabricNode{{ObjectMeta: metav1.ObjectMeta{Name: "storage-node1"}, Status: v1beta1.FabricNodeStatus{
			NodeRole: v1beta1.NodeRoleStorage,
			StorageNics: []v1beta1.NicInfo{{
				State:        "up",
				LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "storage-leaf1", Port: "Ethernet1"},
			}},
		}}},
		[]v1beta1.Switch{{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "storage-spine1",
				Annotations: map[string]string{v1beta1.SwitchNeighborsAnnotation: `["storage-leaf1"]`},
			},
			Spec: v1beta1.SwitchSpec{Role: v1beta1.SwitchRoleStorage},
		}},
	)

	if inputs.nodeOnly {
		t.Fatal("Storage Switch resources unexpectedly produced node-only inputs")
	}
	if !inputs.syntheticSwitches["storage-leaf1"] {
		t.Fatalf("Storage node-discovered leaf was not marked synthetic: %#v", inputs.syntheticSwitches)
	}
	if len(inputs.hosts) != 1 || inputs.hosts[0].Name != "storage-node1" {
		t.Fatalf("hosts = %#v", inputs.hosts)
	}
	if len(inputs.switches) != 2 {
		t.Fatalf("switches = %#v", inputs.switches)
	}
}

func mustBuildTopologyInputsForRole(t *testing.T, role v1beta1.SwitchRole, fabricNodes []v1beta1.FabricNode, switches []v1beta1.Switch) topologyInputs {
	t.Helper()
	inputs, err := buildTopologyInputsForRole(role, fabricNodes, switches)
	if err != nil {
		t.Fatal(err)
	}
	return inputs
}
