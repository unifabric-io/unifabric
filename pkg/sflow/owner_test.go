// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetTopOwnerReturnsOnOwnerCycle(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apps scheme: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "workloads",
			Name:      "trainer-0",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "trainer-rs",
				},
			},
		},
	}
	replicaSet := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "workloads",
			Name:      "trainer-rs",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "trainer",
				},
			},
		},
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "workloads",
			Name:      "trainer",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "trainer-rs",
				},
			},
		},
	}
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pod, replicaSet, deployment).
		Build()

	owner := getTopOwner(context.Background(), client, pod)
	if owner == nil || owner.Kind != "Deployment" || owner.Name != "trainer" {
		t.Fatalf("owner = %#v, want Deployment/trainer", owner)
	}
}

func TestKubernetesOwnerResolverUsesCanceledContext(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add apps scheme: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "workloads",
			Name:      "trainer-0",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "trainer-rs",
				},
			},
		},
	}
	replicaSet := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "workloads",
			Name:      "trainer-rs",
		},
	}
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pod, replicaSet).
		Build()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	owner := KubernetesOwnerResolver{Client: client}.OwnerForPod(ctx, pod)
	if owner != nil {
		t.Fatalf("owner = %#v, want nil after context cancellation", owner)
	}
}
