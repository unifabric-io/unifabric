// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchtopology

import (
	"context"
	"testing"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/logger"
	"github.com/unifabric-io/unifabric/pkg/topologylabel"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func mustAutoTemplate(t *testing.T) *topologylabel.Template {
	t.Helper()
	compiled, err := topologylabel.Compile("scaleout", "scale-out.unifabric.io/tier-{{ .Tier }}")
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func TestBuildAutoLabelPlanAssignsDynamicTierNamesAndMembers(t *testing.T) {
	fabricNodes, switches, nodes := redundantTwoTierInputs()
	plan, err := buildAutoLabelPlan(v1beta1.SwitchRoleScaleOut, mustAutoTemplate(t), fabricNodes, switches, nodes)
	if err != nil {
		t.Fatalf("buildAutoLabelPlan() error = %v", err)
	}
	for _, nodeName := range []string{"node1", "node2"} {
		if got := plan.nodes[nodeName]["scale-out.unifabric.io/tier-1"]; got != "tier1-group1" {
			t.Fatalf("Node %s tier 1 = %q", nodeName, got)
		}
		if got := plan.nodes[nodeName]["scale-out.unifabric.io/tier-2"]; got != "tier2-group1" {
			t.Fatalf("Node %s tier 2 = %q", nodeName, got)
		}
	}
	for _, switchName := range []string{"leaf1", "leaf2"} {
		if got := plan.switches[switchName][v1beta1.TopologyDomainLabel]; got != "tier1-group1" {
			t.Fatalf("Switch %s tier 1 = %q", switchName, got)
		}
	}
	for _, switchName := range []string{"spine1", "spine2"} {
		if got := plan.switches[switchName][v1beta1.TopologyDomainLabel]; got != "tier2-group1" {
			t.Fatalf("Switch %s tier 2 = %q", switchName, got)
		}
	}
}

func TestBuildAutoLabelPlanWithoutSwitchCRAssignsOnlyNodeTierOne(t *testing.T) {
	fabricNodes, _, nodes := redundantTwoTierInputs()
	plan, err := buildAutoLabelPlan(v1beta1.SwitchRoleScaleOut, mustAutoTemplate(t), fabricNodes, nil, nodes)
	if err != nil {
		t.Fatalf("buildAutoLabelPlan() error = %v", err)
	}
	for _, nodeName := range []string{"node1", "node2"} {
		if got := plan.nodes[nodeName]["scale-out.unifabric.io/tier-1"]; got != "tier1-group1" {
			t.Fatalf("Node %s tier 1 = %q", nodeName, got)
		}
		if _, exists := plan.nodes[nodeName]["scale-out.unifabric.io/tier-2"]; exists {
			t.Fatalf("Node %s unexpectedly received a tier 2 label", nodeName)
		}
	}
	if len(plan.switches) != 0 {
		t.Fatalf("node-only plan attempted to label synthetic Switches: %#v", plan.switches)
	}
}

func TestBuildAutoLabelPlanUsesManualSwitchNeighbors(t *testing.T) {
	fabricNodes := []v1beta1.FabricNode{{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: v1beta1.FabricNodeStatus{
			NodeRole: v1beta1.NodeRoleGPU,
			ScaleOutNics: []v1beta1.NicInfo{{
				State:        "up",
				LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "leaf1", Port: "Ethernet1"},
			}},
		},
	}}
	switches := []v1beta1.Switch{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "spine1",
				Annotations: map[string]string{v1beta1.SwitchNeighborsAnnotation: `["leaf1"]`},
			},
			Spec: v1beta1.SwitchSpec{Role: v1beta1.SwitchRoleScaleOut},
		},
	}
	nodes := []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "node1"}}}

	plan, err := buildAutoLabelPlan(v1beta1.SwitchRoleScaleOut, mustAutoTemplate(t), fabricNodes, switches, nodes)
	if err != nil {
		t.Fatalf("buildAutoLabelPlan() error = %v", err)
	}
	if got := plan.nodes["node1"]["scale-out.unifabric.io/tier-1"]; got != "tier1-group1" {
		t.Fatalf("Node tier 1 = %q", got)
	}
	if got := plan.nodes["node1"]["scale-out.unifabric.io/tier-2"]; got != "tier2-group1" {
		t.Fatalf("Node tier 2 = %q", got)
	}
	if _, exists := plan.switches["leaf1"]; exists {
		t.Fatalf("plan attempted to label synthetic leaf: %#v", plan.switches["leaf1"])
	}
	if got := plan.switches["spine1"][v1beta1.TopologyDomainLabel]; got != "tier2-group1" {
		t.Fatalf("spine domain = %q", got)
	}
}

