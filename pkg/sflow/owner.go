// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type OwnerResolver interface {
	OwnerForPod(pod *corev1.Pod) *OwnerRef
}

type KubernetesOwnerResolver struct {
	Client client.Reader
}

func (r KubernetesOwnerResolver) OwnerForPod(pod *corev1.Pod) *OwnerRef {
	if r.Client == nil || pod == nil {
		return nil
	}
	return getTopOwner(context.Background(), r.Client, pod)
}

func getTopOwner(ctx context.Context, c client.Reader, obj metav1.Object) *OwnerRef {
	var lastOwnerRef *OwnerRef
	for {
		ownerRefs := obj.GetOwnerReferences()
		if len(ownerRefs) == 0 {
			return lastOwnerRef
		}
		ownerRef := ownerRefs[0]
		ownerObj := &unstructured.Unstructured{}
		ownerObj.SetAPIVersion(ownerRef.APIVersion)
		ownerObj.SetKind(ownerRef.Kind)
		if err := c.Get(ctx, types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      ownerRef.Name,
		}, ownerObj); err != nil {
			return lastOwnerRef
		}
		lastOwnerRef = &OwnerRef{
			APIVersion: ownerRef.APIVersion,
			Kind:       ownerRef.Kind,
			Namespace:  obj.GetNamespace(),
			Name:       ownerRef.Name,
		}
		obj = ownerObj
	}
}
