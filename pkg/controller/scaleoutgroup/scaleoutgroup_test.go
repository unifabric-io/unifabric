// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package scaleoutgroup

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/logger"
	"github.com/unifabric-io/unifabric/pkg/utils"
)

var log = logger.MustNew(logger.LevelDebug)

func testControllerConfig() *config.ControllerConfig {
	return &config.ControllerConfig{
		TopologyLabels: config.TopologyLabelsConfig{
			ScaleOutLeaf: config.DefaultLabelScaleOutLeaf,
		},
	}
}

// Test 1: full sync cleans up empty groups.
func TestReconcile_FullSyncCleansEmptyGroups(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create an empty group with no nodes.
	emptyGroup := &v1beta1.ScaleOutLeafGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "empty-group",
		},
		Status: v1beta1.ScaleOutLeafGroupStatus{
			Nodes:    []v1beta1.Node{}, // Empty nodes.
			Switches: []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}},
		},
	}

	// Create a valid FabricNode.
	fabricNode1 := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: v1beta1.FabricNodeStatus{
			ScaleOutNics: []v1beta1.NicInfo{
				{Name: "eth0", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "switch2", MgmtIP: "192.168.1.2"}},
			},
		},
	}

	k8sNode1 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(emptyGroup, fabricNode1, k8sNode1).
		WithStatusSubresource(&v1beta1.ScaleOutLeafGroup{}).
		Build()

	reconciler := &Reconciler{
		cfg:            testControllerConfig(),
		client:         fakeClient,
		log:            log,
		recorder:       record.NewFakeRecorder(10),
		reconcileCount: 0, // First full sync.
	}

	_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node1"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify that the empty group is deleted and a new group is created.
	groupList := &v1beta1.ScaleOutLeafGroupList{}
	err = fakeClient.List(context.TODO(), groupList)
	if err != nil {
		t.Fatalf("Failed to list groups: %v", err)
	}

	if len(groupList.Items) != 1 {
		t.Errorf("Expected 1 group after cleanup, got %d", len(groupList.Items))
	}

	// Verify that the new group contains node1 and switch2.
	newGroup := groupList.Items[0]
	if len(newGroup.Status.Nodes) != 1 || newGroup.Status.Nodes[0].Name != "node1" {
		t.Errorf("Expected group to contain node1, got %v", newGroup.Status.Nodes)
	}

	expectedStatus := v1beta1.ScaleOutLeafGroupStatus{
		TotalNodes:   1,
		HealthyNodes: 1,
		Healthy:      true,
		Nodes:        []v1beta1.Node{{Name: "node1", Healthy: true}},
		Switches:     []v1beta1.Switch{{Name: "switch2", MgmtIP: "192.168.1.2"}},
	}
	if !reflect.DeepEqual(newGroup.Status, expectedStatus) {
		t.Errorf("Expected group status %v, got %v", expectedStatus, newGroup.Status)
	}
}

