// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchtopology

import (
	"context"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/logger"
	"github.com/unifabric-io/unifabric/pkg/switchagent"
)

func TestTopologyGroupLabelValueAddsTierPrefixInHashMode(t *testing.T) {
	naming := config.TopologyGroupNamingConfig{LabelValueFormat: "hash", HashLength: 7}

	leafValue := topologyGroupLabelValue(naming, topologyTierLeaf, []string{"leaf2", "leaf1"})
	if !strings.HasPrefix(leafValue, "leaf-") {
		t.Fatalf("expected leaf label value to have leaf- prefix, got %s", leafValue)
	}
	if len(leafValue) != len("leaf-")+7 {
		t.Fatalf("expected leaf label value length %d, got %d", len("leaf-")+7, len(leafValue))
	}

	spineValue := topologyGroupLabelValue(naming, topologyTierSpine, []string{"spine1"})
	if !strings.HasPrefix(spineValue, "spine-") {
		t.Fatalf("expected spine label value to have spine- prefix, got %s", spineValue)
	}

	coreValue := topologyGroupLabelValue(naming, topologyTierCore, []string{"core1"})
	if !strings.HasPrefix(coreValue, "core-") {
		t.Fatalf("expected core label value to have core- prefix, got %s", coreValue)
	}
}

func TestBuildDesiredStateProjectsTopologyGroupsAndNodeLabels(t *testing.T) {
	cfg := &config.ControllerConfig{
		TopologyLabels: config.TopologyLabelsConfig{
			ScaleOutLeaf:  config.DefaultLabelScaleOutLeaf,
			ScaleOutSpine: config.DefaultLabelScaleOutSpine,
			ScaleOutCore:  config.DefaultLabelScaleOutCore,
		},
		ScaleOutDiscovery: config.ScaleOutDiscoveryConfig{
			Switches: config.ScaleOutSwitchesConfig{
				GroupNaming: config.TopologyGroupNamingConfig{
					LabelValueFormat: "name",
					HashLength:       7,
				},
			},
		},
	}

	fabricNodes := []v1beta1.FabricNode{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node1"},
			Status: v1beta1.FabricNodeStatus{
				NodeRole: v1beta1.NodeRoleGPU,
				ScaleOutNics: []v1beta1.NicInfo{
					{Name: "eth0", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "leaf1", Port: "Ethernet1"}},
					{Name: "eth1", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "leaf2", Port: "Ethernet1"}},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node2"},
			Status: v1beta1.FabricNodeStatus{
				NodeRole: v1beta1.NodeRoleGPU,
				ScaleOutNics: []v1beta1.NicInfo{
					{Name: "eth0", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "leaf1", Port: "Ethernet2"}},
					{Name: "eth1", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "leaf2", Port: "Ethernet2"}},
				},
			},
		},
	}

	switches := []v1beta1.Switch{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "leaf1"},
			Status: v1beta1.SwitchStatus{
				Healthy: true,
				LLDPNeighbors: []v1beta1.SwitchNeighbor{
					{RemoteSystemName: "node1"},
					{RemoteSystemName: "node2"},
					{RemoteSystemName: "spine1", RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeSwitch},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "leaf2"},
			Status: v1beta1.SwitchStatus{
				Healthy: true,
				LLDPNeighbors: []v1beta1.SwitchNeighbor{
					{RemoteSystemName: "node1"},
					{RemoteSystemName: "node2"},
					{RemoteSystemName: "spine1", RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeSwitch},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "spine1"},
			Status: v1beta1.SwitchStatus{
				Healthy: true,
				LLDPNeighbors: []v1beta1.SwitchNeighbor{
					{RemoteSystemName: "leaf1", RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeSwitch},
					{RemoteSystemName: "leaf2", RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeSwitch},
				},
			},
		},
	}

	desiredGroups, desiredNodeLabels, managedNodes := buildDesiredState(cfg, fabricNodes, switches)
	if len(managedNodes) != 2 {
		t.Fatalf("expected 2 managed nodes, got %d", len(managedNodes))
	}
	if len(desiredGroups) != 2 {
		t.Fatalf("expected 2 topology groups, got %d", len(desiredGroups))
	}

	leafFound := false
	spineFound := false
	for _, group := range desiredGroups {
		if group.Role != v1beta1.SwitchRoleScaleOut {
			t.Fatalf("expected default topology group role ScaleOut, got %s", group.Role)
		}
		switch group.Tier {
		case topologyTierLeaf:
			leafFound = true
			if group.LabelValue != "leaf1-leaf2-group" {
				t.Fatalf("expected leaf label value leaf1-leaf2-group, got %s", group.LabelValue)
			}
			if len(group.Nodes) != 2 || group.Nodes[0] != "node1" || group.Nodes[1] != "node2" {
				t.Fatalf("unexpected leaf group nodes: %#v", group.Nodes)
			}
		case topologyTierSpine:
			spineFound = true
			if group.LabelValue != "spine1" {
				t.Fatalf("expected spine label value spine1, got %s", group.LabelValue)
			}
		}
	}
	if !leafFound || !spineFound {
		t.Fatalf("expected both leaf and spine topology groups, got %#v", desiredGroups)
	}

	node1Labels := desiredNodeLabels["node1"]
	if node1Labels.Leaf != "leaf1-leaf2-group" {
		t.Fatalf("expected node1 leaf label leaf1-leaf2-group, got %s", node1Labels.Leaf)
	}
	if node1Labels.Spine != "spine1" {
		t.Fatalf("expected node1 spine label spine1, got %s", node1Labels.Spine)
	}
	if node1Labels.Core != "" {
		t.Fatalf("expected empty core label, got %s", node1Labels.Core)
	}
}

