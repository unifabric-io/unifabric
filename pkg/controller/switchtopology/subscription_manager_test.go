// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchtopology

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/logger"
	"github.com/unifabric-io/unifabric/pkg/switchagent"
)

func TestClassifySubscriptionErrorUsesSnapshotRejectedForInvalidSnapshot(t *testing.T) {
	err := grpcstatus.Error(codes.InvalidArgument, "snapshot contains malformed lldp neighbor entry")

	if reason := classifySubscriptionError(err); reason != v1beta1.SwitchReasonSnapshotRejected {
		t.Fatalf("expected %s, got %s", v1beta1.SwitchReasonSnapshotRejected, reason)
	}
}

func TestSyncSubscriptionsKeepsLabelOnlySwitchWithoutDialing(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add api scheme: %v", err)
	}

	switchObj := &v1beta1.Switch{
		ObjectMeta: metav1.ObjectMeta{
			Name: "spine1",
			Labels: map[string]string{
				v1beta1.TopologyDomainLabel: "tier2-group1",
			},
		},
		Status: v1beta1.SwitchStatus{Healthy: true},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(switchObj).
		WithStatusSubresource(&v1beta1.Switch{}).
		Build()
	manager := &subscriptionManager{
		client:        fakeClient,
		cfg:           &config.ControllerConfig{},
		log:           logger.MustNew(logger.LevelDebug),
		subscriptions: map[string]switchSubscription{},
	}

	if err := manager.syncSubscriptions(context.Background()); err != nil {
		t.Fatalf("syncSubscriptions returned error: %v", err)
	}
	if len(manager.subscriptions) != 0 {
		t.Fatalf("label-only Switch unexpectedly started subscriptions: %#v", manager.subscriptions)
	}

	stored := &v1beta1.Switch{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: switchObj.Name}, stored); err != nil {
		t.Fatalf("failed to fetch label-only Switch: %v", err)
	}
	if stored.Status.Healthy || stored.Status.Hostname != "" || stored.Status.LLDPNeighborCount != 0 || len(stored.Status.Conditions) != 0 || len(stored.Status.LLDPNeighbors) != 0 {
		t.Fatalf("subscription status was not cleared: %#v", stored.Status)
	}
	if stored.Labels[v1beta1.TopologyDomainLabel] != "tier2-group1" {
		t.Fatalf("topology membership label changed: %#v", stored.Labels)
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

	stored := &v1beta1.Switch{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "leaf1"}, stored); err != nil {
		t.Fatalf("failed to fetch switch after malformed snapshot: %v", err)
	}
	if stored.Status.Healthy {
		t.Fatal("expected malformed snapshot not to mark switch healthy")
	}
	if stored.Status.LLDPNeighborCount != 0 {
		t.Fatalf("expected malformed snapshot not to store neighbor count, got %d", stored.Status.LLDPNeighborCount)
	}
	if len(stored.Status.LLDPNeighbors) != 0 {
		t.Fatalf("expected malformed snapshot not to store neighbors, got %#v", stored.Status.LLDPNeighbors)
	}
	if len(stored.Status.Conditions) != 0 {
		t.Fatalf("expected malformed snapshot not to update switch conditions before disconnect handling, got %#v", stored.Status.Conditions)
	}

	reason := classifySubscriptionError(err)
	if reason != v1beta1.SwitchReasonSnapshotRejected {
		t.Fatalf("expected malformed snapshot reason %s, got %s", v1beta1.SwitchReasonSnapshotRejected, reason)
	}
	if err := manager.markSwitchDisconnected(context.Background(), "leaf1", reason, err.Error()); err != nil {
		t.Fatalf("markSwitchDisconnected returned error: %v", err)
	}

	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "leaf1"}, stored); err != nil {
		t.Fatalf("failed to fetch updated switch: %v", err)
	}
	connected := meta.FindStatusCondition(stored.Status.Conditions, v1beta1.SwitchConditionConnected)
	if connected == nil {
		t.Fatalf("expected connected condition, got %#v", stored.Status.Conditions)
	}
	if connected.Reason != v1beta1.SwitchReasonSnapshotRejected {
		t.Fatalf("expected connected condition reason %s, got %s", v1beta1.SwitchReasonSnapshotRejected, connected.Reason)
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

func TestHandleSnapshotIgnoresMalformedEntriesOnConfiguredSwitchPorts(t *testing.T) {
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
	nodeObj := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(switchObj, nodeObj).
		WithStatusSubresource(&v1beta1.Switch{}).
		Build()

	manager := &subscriptionManager{
		client: fakeClient,
		cfg: &config.ControllerConfig{
			ScaleOutDiscovery: config.ScaleOutDiscoveryConfig{
				Switches: config.ScaleOutSwitchesConfig{
					IgnoreSwitchPorts: []string{"mgmt*"},
				},
			},
		},
		log: logger.MustNew(logger.LevelDebug),
	}

	var lastGeneration uint64
	updated, err := manager.handleSnapshot(context.Background(), "leaf1", &switchagent.LLDPNeighborSnapshot{
		SwitchName: "leaf1",
		Generation: 1,
		LldpNeighbors: []*switchagent.LLDPNeighbor{
			{LocalDeviceName: "leaf1", LocalPort: "mgmt0", RemoteSystemName: "", RemotePortId: ""},
			{LocalDeviceName: "leaf1", LocalPort: "Ethernet1", RemoteSystemName: "node1", RemotePortId: "eth0"},
		},
	}, &lastGeneration)
	if err != nil {
		t.Fatalf("expected malformed ignored-port entry to be skipped, got error: %v", err)
	}
	if !updated {
		t.Fatal("expected snapshot to update switch status")
	}

	stored := &v1beta1.Switch{}
	if err := fakeClient.Get(context.Background(), types.NamespacedName{Name: "leaf1"}, stored); err != nil {
		t.Fatalf("failed to fetch updated switch: %v", err)
	}
	if stored.Status.LLDPNeighborCount != 1 {
		t.Fatalf("expected one stored neighbor after ignored-port filtering, got %d", stored.Status.LLDPNeighborCount)
	}
	if len(stored.Status.LLDPNeighbors) != 1 || stored.Status.LLDPNeighbors[0].RemoteSystemName != "node1" {
		t.Fatalf("unexpected stored neighbors: %#v", stored.Status.LLDPNeighbors)
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
	neighborTypesByName := map[string]v1beta1.SwitchLLDPRemoteSystemType{}
	for _, neighbor := range stored.Status.LLDPNeighbors {
		neighborTypesByName[neighbor.RemoteSystemName] = neighbor.RemoteSystemType
	}
	if neighborTypesByName["node-gpu-1"] != v1beta1.SwitchLLDPRemoteSystemTypeKubernetesNode {
		t.Fatalf("expected node-gpu-1 to be classified as KubernetesNode, got %#v", neighborTypesByName)
	}
	if neighborTypesByName["spine1"] != v1beta1.SwitchLLDPRemoteSystemTypeSwitch {
		t.Fatalf("expected spine1 to be classified as Switch, got %#v", neighborTypesByName)
	}
}