// Test 2: a new node joins an existing group.
func TestReconcile_NewNodeJoin(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	switches := []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}}
	switchNames := getSwitchNames(switches)
	groupName := utils.HashNodesToShortSHA(log, switchNames)

	// Create an existing group.
	existingGroup := &v1beta1.ScaleOutLeafGroup{
		ObjectMeta: metav1.ObjectMeta{Name: groupName},
		Status: v1beta1.ScaleOutLeafGroupStatus{
			Healthy:  true,
			Nodes:    []v1beta1.Node{{Name: "node1", Healthy: true}},
			Switches: []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}},
		},
	}

	// New node whose neighbors match the existing group.
	newFabricNode := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node2"},
		Status: v1beta1.FabricNodeStatus{
			ScaleOutNics: []v1beta1.NicInfo{
				{Name: "eth0", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "switch1", MgmtIP: "192.168.1.1"}},
				{Name: "eth1", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "switch2", MgmtIP: "192.168.1.2"}},
			},
		},
	}

	k8sNode2 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node2"}}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingGroup, newFabricNode, k8sNode2).
		WithStatusSubresource(&v1beta1.ScaleOutLeafGroup{}).
		Build()

	reconciler := &Reconciler{
		cfg:            testControllerConfig(),
		client:         fakeClient,
		log:            log,
		recorder:       record.NewFakeRecorder(10),
		reconcileCount: 1, // Non-initial sync.
	}

	_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node2"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify that node2 joins the existing group.
	updatedGroup := &v1beta1.ScaleOutLeafGroup{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: groupName}, updatedGroup)
	if err != nil {
		t.Fatalf("Failed to get updated group: %v", err)
	}

	expectedStatus := v1beta1.ScaleOutLeafGroupStatus{
		TotalNodes:   2,
		HealthyNodes: 2,
		Healthy:      true,
		Nodes:        []v1beta1.Node{{Name: "node1", Healthy: true}, {Name: "node2", Healthy: true}},
		Switches:     []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}},
	}
	if !reflect.DeepEqual(updatedGroup.Status, expectedStatus) {
		t.Errorf("Expected status %v, got %v", expectedStatus, updatedGroup.Status)
	}

	// Verify node labels.
	updatedNode2 := &corev1.Node{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node2"}, updatedNode2)
	if err != nil {
		t.Fatalf("Failed to get updated node2: %v", err)
	}
	if updatedNode2.Labels[reconciler.cfg.TopologyLabels.ScaleOutLeaf] != groupName {
		t.Errorf("Expected node2 label to be '%s', got %s", groupName, updatedNode2.Labels[reconciler.cfg.TopologyLabels.ScaleOutLeaf])
	}
}

// Test 3: a new node creates a new group.
func TestReconcile_NewNodeJoinWithCreateGroup(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	switches := []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}}
	switchNames := getSwitchNames(switches)
	groupName1 := utils.HashNodesToShortSHA(log, switchNames)
	// Create an existing group.
	existingGroup := &v1beta1.ScaleOutLeafGroup{
		ObjectMeta: metav1.ObjectMeta{Name: groupName1},
		Status: v1beta1.ScaleOutLeafGroupStatus{
			Nodes:    []v1beta1.Node{{Name: "node1", Healthy: true}},
			Switches: []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}},
		},
	}

	// New node whose neighbors do not match the existing group.
	newFabricNode := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node2"},
		Status: v1beta1.FabricNodeStatus{
			ScaleOutNics: []v1beta1.NicInfo{
				{Name: "eth0", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "switch3", MgmtIP: "192.168.1.3"}},
				{Name: "eth1", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "switch4", MgmtIP: "192.168.1.4"}},
			},
		},
	}

	k8sNode2 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node2"}}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingGroup, newFabricNode, k8sNode2).
		WithStatusSubresource(&v1beta1.ScaleOutLeafGroup{}).
		Build()

	reconciler := &Reconciler{
		cfg:            testControllerConfig(),
		client:         fakeClient,
		log:            log,
		recorder:       record.NewFakeRecorder(10),
		reconcileCount: 1, // Non-initial sync.
	}

	_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node2"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify that a new group is created.
	switches2 := []v1beta1.Switch{{Name: "switch3", MgmtIP: "192.168.1.3"}, {Name: "switch4", MgmtIP: "192.168.1.4"}}
	switchNames2 := getSwitchNames(switches2)
	groupName2 := utils.HashNodesToShortSHA(log, switchNames2)
	updatedGroup := &v1beta1.ScaleOutLeafGroup{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: groupName2}, updatedGroup)
	if err != nil {
		t.Fatalf("Failed to get updated group: %v", err)
	}

	expectedStatus := v1beta1.ScaleOutLeafGroupStatus{
		TotalNodes:   1,
		HealthyNodes: 1,
		Healthy:      true,
		Nodes:        []v1beta1.Node{{Name: "node2", Healthy: true}},
		Switches:     []v1beta1.Switch{{Name: "switch3", MgmtIP: "192.168.1.3"}, {Name: "switch4", MgmtIP: "192.168.1.4"}},
	}

	if !reflect.DeepEqual(updatedGroup.Status, expectedStatus) {
		t.Errorf("Expected status %v, got %v", expectedStatus, updatedGroup.Status)
	}

	// Verify node labels.
	updatedNode2 := &corev1.Node{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node2"}, updatedNode2)
	if err != nil {
		t.Fatalf("Failed to get updated node2: %v", err)
	}
	if updatedNode2.Labels[reconciler.cfg.TopologyLabels.ScaleOutLeaf] != groupName2 {
		t.Errorf("Expected node2 label to be '%s', got %s", groupName2, updatedNode2.Labels[reconciler.cfg.TopologyLabels.ScaleOutLeaf])
	}
}

