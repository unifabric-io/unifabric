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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func fabricNodeTopologyTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func TestFabricNodeTopologyReconcileSetsMembership(t *testing.T) {
	templates, err := topologylabel.CompileSet(
		"scale-up.unifabric.io/tier-{{ .Tier }}",
		"scale-out.unifabric.io/tier-{{ .Tier }}",
		"storage.unifabric.io/tier-{{ .Tier }}",
	)
	if err != nil {
		t.Fatal(err)
	}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "node-a",
		Labels: topologyLabels("rack-a", "row-a"),
	}}
	fabricNode := &v1beta1.FabricNode{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}

	c := fake.NewClientBuilder().
		WithScheme(fabricNodeTopologyTestScheme(t)).
		WithObjects(node, fabricNode).
		WithStatusSubresource(&v1beta1.FabricNode{}).
		Build()

	r := &fabricNodeTopologyReconciler{client: c, templates: templates, log: logger.MustNew(logger.LevelDebug)}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a"}}); err != nil {
		t.Fatal(err)
	}

	var stored v1beta1.FabricNode
	if err := c.Get(context.Background(), types.NamespacedName{Name: "node-a"}, &stored); err != nil {
		t.Fatal(err)
	}
	if got := stored.Status.Topologies; len(got) != 1 || got[0] != v1beta1.TopologyScaleOut {
		t.Fatalf("status.topologies = %#v, want [%q]", got, v1beta1.TopologyScaleOut)
	}
}

func TestFabricNodeTopologyReconcileClearsMembership(t *testing.T) {
	templates, err := topologylabel.CompileSet(
		"scale-up.unifabric.io/tier-{{ .Tier }}",
		"scale-out.unifabric.io/tier-{{ .Tier }}",
		"storage.unifabric.io/tier-{{ .Tier }}",
	)
	if err != nil {
		t.Fatal(err)
	}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}
	fabricNode := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status:     v1beta1.FabricNodeStatus{Topologies: []string{v1beta1.TopologyScaleOut}},
	}

	c := fake.NewClientBuilder().
		WithScheme(fabricNodeTopologyTestScheme(t)).
		WithObjects(node, fabricNode).
		WithStatusSubresource(&v1beta1.FabricNode{}).
		Build()

	r := &fabricNodeTopologyReconciler{client: c, templates: templates, log: logger.MustNew(logger.LevelDebug)}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a"}}); err != nil {
		t.Fatal(err)
	}

	var stored v1beta1.FabricNode
	if err := c.Get(context.Background(), types.NamespacedName{Name: "node-a"}, &stored); err != nil {
		t.Fatal(err)
	}
	if got := stored.Status.Topologies; got != nil {
		t.Fatalf("status.topologies = %#v, want nil", got)
	}
}

func TestFabricNodeTopologyReconcileNoFabricNodeIsNoop(t *testing.T) {
	templates, err := topologylabel.CompileSet(
		"scale-up.unifabric.io/tier-{{ .Tier }}",
		"scale-out.unifabric.io/tier-{{ .Tier }}",
		"storage.unifabric.io/tier-{{ .Tier }}",
	)
	if err != nil {
		t.Fatal(err)
	}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", Labels: topologyLabels("rack-a", "row-a")}}

	c := fake.NewClientBuilder().
		WithScheme(fabricNodeTopologyTestScheme(t)).
		WithObjects(node).
		WithStatusSubresource(&v1beta1.FabricNode{}).
		Build()

	r := &fabricNodeTopologyReconciler{client: c, templates: templates, log: logger.MustNew(logger.LevelDebug)}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "node-a"}}); err != nil {
		t.Fatal(err)
	}
}

func TestMatchingTopologies(t *testing.T) {
	templates, err := topologylabel.CompileSet(
		"scale-up.unifabric.io/tier-{{ .Tier }}",
		"scale-out.unifabric.io/tier-{{ .Tier }}",
		"storage.unifabric.io/tier-{{ .Tier }}",
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		labels map[string]string
		want   []string
	}{
		{name: "no labels", labels: nil, want: nil},
		{name: "scale-out only", labels: topologyLabels("rack-a", "row-a"), want: []string{v1beta1.TopologyScaleOut}},
		{name: "scale-out and storage", labels: map[string]string{
			"scale-out.unifabric.io/tier-1": "rack-a",
			"storage.unifabric.io/tier-1":   "storage-a",
		}, want: []string{v1beta1.TopologyScaleOut, v1beta1.TopologyStorage}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchingTopologies(tt.labels, templates)
			if len(got) != len(tt.want) {
				t.Fatalf("matchingTopologies() = %#v, want %#v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("matchingTopologies() = %#v, want %#v", got, tt.want)
				}
			}
		})
	}
}