func TestBuildAutoLabelPlanStorageUsesManualSwitchNeighbors(t *testing.T) {
	storageTemplate, err := topologylabel.Compile("storage", "storage.unifabric.io/tier-{{ .Tier }}")
	if err != nil {
		t.Fatal(err)
	}
	fabricNodes := []v1beta1.FabricNode{{
		ObjectMeta: metav1.ObjectMeta{Name: "storage-node1"},
		Status: v1beta1.FabricNodeStatus{
			NodeRole: v1beta1.NodeRoleStorage,
			StorageNics: []v1beta1.NicInfo{{
				State:        "up",
				LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "storage-leaf1", Port: "Ethernet1"},
			}},
		},
	}}
	switches := []v1beta1.Switch{{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "storage-spine1",
			Annotations: map[string]string{v1beta1.SwitchNeighborsAnnotation: `["storage-leaf1"]`},
		},
		Spec: v1beta1.SwitchSpec{Role: v1beta1.SwitchRoleStorage},
	}}
	nodes := []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "storage-node1"}}}

	plan, err := buildAutoLabelPlan(v1beta1.SwitchRoleStorage, storageTemplate, fabricNodes, switches, nodes)
	if err != nil {
		t.Fatalf("buildAutoLabelPlan() error = %v", err)
	}
	if got := plan.nodes["storage-node1"]["storage.unifabric.io/tier-1"]; got != "tier1-group1" {
		t.Fatalf("Storage Node tier 1 = %q", got)
	}
	if got := plan.nodes["storage-node1"]["storage.unifabric.io/tier-2"]; got != "tier2-group1" {
		t.Fatalf("Storage Node tier 2 = %q", got)
	}
	if _, exists := plan.switches["storage-leaf1"]; exists {
		t.Fatalf("plan attempted to label synthetic Storage leaf: %#v", plan.switches["storage-leaf1"])
	}
	if got := plan.switches["storage-spine1"][v1beta1.TopologyDomainLabel]; got != "tier2-group1" {
		t.Fatalf("Storage spine domain = %q", got)
	}
}

func TestBuildAutoLabelPlanUsesUnhealthySwitchData(t *testing.T) {
	fabricNodes, switches, nodes := redundantTwoTierInputs()
	for index := range switches {
		switches[index].Status.Healthy = false
	}

	plan, err := buildAutoLabelPlan(v1beta1.SwitchRoleScaleOut, mustAutoTemplate(t), fabricNodes, switches, nodes)
	if err != nil {
		t.Fatalf("buildAutoLabelPlan() error = %v", err)
	}
	for _, nodeName := range []string{"node1", "node2"} {
		if got := plan.nodes[nodeName]["scale-out.unifabric.io/tier-2"]; got != "tier2-group1" {
			t.Fatalf("Node %s tier 2 from unhealthy Switch data = %q", nodeName, got)
		}
	}
	for _, switchName := range []string{"leaf1", "leaf2", "spine1", "spine2"} {
		if len(plan.switches[switchName]) == 0 {
			t.Fatalf("unhealthy Switch %s did not receive a topology label", switchName)
		}
	}
}

func TestBuildAutoLabelPlanReusesLockedNameAndOnlyFillsMissing(t *testing.T) {
	fabricNodes, switches, nodes := redundantTwoTierInputs()
	nodes[0].Labels = map[string]string{"scale-out.unifabric.io/tier-1": "tier1-group7"}
	switches[0].Labels = map[string]string{v1beta1.TopologyDomainLabel: "tier1-group7"}

	plan, err := buildAutoLabelPlan(v1beta1.SwitchRoleScaleOut, mustAutoTemplate(t), fabricNodes, switches, nodes)
	if err != nil {
		t.Fatalf("buildAutoLabelPlan() error = %v", err)
	}
	if _, exists := plan.nodes["node1"]["scale-out.unifabric.io/tier-1"]; exists {
		t.Fatal("plan attempted to rewrite an existing locked Node label")
	}
	if got := plan.nodes["node2"]["scale-out.unifabric.io/tier-1"]; got != "tier1-group7" {
		t.Fatalf("missing Node label = %q", got)
	}
	if got := plan.switches["leaf2"][v1beta1.TopologyDomainLabel]; got != "tier1-group7" {
		t.Fatalf("missing Switch label = %q", got)
	}
}