// Test 4: a new node with no neighbors does not join any group and returns the expected error.
func TestReconcile_NewNodeWithNoNeighbors(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// New node with incomplete neighbor information.
	fabricNodeIncomplete := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: v1beta1.FabricNodeStatus{
			ScaleOutNics: []v1beta1.NicInfo{
				{Name: "eth0", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: ""}}, // Empty hostname.
				{Name: "eth1", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: ""}},
			},
		},
	}

	k8sNode1 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}

	switches := []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}}
	switchNames := getSwitchNames(switches)
	groupName := utils.HashNodesToShortSHA(log, switchNames)
	group := &v1beta1.ScaleOutLeafGroup{
		ObjectMeta: metav1.ObjectMeta{Name: groupName},
		Status: v1beta1.ScaleOutLeafGroupStatus{
			Nodes:    []v1beta1.Node{{Name: "node2", Healthy: true}},
			Switches: []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(fabricNodeIncomplete, k8sNode1, group).
		WithStatusSubresource(&v1beta1.ScaleOutLeafGroup{}).
		Build()

	reconciler := &Reconciler{
		cfg:            testControllerConfig(),
		client:         fakeClient,
		log:            log,
		recorder:       record.NewFakeRecorder(10),
		reconcileCount: 1,
	}

	_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node1"}})
	if err == nil {
		t.Fatalf("expected error, but got nil")
	}

	expectedError := "no neighbors found for node node1"
	if err.Error() != expectedError {
		t.Fatalf("expected error '%s', but got %v", expectedError, err)
	}
}

// Test 5: a new node with incomplete neighbors joins an existing group but is marked unhealthy.
func TestReconcile_NewNodeIncompleteNeighborsWithUnhealthyNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// New node with incomplete neighbor information.
	fabricNodeIncomplete := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: v1beta1.FabricNodeStatus{
			ScaleOutNics: []v1beta1.NicInfo{
				{Name: "eth0", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: ""}}, // Empty hostname.
				{Name: "eth1", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "switch1", MgmtIP: "192.168.1.1"}},
			},
		},
	}

	k8sNode1 := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}
	switches := []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}}
	switchNames := getSwitchNames(switches)
	groupName := utils.HashNodesToShortSHA(log, switchNames)
	group := &v1beta1.ScaleOutLeafGroup{
		ObjectMeta: metav1.ObjectMeta{Name: groupName},
		Status: v1beta1.ScaleOutLeafGroupStatus{
			Healthy:  true,
			Nodes:    []v1beta1.Node{{Name: "node2", Healthy: true}},
			Switches: []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(fabricNodeIncomplete, k8sNode1, group).
		WithStatusSubresource(&v1beta1.ScaleOutLeafGroup{}).
		Build()

	reconciler := &Reconciler{
		cfg:            testControllerConfig(),
		client:         fakeClient,
		log:            log,
		recorder:       record.NewFakeRecorder(10),
		reconcileCount: 1,
	}

	_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node1"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	g := &v1beta1.ScaleOutLeafGroup{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: groupName}, g)
	if err != nil {
		t.Fatalf("Failed to get group: %v", err)
	}

	// Verify that node1 joins the group but is marked unhealthy.
	expectedGroupStatus := v1beta1.ScaleOutLeafGroupStatus{
		TotalNodes:   2,
		HealthyNodes: 1,
		Healthy:      false,
		Nodes:        []v1beta1.Node{{Name: "node1", Healthy: false}, {Name: "node2", Healthy: true}},
		Switches:     []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}},
	}
	if !reflect.DeepEqual(g.Status, expectedGroupStatus) {
		t.Errorf("Expected group status to be %v, got %v", expectedGroupStatus, g.Status)
	}

	// Verify node1 labels.
	n1 := &corev1.Node{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node1"}, n1)
	if err != nil {
		t.Fatalf("Failed to get node1: %v", err)
	}

	_, ok := n1.Labels[reconciler.cfg.TopologyLabels.ScaleOutLeaf]
	if ok {
		t.Errorf("unhealthy node1 should not have label %s", reconciler.cfg.TopologyLabels.ScaleOutLeaf)
	}
}