func TestBuildDesiredStateUsesReportedSwitchNamesAsAliases(t *testing.T) {
	cfg := &config.ControllerConfig{
		TopologyLabels: config.TopologyLabelsConfig{
			ScaleOutLeaf:  config.DefaultLabelScaleOutLeaf,
			ScaleOutSpine: config.DefaultLabelScaleOutSpine,
			ScaleOutCore:  config.DefaultLabelScaleOutCore,
		},
		ScaleOutDiscovery: config.ScaleOutDiscoveryConfig{
			Switches: config.ScaleOutSwitchesConfig{
				GroupNaming: config.TopologyGroupNamingConfig{
					LabelValueFormat: "name",
					HashLength:       7,
				},
			},
		},
	}

	fabricNodes := []v1beta1.FabricNode{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node1"},
			Status: v1beta1.FabricNodeStatus{
				NodeRole: v1beta1.NodeRoleGPU,
				ScaleOutNics: []v1beta1.NicInfo{
					{Name: "eth0", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "leaf1", Port: "Ethernet1"}},
				},
			},
		},
	}

	switches := []v1beta1.Switch{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "rack-a-leaf1"},
			Status: v1beta1.SwitchStatus{
				Hostname: "leaf1",
				Healthy:  true,
				LLDPNeighbors: []v1beta1.SwitchNeighbor{
					{RemoteSystemName: "node1"},
					{RemoteSystemName: "spine", RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeSwitch},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "core-spine-a"},
			Status: v1beta1.SwitchStatus{
				Hostname: "spine",
				Healthy:  true,
				LLDPNeighbors: []v1beta1.SwitchNeighbor{
					{RemoteSystemName: "leaf1", RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeSwitch},
				},
			},
		},
	}

	desiredGroups, nodeLabelsByName, managedNodes := buildDesiredState(cfg, fabricNodes, switches)
	if len(managedNodes) != 1 || managedNodes[0] != "node1" {
		t.Fatalf("unexpected managed nodes: %#v", managedNodes)
	}
	if len(desiredGroups) != 2 {
		t.Fatalf("expected 2 topology groups, got %d", len(desiredGroups))
	}
	for _, group := range desiredGroups {
		if group.Role != v1beta1.SwitchRoleScaleOut {
			t.Fatalf("expected alias-based topology groups to default to ScaleOut, got %s", group.Role)
		}
	}

	node1Labels := nodeLabelsByName["node1"]
	if node1Labels.Leaf != "rack-a-leaf1" {
		t.Fatalf("expected node1 leaf label rack-a-leaf1, got %s", node1Labels.Leaf)
	}
	if node1Labels.Spine != "core-spine-a" {
		t.Fatalf("expected node1 spine label core-spine-a, got %s", node1Labels.Spine)
	}
}

