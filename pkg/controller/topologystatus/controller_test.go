// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package topologystatus

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

func TestReconcileAtomicallyPublishesStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	objects := []runtime.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: topologyLabels("rack-a", "row-a")}},
		&v1beta1.Switch{ObjectMeta: metav1.ObjectMeta{Name: "leaf-a", Labels: topologyDomain("rack-a")}},
		&v1beta1.Switch{ObjectMeta: metav1.ObjectMeta{Name: "spine-a", Labels: topologyDomain("row-a")}},
		&v1beta1.Switch{ObjectMeta: metav1.ObjectMeta{Name: "storage-a", Labels: topologyDomain("rack-a")}, Spec: v1beta1.SwitchSpec{Role: v1beta1.SwitchRoleStorage}},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).WithStatusSubresource(&v1beta1.Topology{}).Build()
	templates, err := topologylabel.CompileSet(
		"scale-up.unifabric.io/tier-{{ .Tier }}",
		"scale-out.unifabric.io/tier-{{ .Tier }}",
		"storage.unifabric.io/tier-{{ .Tier }}",
	)
	if err != nil {
		t.Fatal(err)
	}
	r := &Reconciler{client: client, templates: templates, recorder: record.NewFakeRecorder(10), log: logger.MustNew(logger.LevelDebug)}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: v1beta1.TopologyScaleOut}}); err != nil {
		t.Fatal(err)
	}
	var stored v1beta1.Topology
	if err := client.Get(context.Background(), types.NamespacedName{Name: v1beta1.TopologyScaleOut}, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored.Status.Domains) != 2 || len(stored.Status.Nodes) != 1 {
		t.Fatalf("status = %#v", stored.Status)
	}
	if len(stored.Status.Domains[0].Members) != 1 || stored.Status.Domains[0].Members[0] != "spine-a" {
		t.Fatalf("tier 2 members = %#v", stored.Status.Domains[0].Members)
	}
	if len(stored.Status.Domains[1].Members) != 1 || stored.Status.Domains[1].Members[0] != "leaf-a" {
		t.Fatalf("tier 1 members = %#v", stored.Status.Domains[1].Members)
	}
	for _, name := range []string{v1beta1.TopologyScaleUp, v1beta1.TopologyStorage} {
		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: name}}); err != nil {
			t.Fatalf("Reconcile(%s) error = %v", name, err)
		}
		if err := client.Get(context.Background(), types.NamespacedName{Name: name}, &stored); !apierrors.IsNotFound(err) {
			t.Fatalf("Topology/%s Get error = %v, want NotFound", name, err)
		}
	}
}

func TestReconcileSkipsMissingTopologiesWithoutData(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	templates, err := topologylabel.CompileSet(
		"scale-up.unifabric.io/tier-{{ .Tier }}",
		"scale-out.unifabric.io/tier-{{ .Tier }}",
		"storage.unifabric.io/tier-{{ .Tier }}",
	)
	if err != nil {
		t.Fatal(err)
	}
	r := &Reconciler{client: client, templates: templates, recorder: record.NewFakeRecorder(10), log: logger.MustNew(logger.LevelDebug)}
	for _, name := range v1beta1.FixedTopologyNames {
		if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: name}}); err != nil {
			t.Fatalf("Reconcile(%s) error = %v", name, err)
		}
		var object v1beta1.Topology
		if err := client.Get(context.Background(), types.NamespacedName{Name: name}, &object); !apierrors.IsNotFound(err) {
			t.Fatalf("Topology/%s Get error = %v, want NotFound", name, err)
		}
	}
}

func TestReconcileRetainsExistingTopologyWhenDataBecomesEmpty(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	object := &v1beta1.Topology{
		ObjectMeta: metav1.ObjectMeta{Name: v1beta1.TopologyScaleOut},
		Status: v1beta1.TopologyStatus{
			Domains: []v1beta1.TopologyDomain{{Name: "rack-a", Tier: 1}},
			Nodes:   []v1beta1.TopologyNodeGroup{{Nodes: []string{"node-a"}, DomainPath: []string{"rack-a"}}},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(object).WithStatusSubresource(&v1beta1.Topology{}).Build()
	templates, err := topologylabel.CompileSet(
		"scale-up.unifabric.io/tier-{{ .Tier }}",
		"scale-out.unifabric.io/tier-{{ .Tier }}",
		"storage.unifabric.io/tier-{{ .Tier }}",
	)
	if err != nil {
		t.Fatal(err)
	}
	r := &Reconciler{client: client, templates: templates, recorder: record.NewFakeRecorder(10), log: logger.MustNew(logger.LevelDebug)}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: object.Name}}); err != nil {
		t.Fatal(err)
	}

	var stored v1beta1.Topology
	if err := client.Get(context.Background(), types.NamespacedName{Name: object.Name}, &stored); err != nil {
		t.Fatalf("existing Topology was removed: %v", err)
	}
	if !topologyStatusEmpty(stored.Status) {
		t.Fatalf("status after empty snapshot = %#v", stored.Status)
	}
}

func TestTopologyLabelPredicateWatchesSwitchRoleChange(t *testing.T) {
	templates, err := topologylabel.CompileSet(
		"scale-up.unifabric.io/tier-{{ .Tier }}",
		"scale-out.unifabric.io/tier-{{ .Tier }}",
		"storage.unifabric.io/tier-{{ .Tier }}",
	)
	if err != nil {
		t.Fatal(err)
	}
	oldSwitch := &v1beta1.Switch{
		ObjectMeta: metav1.ObjectMeta{Name: "switch-a", Labels: topologyDomain("rack-a")},
		Spec:       v1beta1.SwitchSpec{Role: v1beta1.SwitchRoleScaleOut},
	}
	newSwitch := oldSwitch.DeepCopy()
	newSwitch.Spec.Role = v1beta1.SwitchRoleStorage
	if !topologyLabelPredicate(templates).Update(event.UpdateEvent{ObjectOld: oldSwitch, ObjectNew: newSwitch}) {
		t.Fatal("Switch role change did not trigger topology status reconciliation")
	}
}