// Test 6: FabricNode deletion removes the node from the group, deletes empty groups, and clears labels.
func TestReconcile_FabricNodeDeletionRemovesFromGroup(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create a group with only one node.
	switches := []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}}
	switchNames := getSwitchNames(switches)
	groupName := utils.HashNodesToShortSHA(log, switchNames)
	singleNodeGroup := &v1beta1.ScaleOutLeafGroup{
		ObjectMeta: metav1.ObjectMeta{Name: groupName},
		Status: v1beta1.ScaleOutLeafGroupStatus{
			Healthy:  true,
			Nodes:    []v1beta1.Node{{Name: "node1", Healthy: true}},
			Switches: []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}},
		},
	}

	reconciler := &Reconciler{
		cfg:            testControllerConfig(),
		log:            log,
		recorder:       record.NewFakeRecorder(10),
		reconcileCount: 1,
	}

	k8sNode1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{reconciler.cfg.TopologyLabels.ScaleOutLeaf: groupName},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(singleNodeGroup, k8sNode1).
		WithStatusSubresource(&v1beta1.ScaleOutLeafGroup{}).
		Build()

	reconciler.client = fakeClient

	// Simulate FabricNode deletion by omitting it from the fake client.
	_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node1"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify that the group is deleted.
	groupList := &v1beta1.ScaleOutLeafGroupList{}
	err = fakeClient.List(context.TODO(), groupList)
	if err != nil {
		t.Fatalf("Failed to list groups: %v", err)
	}
	if len(groupList.Items) != 0 {
		t.Errorf("Expected group to be deleted, but found %d groups", len(groupList.Items))
	}

	// Verify that the node label is removed.
	updatedNode1 := &corev1.Node{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node1"}, updatedNode1)
	if err != nil {
		t.Fatalf("Failed to get updated node1: %v", err)
	}

	_, ok := updatedNode1.Labels[reconciler.cfg.TopologyLabels.ScaleOutLeaf]
	if ok {
		t.Errorf("node1 should not have label %s", reconciler.cfg.TopologyLabels.ScaleOutLeaf)
	}
}