func TestBuildDesiredStatePartitionsSwitchesByRole(t *testing.T) {
	cfg := &config.ControllerConfig{
		TopologyLabels: config.TopologyLabelsConfig{
			ScaleOutLeaf:  config.DefaultLabelScaleOutLeaf,
			ScaleOutSpine: config.DefaultLabelScaleOutSpine,
			ScaleOutCore:  config.DefaultLabelScaleOutCore,
		},
		ScaleOutDiscovery: config.ScaleOutDiscoveryConfig{
			Switches: config.ScaleOutSwitchesConfig{
				GroupNaming: config.TopologyGroupNamingConfig{
					LabelValueFormat: "name",
					HashLength:       7,
				},
			},
		},
		NodeTopologyDiscovery: config.ControllerNodeTopologyConfig{
			StorageInterfaceSelector: "interface=eth10",
			ScaleUpInterfaceSelector: "interface=eth8",
		},
	}

	fabricNodes := []v1beta1.FabricNode{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node-gpu-1"},
			Status: v1beta1.FabricNodeStatus{
				NodeRole: v1beta1.NodeRoleGPU,
				ScaleOutNics: []v1beta1.NicInfo{
					{Name: "eth0", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "scaleout-leaf1", Port: "Ethernet1"}},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node-storage-1"},
			Status: v1beta1.FabricNodeStatus{
				NodeRole: v1beta1.NodeRoleStorage,
				StorageNics: []v1beta1.NicInfo{
					{Name: "eth10", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "storage-leaf1", Port: "Ethernet7"}},
				},
			},
		},
	}

	switches := []v1beta1.Switch{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "scaleout-leaf1"},
			Spec:       v1beta1.SwitchSpec{Role: v1beta1.SwitchRoleScaleOut},
			Status: v1beta1.SwitchStatus{
				Healthy: true,
				LLDPNeighbors: []v1beta1.SwitchNeighbor{
					{RemoteSystemName: "node-gpu-1", RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeKubernetesNode},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "storage-leaf1"},
			Spec:       v1beta1.SwitchSpec{Role: v1beta1.SwitchRoleStorage},
			Status: v1beta1.SwitchStatus{
				Healthy: true,
				LLDPNeighbors: []v1beta1.SwitchNeighbor{
					{RemoteSystemName: "node-storage-1", RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeKubernetesNode},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "scaleup-leaf1"},
			Spec:       v1beta1.SwitchSpec{Role: v1beta1.SwitchRoleScaleUp},
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
	}

	desiredGroups, nodeLabelsByName, managedNodes := buildDesiredState(cfg, fabricNodes, switches)
	if len(managedNodes) != 2 {
		t.Fatalf("expected 2 managed nodes, got %d", len(managedNodes))
	}
	if len(desiredGroups) != 2 {
		t.Fatalf("expected 2 role-partitioned topology groups, got %d", len(desiredGroups))
	}

	roleToGroup := map[v1beta1.SwitchRole]desiredTopologyGroup{}
	for _, group := range desiredGroups {
		roleToGroup[group.Role] = group
	}

	scaleOutGroup, ok := roleToGroup[v1beta1.SwitchRoleScaleOut]
	if !ok {
		t.Fatalf("expected a ScaleOut topology group, got %#v", desiredGroups)
	}
	if len(scaleOutGroup.Switches) != 1 || scaleOutGroup.Switches[0] != "scaleout-leaf1" {
		t.Fatalf("unexpected ScaleOut topology group members: %#v", scaleOutGroup.Switches)
	}
	if len(scaleOutGroup.Nodes) != 1 || scaleOutGroup.Nodes[0] != "node-gpu-1" {
		t.Fatalf("unexpected ScaleOut topology group nodes: %#v", scaleOutGroup.Nodes)
	}

	storageGroup, ok := roleToGroup[v1beta1.SwitchRoleStorage]
	if !ok {
		t.Fatalf("expected a Storage topology group, got %#v", desiredGroups)
	}
	if len(storageGroup.Switches) != 1 || storageGroup.Switches[0] != "storage-leaf1" {
		t.Fatalf("unexpected Storage topology group members: %#v", storageGroup.Switches)
	}
	if len(storageGroup.Nodes) != 1 || storageGroup.Nodes[0] != "node-storage-1" {
		t.Fatalf("unexpected Storage topology group nodes: %#v", storageGroup.Nodes)
	}

	if _, ok := roleToGroup[v1beta1.SwitchRoleScaleUp]; ok {
		t.Fatalf("expected ScaleUp to be deferred in this PR, got %#v", desiredGroups)
	}

	if nodeLabelsByName["node-gpu-1"].Leaf != "scaleout-leaf1" {
		t.Fatalf("expected ScaleOut node label to be written from ScaleOut role, got %#v", nodeLabelsByName["node-gpu-1"])
	}
	if storageLabels := nodeLabelsByName["node-storage-1"]; storageLabels != (desiredNodeLabels{}) {
		t.Fatalf("expected Storage role to avoid scale-out node labels, got %#v", storageLabels)
	}
}

