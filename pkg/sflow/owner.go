// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const maxOwnerResolutionDepth = 32

type OwnerResolver interface {
	OwnerForPod(ctx context.Context, pod *corev1.Pod) *OwnerRef
}

type KubernetesOwnerResolver struct {
	Client client.Reader
}

func (r KubernetesOwnerResolver) OwnerForPod(ctx context.Context, pod *corev1.Pod) *OwnerRef {
	if r.Client == nil || pod == nil {
		return nil
	}
	return getTopOwner(ctx, r.Client, pod)
}

func getTopOwner(ctx context.Context, c client.Reader, obj metav1.Object) *OwnerRef {
	var lastOwnerRef *OwnerRef
	seen := make(map[string]struct{})
	for depth := 0; depth < maxOwnerResolutionDepth; depth++ {
		select {
		case <-ctx.Done():
			return lastOwnerRef
		default:
		}

		ownerRefs := obj.GetOwnerReferences()
		if len(ownerRefs) == 0 {
			return lastOwnerRef
		}
		ownerRef := ownerRefs[0]
		key := ownerRefKey(obj.GetNamespace(), ownerRef)
		if _, ok := seen[key]; ok {
			return lastOwnerRef
		}
		seen[key] = struct{}{}

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
	return lastOwnerRef
}

func ownerRefKey(namespace string, ownerRef metav1.OwnerReference) string {
	return fmt.Sprintf("%s/%s/%s/%s", namespace, ownerRef.APIVersion, ownerRef.Kind, ownerRef.Name)
}