// Test 7: partial neighbor loss keeps the node in the group, marks it unhealthy, and removes the label.
func TestReconcile_PartialNeighborLossMarksUnhealthy(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	switches := []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}}
	switchNames := getSwitchNames(switches)
	groupName := utils.HashNodesToShortSHA(log, switchNames)
	existingGroup := &v1beta1.ScaleOutLeafGroup{
		ObjectMeta: metav1.ObjectMeta{Name: groupName},
		Status: v1beta1.ScaleOutLeafGroupStatus{
			Healthy:  true,
			Nodes:    []v1beta1.Node{{Name: "node1", Healthy: true}, {Name: "node2", Healthy: true}},
			Switches: []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}},
		},
	}

	// Node partially loses neighbors and keeps only one neighbor.
	fabricNodePartialLoss := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: v1beta1.FabricNodeStatus{
			ScaleOutNics: []v1beta1.NicInfo{
				{Name: "eth0", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "switch1", MgmtIP: "192.168.1.1"}},
				{Name: "eth1", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: ""}},
				// eth1 has no neighbor information, simulating a link failure.
			},
		},
	}

	reconciler := &Reconciler{
		cfg:            testControllerConfig(),
		log:            log,
		recorder:       record.NewFakeRecorder(10),
		reconcileCount: 1,
	}

	k8sNode1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{reconciler.cfg.TopologyLabels.ScaleOutLeaf: groupName},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existingGroup, fabricNodePartialLoss, k8sNode1).
		WithStatusSubresource(&v1beta1.ScaleOutLeafGroup{}).
		Build()
	reconciler.client = fakeClient

	_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node1"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// Verify that the node stays in the group but is marked unhealthy.
	updatedGroup := &v1beta1.ScaleOutLeafGroup{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: groupName}, updatedGroup)
	if err != nil {
		t.Fatalf("Failed to get updated group: %v", err)
	}

	expectedGroupStatus := v1beta1.ScaleOutLeafGroupStatus{
		TotalNodes:   2,
		HealthyNodes: 1,
		Healthy:      false,
		Nodes:        []v1beta1.Node{{Name: "node1", Healthy: false}, {Name: "node2", Healthy: true}},
		Switches:     []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}},
	}
	if !reflect.DeepEqual(updatedGroup.Status, expectedGroupStatus) {
		t.Errorf("Expected group status to be %v, got %v", expectedGroupStatus, updatedGroup.Status)
	}

	// Verify that the node label is removed.
	updatedNode1 := &corev1.Node{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node1"}, updatedNode1)
	if err != nil {
		t.Fatalf("Failed to get updated node1: %v", err)
	}
	_, ok := updatedNode1.Labels[reconciler.cfg.TopologyLabels.ScaleOutLeaf]
	if ok {
		t.Errorf("unhealthy node1 should not have label %s", reconciler.cfg.TopologyLabels.ScaleOutLeaf)
	}
}

// Test 8: a node changes RDMA zones, leaves the old group, and joins the new group.
func TestReconcile_NodeChangesRDMAZone(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	switches := []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}}
	switchNames := getSwitchNames(switches)
	groupName1 := utils.HashNodesToShortSHA(log, switchNames)
	group1 := &v1beta1.ScaleOutLeafGroup{
		ObjectMeta: metav1.ObjectMeta{Name: groupName1},
		Status: v1beta1.ScaleOutLeafGroupStatus{
			Healthy: true,
			Nodes: []v1beta1.Node{{Name: "node1", Healthy: true},
				{Name: "node2", Healthy: true},
			},
			Switches: []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}},
		},
	}

	switches2 := []v1beta1.Switch{{Name: "switch3", MgmtIP: "192.168.1.3"}, {Name: "switch4", MgmtIP: "192.168.1.4"}}
	switchNames2 := getSwitchNames(switches2)
	groupName2 := utils.HashNodesToShortSHA(log, switchNames2)
	group2 := &v1beta1.ScaleOutLeafGroup{
		ObjectMeta: metav1.ObjectMeta{Name: groupName2},
		Status: v1beta1.ScaleOutLeafGroupStatus{
			Healthy:  true,
			Nodes:    []v1beta1.Node{{Name: "node3", Healthy: true}},
			Switches: []v1beta1.Switch{{Name: "switch3", MgmtIP: "192.168.1.3"}, {Name: "switch4", MgmtIP: "192.168.1.4"}},
		},
	}

	// The node is now connected to different switches.
	fabricNodeNewZone := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node2"},
		Status: v1beta1.FabricNodeStatus{
			ScaleOutNics: []v1beta1.NicInfo{
				{Name: "eth0", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "switch3", MgmtIP: "192.168.1.3"}},
				{Name: "eth1", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "switch4", MgmtIP: "192.168.1.4"}},
			},
		},
	}

	reconciler := &Reconciler{
		cfg:            testControllerConfig(),
		log:            log,
		recorder:       record.NewFakeRecorder(10),
		reconcileCount: 1,
	}

	k8sNode1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node2",
			Labels: map[string]string{reconciler.cfg.TopologyLabels.ScaleOutLeaf: groupName1},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(group1, group2, fabricNodeNewZone, k8sNode1).
		WithStatusSubresource(&v1beta1.ScaleOutLeafGroup{}).
		Build()
	reconciler.client = fakeClient

	_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node2"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	oldGroup := &v1beta1.ScaleOutLeafGroup{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: groupName1}, oldGroup)
	if err != nil {
		t.Fatalf("Failed to get old group: %v", err)
	}

	expectedOldGroupStatus := v1beta1.ScaleOutLeafGroupStatus{
		TotalNodes:   1,
		HealthyNodes: 1,
		Healthy:      true,
		Nodes:        []v1beta1.Node{{Name: "node1", Healthy: true}},
		Switches:     []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}},
	}
	if !reflect.DeepEqual(oldGroup.Status, expectedOldGroupStatus) {
		t.Errorf("Expected old group status to be %v, got %v", expectedOldGroupStatus, oldGroup.Status)
	}

	newGroup := &v1beta1.ScaleOutLeafGroup{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: groupName2}, newGroup)
	if err != nil {
		t.Fatalf("Failed to get new group: %v", err)
	}
	expectedNewGroupStatus := v1beta1.ScaleOutLeafGroupStatus{
		TotalNodes:   2,
		HealthyNodes: 2,
		Healthy:      true,
		Nodes:        []v1beta1.Node{{Name: "node2", Healthy: true}, {Name: "node3", Healthy: true}},
		Switches:     []v1beta1.Switch{{Name: "switch3", MgmtIP: "192.168.1.3"}, {Name: "switch4", MgmtIP: "192.168.1.4"}},
	}
	if !reflect.DeepEqual(newGroup.Status, expectedNewGroupStatus) {
		t.Errorf("Expected new group status to be %v, got %v", expectedNewGroupStatus, newGroup.Status)
	}

	// Verify that the node label is updated to the new group.
	updatedNode2 := &corev1.Node{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node2"}, updatedNode2)
	if err != nil {
		t.Fatalf("Failed to get updated node2: %v", err)
	}

	if updatedNode2.Labels[reconciler.cfg.TopologyLabels.ScaleOutLeaf] != groupName2 {
		t.Errorf("Expected node2 label to be '%s', got %s", groupName2, updatedNode2.Labels[reconciler.cfg.TopologyLabels.ScaleOutLeaf])
	}
}