func TestBuildTopologyInputsForRoleIgnoresSwitchReportedHostNeighbors(t *testing.T) {
	inputs := buildTopologyInputsForRole(
		v1beta1.SwitchRoleScaleOut,
		config.ControllerNodeTopologyConfig{},
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

func TestReconcileUpdatesManagedSwitchCountMetric(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}

	switchCountMetric.Set(0)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&v1beta1.Switch{ObjectMeta: metav1.ObjectMeta{Name: "leaf1"}, Status: v1beta1.SwitchStatus{Healthy: true}},
			&v1beta1.Switch{ObjectMeta: metav1.ObjectMeta{Name: "leaf2"}, Status: v1beta1.SwitchStatus{Healthy: false}},
		).
		Build()

	reconciler := &Reconciler{
		client: fakeClient,
		cfg:    &config.ControllerConfig{},
		log:    logger.MustNew(logger.LevelDebug),
	}

	if _, err := reconciler.Reconcile(context.Background(), reconcile.Request{}); err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}

	if got := testutil.ToFloat64(switchCountMetric); got != 1 {
		t.Fatalf("expected switch count metric 1, got %v", got)
	}
}

func TestHandleSnapshotUpdatesAndInvalidatesSwitchStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}

	switchObj := &v1beta1.Switch{
		ObjectMeta: metav1.ObjectMeta{Name: "leaf1"},
		Spec:       v1beta1.SwitchSpec{MgmtIP: "10.0.0.10"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(switchObj).
		WithStatusSubresource(&v1beta1.Switch{}).
		Build()

	manager := &subscriptionManager{
		client: fakeClient,
		cfg:    &config.ControllerConfig{},
		log:    logger.MustNew(logger.LevelDebug),
	}
	parseSuccessBefore := testutil.ToFloat64(switchLLDPParseSuccessTotal)

	var lastGeneration uint64
	updated, err := manager.handleSnapshot(context.Background(), "leaf1", &switchagent.LLDPNeighborSnapshot{
		SwitchName: "leaf",
		Generation: 1,
		LldpNeighbors: []*switchagent.LLDPNeighbor{
			{LocalDeviceName: "leaf", LocalPort: "Ethernet1", RemoteSystemName: "node1", RemotePortId: "eth0"},
		},
	}, &lastGeneration)
	if err != nil {
		t.Fatalf("handleSnapshot returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected first snapshot to update switch status")
	}
	if parseSuccessAfter := testutil.ToFloat64(switchLLDPParseSuccessTotal); parseSuccessAfter != parseSuccessBefore+1 {
		t.Fatalf("expected lldp parse success counter to increment by 1, got before=%v after=%v", parseSuccessBefore, parseSuccessAfter)
	}
	if lastGeneration != 1 {
		t.Fatalf("expected last generation 1, got %d", lastGeneration)
	}

	stored := &v1beta1.Switch{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "leaf1"}, stored); err != nil {
		t.Fatalf("failed to fetch updated switch: %v", err)
	}
	if !stored.Status.Healthy {
		t.Fatal("expected switch to be healthy after accepted snapshot")
	}
	if stored.Status.Hostname != "leaf" {
		t.Fatalf("expected hostname leaf, got %q", stored.Status.Hostname)
	}
	if stored.Status.LLDPNeighborCount != 1 {
		t.Fatalf("expected lldpNeighborCount 1, got %d", stored.Status.LLDPNeighborCount)
	}
	if len(stored.Status.LLDPNeighbors) != 1 {
		t.Fatalf("expected 1 stored neighbor, got %#v", stored.Status.LLDPNeighbors)
	}
	if stored.Status.LLDPNeighbors[0].RemoteSystemName != "node1" {
		t.Fatalf("expected stored remote system name node1, got %#v", stored.Status.LLDPNeighbors[0])
	}
	if len(stored.Status.Conditions) != 2 {
		t.Fatalf("expected 2 switch conditions, got %d", len(stored.Status.Conditions))
	}

	updated, err = manager.handleSnapshot(context.Background(), "leaf1", &switchagent.LLDPNeighborSnapshot{
		SwitchName: "leaf",
		Generation: 1,
		LldpNeighbors: []*switchagent.LLDPNeighbor{
			{LocalDeviceName: "leaf1", LocalPort: "Ethernet1", RemoteSystemName: "", RemotePortId: "eth0"},
		},
	}, &lastGeneration)
	if err != nil {
		t.Fatalf("duplicate snapshot returned error: %v", err)
	}
	if updated {
		t.Fatal("expected duplicate generation snapshot to be ignored")
	}

	if err := manager.markSwitchDisconnected(context.Background(), "leaf1", v1beta1.SwitchReasonDialFailed, "dial failed"); err != nil {
		t.Fatalf("markSwitchDisconnected returned error: %v", err)
	}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "leaf1"}, stored); err != nil {
		t.Fatalf("failed to fetch disconnected switch: %v", err)
	}
	if stored.Status.Healthy {
		t.Fatal("expected switch to be unhealthy after disconnect")
	}
}