func TestBuildAutoLabelPlanIgnoresOtherRoleGroupNames(t *testing.T) {
	fabricNodes, switches, nodes := redundantTwoTierInputs()
	switches = append(switches, v1beta1.Switch{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "storage1",
			Labels: map[string]string{v1beta1.TopologyDomainLabel: "tier1-group99"},
		},
		Spec: v1beta1.SwitchSpec{Role: v1beta1.SwitchRoleStorage},
	})

	plan, err := buildAutoLabelPlan(v1beta1.SwitchRoleScaleOut, mustAutoTemplate(t), fabricNodes, switches, nodes)
	if err != nil {
		t.Fatalf("buildAutoLabelPlan() error = %v", err)
	}
	if got := plan.nodes["node1"]["scale-out.unifabric.io/tier-1"]; got != "tier1-group1" {
		t.Fatalf("other-role group name affected scale-out numbering: %q", got)
	}
}

func TestBuildAutoLabelPlanRejectsLockedConflict(t *testing.T) {
	fabricNodes, switches, nodes := redundantTwoTierInputs()
	nodes[0].Labels = map[string]string{"scale-out.unifabric.io/tier-1": "tier1-group1"}
	nodes[1].Labels = map[string]string{"scale-out.unifabric.io/tier-1": "tier1-group2"}
	if _, err := buildAutoLabelPlan(v1beta1.SwitchRoleScaleOut, mustAutoTemplate(t), fabricNodes, switches, nodes); err == nil {
		t.Fatal("buildAutoLabelPlan() accepted conflicting locked names")
	}
}

func TestBuildAutoLabelPlanAllowsMissingRedundantLeafLink(t *testing.T) {
	fabricNodes, switches, nodes := redundantTwoTierInputs()
	// node1's link to leaf2 is no longer reported. The remaining shared node2
	// link still identifies leaf1 and leaf2 as one redundant failure domain.
	fabricNodes[0].Status.ScaleOutNics = fabricNodes[0].Status.ScaleOutNics[:1]

	plan, err := buildAutoLabelPlan(v1beta1.SwitchRoleScaleOut, mustAutoTemplate(t), fabricNodes, switches, nodes)
	if err != nil {
		t.Fatalf("buildAutoLabelPlan() error = %v", err)
	}
	for _, nodeName := range []string{"node1", "node2"} {
		if got := plan.nodes[nodeName]["scale-out.unifabric.io/tier-1"]; got != "tier1-group1" {
			t.Fatalf("Node %s tier 1 = %q", nodeName, got)
		}
	}
	for _, switchName := range []string{"leaf1", "leaf2"} {
		if got := plan.switches[switchName][v1beta1.TopologyDomainLabel]; got != "tier1-group1" {
			t.Fatalf("Switch %s tier 1 = %q", switchName, got)
		}
	}
}

func TestSwitchInputPredicateIgnoresPureLabelUpdate(t *testing.T) {
	oldSwitch := &v1beta1.Switch{ObjectMeta: metav1.ObjectMeta{Name: "leaf1"}, Status: v1beta1.SwitchStatus{Healthy: true}}
	newSwitch := oldSwitch.DeepCopy()
	newSwitch.Labels = map[string]string{v1beta1.TopologyDomainLabel: "tier1-group1"}
	if switchTopologyInputPredicate().Update(event.UpdateEvent{ObjectOld: oldSwitch, ObjectNew: newSwitch}) {
		t.Fatal("pure Switch label update triggered auto discovery")
	}
	newSwitch.Status.Healthy = false
	if switchTopologyInputPredicate().Update(event.UpdateEvent{ObjectOld: oldSwitch, ObjectNew: newSwitch}) {
		t.Fatal("pure Switch health update triggered auto discovery")
	}
	newSwitch = oldSwitch.DeepCopy()
	newSwitch.Annotations = map[string]string{v1beta1.SwitchNeighborsAnnotation: ""}
	if !switchTopologyInputPredicate().Update(event.UpdateEvent{ObjectOld: oldSwitch, ObjectNew: newSwitch}) {
		t.Fatal("adding an empty manual Switch neighbor annotation did not trigger auto discovery")
	}
	newSwitch = oldSwitch.DeepCopy()
	newSwitch.Annotations = map[string]string{v1beta1.SwitchNeighborsAnnotation: `["spine1"]`}
	if !switchTopologyInputPredicate().Update(event.UpdateEvent{ObjectOld: oldSwitch, ObjectNew: newSwitch}) {
		t.Fatal("manual Switch neighbor update did not trigger auto discovery")
	}
}

