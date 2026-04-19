// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package fabricnode

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
)

const NetworkStatusAnnotation = "k8s.v1.cni.cncf.io/network-status"

func isSriovRDMA(pod *corev1.Pod) bool {
	status := pod.Annotations[NetworkStatusAnnotation]
	return strings.Contains(status, "rdma-device")
}

func getContainerList(pod *corev1.Pod) []string {
	var containerList []string
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.ContainerID != "" {
			containerList = append(containerList, containerStatus.ContainerID)
		}
	}
	return containerList
}

func isHostRDMA(pod *corev1.Pod) bool {
	// Check if pod is running in host RDMA mode
	if !pod.Spec.HostNetwork {
		return false
	}

	// Check if any container has privileged mode enabled and /dev/infiniband mounted
	for _, container := range pod.Spec.Containers {
		// Check privileged mode
		if container.SecurityContext == nil || container.SecurityContext.Privileged == nil || !*container.SecurityContext.Privileged {
			continue
		}

		// Check /dev/infiniband mount
		for _, mount := range container.VolumeMounts {
			if mount.MountPath == "/dev/infiniband" {
				return true
			}
		}
	}

	return false
}

func isRdmaPodsEqual(old, new []v1beta1.RdmaPod) bool {
	if len(old) != len(new) {
		return false
	}

	// Create a map of old pods by namespace/name
	oldPods := make(map[string]v1beta1.RdmaPod)
	for _, pod := range old {
		key := pod.Namespace + "/" + pod.Name
		oldPods[key] = pod
	}

	// Check each new pod against the map
	for _, pod := range new {
		key := pod.Namespace + "/" + pod.Name
		oldPod, exists := oldPods[key]
		if !exists {
			return false
		}

		// Compare pod fields
		if oldPod.HostRDMA != pod.HostRDMA {
			return false
		}

		// Compare container lists
		if !isContainerListEqual(oldPod.ContainerList, pod.ContainerList) {
			return false
		}

		// Compare TopOwner
		if !isOwnerRefEqual(oldPod.TopOwner, pod.TopOwner) {
			return false
		}
	}

	return true
}

func isContainerListEqual(old, new []string) bool {
	if len(old) != len(new) {
		return false
	}

	// Create a map of old container IDs
	oldContainers := make(map[string]struct{})
	for _, id := range old {
		oldContainers[id] = struct{}{}
	}

	// Check each new container ID
	for _, id := range new {
		if _, exists := oldContainers[id]; !exists {
			return false
		}
	}

	return true
}

func isOwnerRefEqual(a, b *v1beta1.OwnerRef) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.APIVersion == b.APIVersion &&
		a.Kind == b.Kind &&
		a.Namespace == b.Namespace &&
		a.Name == b.Name
}

func getTopOwner(ctx context.Context, c client.Client, obj metav1.Object) *v1beta1.OwnerRef {
	var lastOwnerRef *v1beta1.OwnerRef
	for {
		ownerRefs := obj.GetOwnerReferences()
		if len(ownerRefs) == 0 {
			// return the last successfully retrieved ownerRef (or nil if none)
			return lastOwnerRef
		}
		ownerRef := ownerRefs[0]
		ownerObj := &unstructured.Unstructured{}
		ownerObj.SetAPIVersion(ownerRef.APIVersion)
		ownerObj.SetKind(ownerRef.Kind)
		err := c.Get(ctx, types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      ownerRef.Name,
		}, ownerObj)
		if err != nil {
			// return nil if failed to get owner
			return nil
		}
		lastOwnerRef = &v1beta1.OwnerRef{
			APIVersion: ownerRef.APIVersion,
			Kind:       ownerRef.Kind,
			Namespace:  obj.GetNamespace(),
			Name:       ownerRef.Name,
		}
		obj = ownerObj
	}
}

func isPodRunning(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodRunning
}

func (r *fabricNodeReconciler) updateRdmaPodsStatus(ctx context.Context, status *v1beta1.FabricNodeStatus) (bool, error) {
	podList := &corev1.PodList{}
	err := r.client.List(ctx, podList, client.MatchingFields{"spec.nodeName": r.nodeName})
	if err != nil {
		r.Log.Error("failed to list pods on node", "error", err)
		return false, err
	}

	var rdmaPods []v1beta1.RdmaPod
	for _, pod := range podList.Items {
		if isPodRunning(&pod) && (isSriovRDMA(&pod) || isHostRDMA(&pod)) {
			rdmaPod := v1beta1.RdmaPod{
				Namespace:     pod.Namespace,
				Name:          pod.Name,
				ContainerList: getContainerList(&pod),
				HostRDMA:      isHostRDMA(&pod),
				TopOwner:      getTopOwner(ctx, r.client, &pod),
			}
			rdmaPods = append(rdmaPods, rdmaPod)
		}
	}

	if isRdmaPodsEqual(status.RdmaPods, rdmaPods) {
		return false, nil
	}
	status.RdmaPods = rdmaPods
	return true, nil
}
