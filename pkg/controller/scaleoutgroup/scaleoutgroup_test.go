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

// Test 1: 全量同步 - 确保空的 group 被清理
func TestReconcile_FullSyncCleansEmptyGroups(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// 创建一个空的 group（没有 nodes）
	emptyGroup := &v1beta1.ScaleOutLeafGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "empty-group",
		},
		Status: v1beta1.ScaleOutLeafGroupStatus{
			Nodes:    []v1beta1.Node{}, // 空的 nodes
			Switches: []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}},
		},
	}

	// 创建有效的 FabricNode
	fabricNode1 := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: v1beta1.FabricNodeStatus{
			GpuNics: []v1beta1.NicInfo{
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
		cfg:            &config.ControllerConfig{Topology: config.ControllerTopologyConfig{ScaleOutLeafGroups: config.ScaleOutLeafGroupConfig{NodeLabelKey: config.DefaultLabelLeaf}}},
		client:         fakeClient,
		log:            log,
		recorder:       record.NewFakeRecorder(10),
		reconcileCount: 0, // 首次全量同步
	}

	_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node1"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// 验证空 group 被删除，新 group 被创建
	groupList := &v1beta1.ScaleOutLeafGroupList{}
	err = fakeClient.List(context.TODO(), groupList)
	if err != nil {
		t.Fatalf("Failed to list groups: %v", err)
	}

	if len(groupList.Items) != 1 {
		t.Errorf("Expected 1 group after cleanup, got %d", len(groupList.Items))
	}

	// 验证新创建的 group 包含 node1 和 switch2
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

// Test 2: 新增节点加入现有 group
func TestReconcile_NewNodeJoin(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	switches := []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}}
	switchNames := getSwitchNames(switches)
	groupName := utils.HashNodesToShortSHA(log, switchNames)

	// 创建现有 group
	existingGroup := &v1beta1.ScaleOutLeafGroup{
		ObjectMeta: metav1.ObjectMeta{Name: groupName},
		Status: v1beta1.ScaleOutLeafGroupStatus{
			Healthy:  true,
			Nodes:    []v1beta1.Node{{Name: "node1", Healthy: true}},
			Switches: []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}},
		},
	}

	// 新节点，邻居匹配现有 group
	newFabricNode := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node2"},
		Status: v1beta1.FabricNodeStatus{
			GpuNics: []v1beta1.NicInfo{
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
		cfg:            &config.ControllerConfig{Topology: config.ControllerTopologyConfig{ScaleOutLeafGroups: config.ScaleOutLeafGroupConfig{NodeLabelKey: config.DefaultLabelLeaf}}},
		client:         fakeClient,
		log:            log,
		recorder:       record.NewFakeRecorder(10),
		reconcileCount: 1, // 非首次同步
	}

	_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node2"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// 验证 node2 加入现有 group
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

	// 验证节点标签
	updatedNode2 := &corev1.Node{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node2"}, updatedNode2)
	if err != nil {
		t.Fatalf("Failed to get updated node2: %v", err)
	}
	if updatedNode2.Labels[reconciler.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey] != groupName {
		t.Errorf("Expected node2 label to be '%s', got %s", groupName, updatedNode2.Labels[reconciler.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey])
	}
}

// Test 3: 新增节点创建新 group
func TestReconcile_NewNodeJoinWithCreateGroup(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	switches := []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}}
	switchNames := getSwitchNames(switches)
	groupName1 := utils.HashNodesToShortSHA(log, switchNames)
	// 创建现有 group
	existingGroup := &v1beta1.ScaleOutLeafGroup{
		ObjectMeta: metav1.ObjectMeta{Name: groupName1},
		Status: v1beta1.ScaleOutLeafGroupStatus{
			Nodes:    []v1beta1.Node{{Name: "node1", Healthy: true}},
			Switches: []v1beta1.Switch{{Name: "switch1", MgmtIP: "192.168.1.1"}, {Name: "switch2", MgmtIP: "192.168.1.2"}},
		},
	}

	// 新节点，邻居不匹配现有 group
	newFabricNode := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node2"},
		Status: v1beta1.FabricNodeStatus{
			GpuNics: []v1beta1.NicInfo{
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
		cfg:            &config.ControllerConfig{Topology: config.ControllerTopologyConfig{ScaleOutLeafGroups: config.ScaleOutLeafGroupConfig{NodeLabelKey: config.DefaultLabelLeaf}}},
		client:         fakeClient,
		log:            log,
		recorder:       record.NewFakeRecorder(10),
		reconcileCount: 1, // 非首次同步
	}

	_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node2"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// 验证新 group 被创建
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

	// 验证节点标签
	updatedNode2 := &corev1.Node{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node2"}, updatedNode2)
	if err != nil {
		t.Fatalf("Failed to get updated node2: %v", err)
	}
	if updatedNode2.Labels[reconciler.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey] != groupName2 {
		t.Errorf("Expected node2 label to be '%s', got %s", groupName2, updatedNode2.Labels[reconciler.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey])
	}
}

// Test 4: 新增节点没有任何邻居，不加入任何 group, 返回预期错误
func TestReconcile_NewNodeWithNoNeighbors(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// 新节点，邻居信息不完整
	fabricNodeIncomplete := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: v1beta1.FabricNodeStatus{
			GpuNics: []v1beta1.NicInfo{
				{Name: "eth0", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: ""}}, // 空 hostname
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
		cfg:            &config.ControllerConfig{Topology: config.ControllerTopologyConfig{ScaleOutLeafGroups: config.ScaleOutLeafGroupConfig{NodeLabelKey: config.DefaultLabelLeaf}}},
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

// Test 5: 新增节点邻居信息不完整，尝试加入现有 group，但邻居节点状态为不健康
func TestReconcile_NewNodeIncompleteNeighborsWithUnhealthyNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// 新节点，邻居信息不完整
	fabricNodeIncomplete := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: v1beta1.FabricNodeStatus{
			GpuNics: []v1beta1.NicInfo{
				{Name: "eth0", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: ""}}, // 空 hostname
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
		cfg:            &config.ControllerConfig{Topology: config.ControllerTopologyConfig{ScaleOutLeafGroups: config.ScaleOutLeafGroupConfig{NodeLabelKey: config.DefaultLabelLeaf}}},
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

	// 验证 node1 加入到 group， 但状态为 unhealthy
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

	// 验证 node1 的标签
	n1 := &corev1.Node{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node1"}, n1)
	if err != nil {
		t.Fatalf("Failed to get node1: %v", err)
	}

	_, ok := n1.Labels[reconciler.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey]
	if ok {
		t.Errorf("unhealthy node1 should not have label %s", reconciler.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey)
	}
}

// Test 6: FabricNode 删除，从 group 移除节点，检查空 group 删除和标签
func TestReconcile_FabricNodeDeletionRemovesFromGroup(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1beta1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// 创建只有一个节点的 group
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
		cfg:            &config.ControllerConfig{Topology: config.ControllerTopologyConfig{ScaleOutLeafGroups: config.ScaleOutLeafGroupConfig{NodeLabelKey: config.DefaultLabelLeaf}}},
		log:            log,
		recorder:       record.NewFakeRecorder(10),
		reconcileCount: 1,
	}

	k8sNode1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{reconciler.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey: groupName},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(singleNodeGroup, k8sNode1).
		WithStatusSubresource(&v1beta1.ScaleOutLeafGroup{}).
		Build()

	reconciler.client = fakeClient

	// 模拟 FabricNode 被删除（不存在）
	_, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node1"}})
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	// 验证 group 被删除
	groupList := &v1beta1.ScaleOutLeafGroupList{}
	err = fakeClient.List(context.TODO(), groupList)
	if err != nil {
		t.Fatalf("Failed to list groups: %v", err)
	}
	if len(groupList.Items) != 0 {
		t.Errorf("Expected group to be deleted, but found %d groups", len(groupList.Items))
	}

	// 验证节点标签被移除
	updatedNode1 := &corev1.Node{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node1"}, updatedNode1)
	if err != nil {
		t.Fatalf("Failed to get updated node1: %v", err)
	}

	_, ok := updatedNode1.Labels[reconciler.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey]
	if ok {
		t.Errorf("node1 should not have label %s", reconciler.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey)
	}
}

// Test 7: 部分邻居丢失，节点保留在 group 但标记不健康，移除标签
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

	// 节点部分邻居丢失（只有一个邻居）
	fabricNodePartialLoss := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: v1beta1.FabricNodeStatus{
			GpuNics: []v1beta1.NicInfo{
				{Name: "eth0", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "switch1", MgmtIP: "192.168.1.1"}},
				{Name: "eth1", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: ""}},
				// eth1 没有邻居信息，模拟链路故障
			},
		},
	}

	reconciler := &Reconciler{
		cfg:            &config.ControllerConfig{Topology: config.ControllerTopologyConfig{ScaleOutLeafGroups: config.ScaleOutLeafGroupConfig{NodeLabelKey: config.DefaultLabelLeaf}}},
		log:            log,
		recorder:       record.NewFakeRecorder(10),
		reconcileCount: 1,
	}

	k8sNode1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node1",
			Labels: map[string]string{reconciler.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey: groupName},
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

	// 验证节点仍在 group 中但标记为不健康
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

	// 验证节点标签被移除
	updatedNode1 := &corev1.Node{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node1"}, updatedNode1)
	if err != nil {
		t.Fatalf("Failed to get updated node1: %v", err)
	}
	_, ok := updatedNode1.Labels[reconciler.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey]
	if ok {
		t.Errorf("unhealthy node1 should not have label %s", reconciler.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey)
	}
}

// Test 8: 节点变更 RDMA 区域，从旧 group 移除，加入新 group
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

	// 节点现在连接到不同的交换机
	fabricNodeNewZone := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node2"},
		Status: v1beta1.FabricNodeStatus{
			GpuNics: []v1beta1.NicInfo{
				{Name: "eth0", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "switch3", MgmtIP: "192.168.1.3"}},
				{Name: "eth1", State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "switch4", MgmtIP: "192.168.1.4"}},
			},
		},
	}

	reconciler := &Reconciler{
		cfg:            &config.ControllerConfig{Topology: config.ControllerTopologyConfig{ScaleOutLeafGroups: config.ScaleOutLeafGroupConfig{NodeLabelKey: config.DefaultLabelLeaf}}},
		log:            log,
		recorder:       record.NewFakeRecorder(10),
		reconcileCount: 1,
	}

	k8sNode1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node2",
			Labels: map[string]string{reconciler.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey: groupName1},
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

	// 验证节点标签更新为新 group
	updatedNode2 := &corev1.Node{}
	err = fakeClient.Get(context.TODO(), types.NamespacedName{Name: "node2"}, updatedNode2)
	if err != nil {
		t.Fatalf("Failed to get updated node2: %v", err)
	}

	if updatedNode2.Labels[reconciler.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey] != groupName2 {
		t.Errorf("Expected node2 label to be '%s', got %s", groupName2, updatedNode2.Labels[reconciler.cfg.Topology.ScaleOutLeafGroups.NodeLabelKey])
	}
}

// Test 9: 删除一个 unhealthy 的节点, group 的状态应该为 healthy
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
		cfg:            &config.ControllerConfig{Topology: config.ControllerTopologyConfig{ScaleOutLeafGroups: config.ScaleOutLeafGroupConfig{NodeLabelKey: config.DefaultLabelLeaf}}},
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