func TestAutoLabelReconcileCreatesTopologyOnlyAfterDiscoveryFindsData(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	fabricNodes, switches, nodes := redundantTwoTierInputs()
	objects := make([]runtime.Object, 0, len(fabricNodes)+len(switches)+len(nodes))
	for index := range fabricNodes {
		objects = append(objects, &fabricNodes[index])
	}
	for index := range switches {
		objects = append(objects, &switches[index])
	}
	for index := range nodes {
		objects = append(objects, &nodes[index])
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
	storageTemplate, err := topologylabel.Compile("storage", "storage.unifabric.io/tier-{{ .Tier }}")
	if err != nil {
		t.Fatal(err)
	}
	r := &autoLabelReconciler{
		client: client,
		reader: client,
		managedTopologies: []managedTopology{
			{
				name:          v1beta1.TopologyScaleOut,
				role:          v1beta1.SwitchRoleScaleOut,
				labelTemplate: mustAutoTemplate(t),
			},
			{
				name:          v1beta1.TopologyStorage,
				role:          v1beta1.SwitchRoleStorage,
				labelTemplate: storageTemplate,
			},
		},
		recorder: record.NewFakeRecorder(10),
		log:      logger.MustNew(logger.LevelDebug),
	}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{}); err != nil {
		t.Fatal(err)
	}

	var storedNode corev1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Name: nodes[0].Name}, &storedNode); err != nil {
		t.Fatal(err)
	}
	if storedNode.Labels["scale-out.unifabric.io/tier-1"] == "" {
		t.Fatalf("Node labels were not populated: %#v", storedNode.Labels)
	}

	var topologyObject v1beta1.Topology
	if err := client.Get(context.Background(), types.NamespacedName{Name: v1beta1.TopologyScaleOut}, &topologyObject); err != nil {
		t.Fatalf("discovered Topology was not created: %v", err)
	}
	if !containsString(topologyObject.Finalizers, autoTopologyFinalizer) {
		t.Fatalf("Topology finalizers = %v", topologyObject.Finalizers)
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: v1beta1.TopologyStorage}, &topologyObject); !apierrors.IsNotFound(err) {
		t.Fatalf("empty storage Topology Get error = %v, want NotFound", err)
	}
}

func TestAutoLabelReconcileDoesNotCreateEmptyTopology(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &autoLabelReconciler{
		client: client,
		reader: client,
		managedTopologies: []managedTopology{{
			name:          v1beta1.TopologyScaleOut,
			role:          v1beta1.SwitchRoleScaleOut,
			labelTemplate: mustAutoTemplate(t),
		}},
		recorder: record.NewFakeRecorder(10),
		log:      logger.MustNew(logger.LevelDebug),
	}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{}); err != nil {
		t.Fatal(err)
	}

	var topologyObject v1beta1.Topology
	if err := client.Get(context.Background(), types.NamespacedName{Name: v1beta1.TopologyScaleOut}, &topologyObject); !apierrors.IsNotFound(err) {
		t.Fatalf("empty Topology Get error = %v, want NotFound", err)
	}
}

func TestResetTopologyClearsOnlyMatchingLabelsBeforeRemovingFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	now := metav1.Now()
	topologyObject := &v1beta1.Topology{ObjectMeta: metav1.ObjectMeta{
		Name:              v1beta1.TopologyScaleOut,
		Finalizers:        []string{autoTopologyFinalizer},
		DeletionTimestamp: &now,
	}}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: map[string]string{
		"scale-out.unifabric.io/tier-1": "tier1-group1",
		"keep.example.io/value":         "yes",
	}}}
	sw := &v1beta1.Switch{ObjectMeta: metav1.ObjectMeta{Name: "leaf-a", Labels: map[string]string{
		v1beta1.TopologyDomainLabel: "tier1-group1",
		"keep.example.io/value":     "yes",
	}}}
	storageSwitch := &v1beta1.Switch{
		ObjectMeta: metav1.ObjectMeta{Name: "storage-a", Labels: map[string]string{
			v1beta1.TopologyDomainLabel: "tier1-group8",
		}},
		Spec: v1beta1.SwitchSpec{Role: v1beta1.SwitchRoleStorage},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(topologyObject, node, sw, storageSwitch).Build()
	r := &autoLabelReconciler{client: client, reader: client, log: logger.MustNew(logger.LevelDebug)}
	if err := r.resetTopology(context.Background(), topologyObject.DeepCopy(), mustAutoTemplate(t), v1beta1.SwitchRoleScaleOut); err != nil {
		t.Fatal(err)
	}
	var storedNode corev1.Node
	if err := client.Get(context.Background(), types.NamespacedName{Name: node.Name}, &storedNode); err != nil {
		t.Fatal(err)
	}
	if _, exists := storedNode.Labels["scale-out.unifabric.io/tier-1"]; exists || storedNode.Labels["keep.example.io/value"] != "yes" {
		t.Fatalf("Node labels after reset = %#v", storedNode.Labels)
	}
	var storedSwitch v1beta1.Switch
	if err := client.Get(context.Background(), types.NamespacedName{Name: sw.Name}, &storedSwitch); err != nil {
		t.Fatal(err)
	}
	if _, exists := storedSwitch.Labels[v1beta1.TopologyDomainLabel]; exists || storedSwitch.Labels["keep.example.io/value"] != "yes" {
		t.Fatalf("Switch labels after reset = %#v", storedSwitch.Labels)
	}
	if err := client.Get(context.Background(), types.NamespacedName{Name: storageSwitch.Name}, &storedSwitch); err != nil {
		t.Fatal(err)
	}
	if storedSwitch.Labels[v1beta1.TopologyDomainLabel] != "tier1-group8" {
		t.Fatalf("scale-out reset changed Storage Switch labels: %#v", storedSwitch.Labels)
	}
}