func TestHandleSnapshotMalformedEntryIncrementsLLDPParseFailureMetric(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}

	switchObj := &v1beta1.Switch{
		ObjectMeta: metav1.ObjectMeta{Name: "leaf1"},
		Spec:       v1beta1.SwitchSpec{MgmtIP: "10.0.0.10"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(switchObj).
		WithStatusSubresource(&v1beta1.Switch{}).
		Build()

	manager := &subscriptionManager{
		client: fakeClient,
		cfg:    &config.ControllerConfig{},
		log:    logger.MustNew(logger.LevelDebug),
	}
	parseFailureBefore := testutil.ToFloat64(switchLLDPParseFailureTotal)

	var lastGeneration uint64
	updated, err := manager.handleSnapshot(context.Background(), "leaf1", &switchagent.LLDPNeighborSnapshot{
		SwitchName: "leaf1",
		Generation: 1,
		LldpNeighbors: []*switchagent.LLDPNeighbor{
			{LocalDeviceName: "leaf1", LocalPort: "Ethernet1", RemoteSystemName: "", RemotePortId: "eth0"},
		},
	}, &lastGeneration)
	if err == nil {
		t.Fatal("expected malformed snapshot to return error")
	}
	if updated {
		t.Fatal("expected malformed snapshot not to update switch status")
	}
	if parseFailureAfter := testutil.ToFloat64(switchLLDPParseFailureTotal); parseFailureAfter != parseFailureBefore+1 {
		t.Fatalf("expected lldp parse failure counter to increment by 1, got before=%v after=%v", parseFailureBefore, parseFailureAfter)
	}
}

func TestHandleSnapshotIgnoresConfiguredSwitchPorts(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}

	switchObj := &v1beta1.Switch{
		ObjectMeta: metav1.ObjectMeta{Name: "leaf1"},
		Spec:       v1beta1.SwitchSpec{MgmtIP: "10.0.0.10"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(switchObj).
		WithStatusSubresource(&v1beta1.Switch{}).
		Build()

	manager := &subscriptionManager{
		client: fakeClient,
		cfg: &config.ControllerConfig{
			ScaleOutDiscovery: config.ScaleOutDiscoveryConfig{
				Switches: config.ScaleOutSwitchesConfig{
					IgnoreSwitchPorts: []string{"mgmt*", "Management*", "oob*"},
				},
			},
		},
		log: logger.MustNew(logger.LevelDebug),
	}

	var lastGeneration uint64
	updated, err := manager.handleSnapshot(context.Background(), "leaf1", &switchagent.LLDPNeighborSnapshot{
		SwitchName: "leaf",
		Generation: 1,
		LldpNeighbors: []*switchagent.LLDPNeighbor{
			{LocalDeviceName: "leaf", LocalPort: "Ethernet1", RemoteSystemName: "node1", RemotePortId: "eth0"},
			{LocalDeviceName: "leaf", LocalPort: "mgmt0", RemoteSystemName: "oob-switch", RemotePortId: "Management1"},
		},
	}, &lastGeneration)
	if err != nil {
		t.Fatalf("handleSnapshot returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected snapshot to update switch status")
	}

	stored := &v1beta1.Switch{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "leaf1"}, stored); err != nil {
		t.Fatalf("failed to fetch updated switch: %v", err)
	}
	if stored.Status.LLDPNeighborCount != 1 {
		t.Fatalf("expected lldpNeighborCount 1 after controller-side filtering, got %d", stored.Status.LLDPNeighborCount)
	}
	if len(stored.Status.LLDPNeighbors) != 1 {
		t.Fatalf("expected 1 stored neighbor after controller-side filtering, got %d", len(stored.Status.LLDPNeighbors))
	}
	if stored.Status.LLDPNeighbors[0].RemoteSystemName != "node1" {
		t.Fatalf("expected remaining neighbor node1, got %#v", stored.Status.LLDPNeighbors[0])
	}
}

func TestHandleSnapshotStoresMultipleLinksToSamePeer(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}

	switchObj := &v1beta1.Switch{
		ObjectMeta: metav1.ObjectMeta{Name: "leaf1"},
		Spec:       v1beta1.SwitchSpec{MgmtIP: "10.0.0.10"},
	}
	nodeObj := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-gpu-1"}}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(switchObj, nodeObj).
		WithStatusSubresource(&v1beta1.Switch{}).
		Build()

	manager := &subscriptionManager{
		client: fakeClient,
		cfg:    &config.ControllerConfig{},
		log:    logger.MustNew(logger.LevelDebug),
	}

	var lastGeneration uint64
	updated, err := manager.handleSnapshot(context.Background(), "leaf1", &switchagent.LLDPNeighborSnapshot{
		SwitchName: "leaf1",
		Generation: 1,
		LldpNeighbors: []*switchagent.LLDPNeighbor{
			{LocalDeviceName: "leaf1", LocalPort: "Ethernet1", RemoteSystemName: "node-gpu-1", RemotePortId: "eth0"},
			{LocalDeviceName: "leaf1", LocalPort: "Ethernet2", RemoteSystemName: "node-gpu-1", RemotePortId: "eth1"},
		},
	}, &lastGeneration)
	if err != nil {
		t.Fatalf("handleSnapshot returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected snapshot to update switch status")
	}

	stored := &v1beta1.Switch{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "leaf1"}, stored); err != nil {
		t.Fatalf("failed to fetch updated switch: %v", err)
	}
	if stored.Status.LLDPNeighborCount != 1 {
		t.Fatalf("expected lldpNeighborCount 1, got %d", stored.Status.LLDPNeighborCount)
	}
	if len(stored.Status.LLDPNeighbors) != 1 {
		t.Fatalf("expected 1 stored neighbor, got %d", len(stored.Status.LLDPNeighbors))
	}
	neighbor := stored.Status.LLDPNeighbors[0]
	if neighbor.RemoteSystemType != v1beta1.SwitchLLDPRemoteSystemTypeKubernetesNode {
		t.Fatalf("expected KubernetesNode remote system type, got %s", neighbor.RemoteSystemType)
	}
	if neighbor.RemoteSystemName != "node-gpu-1" {
		t.Fatalf("unexpected stored remote system name: %s", neighbor.RemoteSystemName)
	}
}