// Test 9: deleting an unhealthy node should make the group healthy.
func TestReconcile_DeleteUnhealthyNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	switches := []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}}
	switchNames := getSwitchNames(switches)
	groupName1 := utils.HashNodesToShortSHA(log, switchNames)
	group1 := &v1beta1.ScaleOutLeafGroup{
		ObjectMeta: metav1.ObjectMeta{Name: groupName1},
		Status: v1beta1.ScaleOutLeafGroupStatus{
			TotalNodes:   3,
			HealthyNodes: 2,
			Healthy:      false,
			Nodes: []v1beta1.Node{{Name: "node1", Healthy: true},
				{Name: "node2", Healthy: true},
				{Name: "node3", Healthy: false},
			},
			Switches: []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}},
		},
	}

	k8sNode3 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node3"},
	}

	reconciler := &Reconciler{
		cfg:            testControllerConfig(),
		log:            log,
		recorder:       record.NewFakeRecorder(10),
		reconcileCount: 1,
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(group1, k8sNode3).
		WithStatusSubresource(&v1beta1.ScaleOutLeafGroup{}).
		Build()
	reconciler.client = fakeClient

	_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node3"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	group := &v1beta1.ScaleOutLeafGroup{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: groupName1}, group)
	if err != nil {
		t.Fatalf("Failed to get group: %v", err)
	}

	expectedOldGroupStatus := v1beta1.ScaleOutLeafGroupStatus{
		TotalNodes:   2,
		HealthyNodes: 2,
		Healthy:      true,
		Nodes:        []v1beta1.Node{{Name: "node1", Healthy: true}, {Name: "node2", Healthy: true}},
		Switches:     []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}},
	}
	if !reflect.DeepEqual(group.Status, expectedOldGroupStatus) {
		t.Errorf("Expected group status to be %v, got %v", expectedOldGroupStatus, group.Status)
	}
}