func TestReleaseUnmanagedFinalizerPreservesLabels(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	object := &v1beta1.Topology{ObjectMeta: metav1.ObjectMeta{
		Name:       v1beta1.TopologyScaleOut,
		Finalizers: []string{autoTopologyFinalizer},
		Labels:     map[string]string{"keep.example.io/value": "yes"},
	}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(object).Build()
	r := &autoLabelReconciler{
		client: client,
		reader: client,
		managedTopologies: []managedTopology{{
			name: v1beta1.TopologyStorage,
		}},
		log: logger.MustNew(logger.LevelDebug),
	}
	released, err := r.releaseUnmanagedFinalizers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !released {
		t.Fatal("unmanaged finalizer was not released")
	}
	var stored v1beta1.Topology
	if err := client.Get(context.Background(), types.NamespacedName{Name: object.Name}, &stored); err != nil {
		t.Fatal(err)
	}
	if containsString(stored.Finalizers, autoTopologyFinalizer) {
		t.Fatalf("finalizers after release = %v", stored.Finalizers)
	}
	if stored.Labels["keep.example.io/value"] != "yes" {
		t.Fatalf("labels after release = %#v", stored.Labels)
	}
}

func redundantTwoTierInputs() ([]v1beta1.FabricNode, []v1beta1.Switch, []corev1.Node) {
	fabricNodes := []v1beta1.FabricNode{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"}, Status: v1beta1.FabricNodeStatus{NodeRole: v1beta1.NodeRoleGPU, ScaleOutNics: []v1beta1.NicInfo{
			{State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "leaf1", Port: "Ethernet1"}},
			{State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "leaf2", Port: "Ethernet1"}},
		}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node2"}, Status: v1beta1.FabricNodeStatus{NodeRole: v1beta1.NodeRoleGPU, ScaleOutNics: []v1beta1.NicInfo{
			{State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "leaf1", Port: "Ethernet2"}},
			{State: "up", LLDPNeighbor: v1beta1.LLDPNeighbor{Hostname: "leaf2", Port: "Ethernet2"}},
		}}},
	}
	switches := []v1beta1.Switch{
		{ObjectMeta: metav1.ObjectMeta{Name: "leaf1"}, Status: v1beta1.SwitchStatus{Healthy: true, LLDPNeighbors: []v1beta1.SwitchNeighbor{
			{RemoteSystemName: "spine1", RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeSwitch},
			{RemoteSystemName: "spine2", RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeSwitch},
		}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "leaf2"}, Status: v1beta1.SwitchStatus{Healthy: true, LLDPNeighbors: []v1beta1.SwitchNeighbor{
			{RemoteSystemName: "spine1", RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeSwitch},
			{RemoteSystemName: "spine2", RemoteSystemType: v1beta1.SwitchLLDPRemoteSystemTypeSwitch},
		}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "spine1"}, Status: v1beta1.SwitchStatus{Healthy: true}},
		{ObjectMeta: metav1.ObjectMeta{Name: "spine2"}, Status: v1beta1.SwitchStatus{Healthy: true}},
	}
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
	}
	return fabricNodes, switches, nodes
}
