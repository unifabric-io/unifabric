// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package fabricnode

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newOwnerReferenceTestReconciler(t *testing.T, storageNode bool, objects ...runtime.Object) *fabricNodeReconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return &fabricNodeReconciler{
		nodeName:    "node-a",
		client:      fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build(),
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		storageNode: storageNode,
	}
}

func TestEnsureNodeOwnerReferenceSetsOwnerWhenNodeExists(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: types.UID("node-a-uid")}}
	r := newOwnerReferenceTestReconciler(t, false, node)
	fabricNode := &v1beta1.FabricNode{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}

	changed, err := r.ensureNodeOwnerReference(context.Background(), fabricNode)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("ensureNodeOwnerReference() changed = false, want true")
	}
	if len(fabricNode.OwnerReferences) != 1 || fabricNode.OwnerReferences[0].UID != node.UID {
		t.Fatalf("OwnerReferences = %#v, want owner UID %q", fabricNode.OwnerReferences, node.UID)
	}
}

func TestEnsureNodeOwnerReferenceLeavesUnsetWhenNodeMissing(t *testing.T) {
	r := newOwnerReferenceTestReconciler(t, false)
	fabricNode := &v1beta1.FabricNode{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}

	changed, err := r.ensureNodeOwnerReference(context.Background(), fabricNode)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("ensureNodeOwnerReference() changed = true, want false")
	}
	if len(fabricNode.OwnerReferences) != 0 {
		t.Fatalf("OwnerReferences = %#v, want none (no dangling empty-UID reference)", fabricNode.OwnerReferences)
	}
}

func TestEnsureNodeOwnerReferenceSkipsStorageNode(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a", UID: types.UID("node-a-uid")}}
	r := newOwnerReferenceTestReconciler(t, true, node)
	fabricNode := &v1beta1.FabricNode{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}}

	changed, err := r.ensureNodeOwnerReference(context.Background(), fabricNode)
	if err != nil {
		t.Fatal(err)
	}
	if changed || len(fabricNode.OwnerReferences) != 0 {
		t.Fatalf("storage node should never get an owner reference, got changed=%v refs=%#v", changed, fabricNode.OwnerReferences)
	}
}

func TestEnsureNodeOwnerReferenceIsNoopWhenAlreadySet(t *testing.T) {
	r := newOwnerReferenceTestReconciler(t, false)
	fabricNode := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "node-a",
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "v1", Kind: "Node", Name: "node-a", UID: types.UID("existing")}},
		},
	}

	changed, err := r.ensureNodeOwnerReference(context.Background(), fabricNode)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("ensureNodeOwnerReference() changed = true, want false (already set, no Node lookup needed)")
	}
}