func TestBuildTopologyInputsUsesUniqueSwitchNeighbors(t *testing.T) {
	inputs, _ := buildTopologyInputs(nil, []v1beta1.Switch{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "leaf1"},
			Status: v1beta1.SwitchStatus{
				Healthy: true,
				LLDPNeighbors: []v1beta1.SwitchNeighbor{
					{
						RemoteSystemName: "spine1",
						RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeSwitch,
					},
				},
			},
		},
	})

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

func TestHandleSnapshotClassifiesRemoteSystemType(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add core scheme: %v", err)
	}

	switchObj := &v1beta1.Switch{
		ObjectMeta: metav1.ObjectMeta{Name: "leaf1"},
		Spec:       v1beta1.SwitchSpec{MgmtIP: "10.0.0.10"},
	}
	nodeObj := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-gpu-1"}}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(switchObj, nodeObj).
		WithStatusSubresource(&v1beta1.Switch{}).
		Build()

	manager := &subscriptionManager{
		client: fakeClient,
		cfg:    &config.ControllerConfig{},
		log:    logger.MustNew(logger.LevelDebug),
	}

	var lastGeneration uint64
	updated, err := manager.handleSnapshot(context.Background(), "leaf1", &switchagent.LLDPNeighborSnapshot{
		SwitchName: "leaf1",
		Generation: 1,
		LldpNeighbors: []*switchagent.LLDPNeighbor{
			{LocalDeviceName: "leaf1", LocalPort: "Ethernet1", RemoteSystemName: "node-gpu-1", RemotePortId: "eth0"},
			{LocalDeviceName: "leaf1", LocalPort: "Ethernet49", RemoteSystemName: "spine1", RemotePortId: "Ethernet1"},
		},
	}, &lastGeneration)
	if err != nil {
		t.Fatalf("handleSnapshot returned error: %v", err)
	}
	if !updated {
		t.Fatal("expected snapshot to update switch status")
	}

	stored := &v1beta1.Switch{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "leaf1"}, stored); err != nil {
		t.Fatalf("failed to fetch updated switch: %v", err)
	}
	if len(stored.Status.LLDPNeighbors) != 2 {
		t.Fatalf("expected 2 stored neighbors, got %d", len(stored.Status.LLDPNeighbors))
	}
	if stored.Status.LLDPNeighbors[0].RemoteSystemType != v1beta1.SwitchLLDPRemoteSystemTypeKubernetesNode {
		t.Fatalf("expected first remote system type KubernetesNode, got %s", stored.Status.LLDPNeighbors[0].RemoteSystemType)
	}
	if stored.Status.LLDPNeighbors[1].RemoteSystemType != v1beta1.SwitchLLDPRemoteSystemTypeSwitch {
		t.Fatalf("expected second remote system type Switch, got %s", stored.Status.LLDPNeighbors[1].RemoteSystemType)
	}
}
