// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"context"
	"log/slog"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ownerInfo is the top-level owner of a Pod, the Pod itself when it has none.
type ownerInfo struct {
	Kind      string
	Namespace string
	Name      string
}

// knownOwnerResources lists, by apiVersion and kind, the owner types the
// agent keeps following upwards, with their plural resource names. All are
// namespaced. Kinds outside the table stop the walk at that level using only
// the kind and name from the reference, without a request.
// Changing the table requires a matching entry in
// chart/values.yaml agent.workloadOwnerRules.
var knownOwnerResources = map[string]map[string]string{
	"apps/v1": {
		"Deployment":  "deployments",
		"ReplicaSet":  "replicasets",
		"StatefulSet": "statefulsets",
		"DaemonSet":   "daemonsets",
	},
	"batch/v1": {
		"Job":     "jobs",
		"CronJob": "cronjobs",
	},
	"kubeflow.org/v1": {
		"PyTorchJob": "pytorchjobs",
		"TFJob":      "tfjobs",
		"MPIJob":     "mpijobs",
		"XGBoostJob": "xgboostjobs",
		"PaddleJob":  "paddlejobs",
		"JAXJob":     "jaxjobs",
	},
	"kubeflow.org/v2beta1": {
		"MPIJob": "mpijobs",
	},
	"trainer.kubeflow.org/v1alpha1": {
		"TrainJob": "trainjobs",
	},
	"jobset.x-k8s.io/v1alpha2": {
		"JobSet": "jobsets",
	},
	"leaderworkerset.x-k8s.io/v1": {
		"LeaderWorkerSet": "leaderworkersets",
	},
	"ray.io/v1": {
		"RayCluster": "rayclusters",
		"RayJob":     "rayjobs",
		"RayService": "rayservices",
	},
	"batch.volcano.sh/v1alpha1": {
		"Job": "jobs",
	},
}

// clusterScopedOwnerKinds are owner kinds known to be cluster scoped that
// are not fetched, the namespace is cleared when one is met. Mirror Pods of
// static Pods are owned by their Node.
var clusterScopedOwnerKinds = map[string]bool{
	"Node": true,
}

// maxOwnerDepth only guards against ownerReferences cycles, real chains are
// far shorter.
const maxOwnerDepth = 8

// ownerResolver walks ownerReferences up to the top-level owner. Fetched
// objects are cached by UID because an owner chain rarely changes once built.
type ownerResolver struct {
	reader  client.Reader
	objects map[types.UID][]metav1.OwnerReference
}

// newOwnerResolver builds a resolver reading through reader, which should be
// the uncached API reader so arbitrary owner kinds do not start informers.
// A nil reader stops every walk at the first reference.
func newOwnerResolver(reader client.Reader) *ownerResolver {
	return &ownerResolver{
		reader:  reader,
		objects: map[types.UID][]metav1.OwnerReference{},
	}
}

// topOwner returns the top-level owner of the Pod name in namespace. On an
// unknown kind or a failed fetch it stops at the highest level known so far.
func (o *ownerResolver) topOwner(ctx context.Context, namespace, name string, refs []metav1.OwnerReference) ownerInfo {
	current := ownerInfo{Kind: "Pod", Namespace: namespace, Name: name}
	for range maxOwnerDepth {
		ref, ok := controllerRef(refs)
		if !ok {
			return current
		}
		current = ownerInfo{Kind: ref.Kind, Namespace: namespace, Name: ref.Name}
		if clusterScopedOwnerKinds[ref.Kind] {
			current.Namespace = ""
			return current
		}
		_, known := knownOwnerResources[ref.APIVersion][ref.Kind]
		if !known {
			logDedup("owner_kind_unknown:"+ref.APIVersion+"/"+ref.Kind, slog.LevelDebug,
				"owner_kind_not_in_known_list", "api_version", ref.APIVersion, "kind", ref.Kind,
				"action", "stop_at_this_level")
			return current
		}
		if o == nil || o.reader == nil {
			return current
		}
		next, err := o.fetch(ctx, ref, namespace)
		if err != nil {
			logDedup("owner_object_fetch:"+string(ref.UID), slog.LevelDebug,
				"owner_object_fetch_failed", "kind", ref.Kind, "name", ref.Name, "reason", err)
			return current
		}
		refs = next
	}
	return current
}

// controllerRef prefers the reference with controller=true, otherwise the
// first one.
func controllerRef(refs []metav1.OwnerReference) (metav1.OwnerReference, bool) {
	if len(refs) == 0 {
		return metav1.OwnerReference{}, false
	}
	for _, ref := range refs {
		if ref.Controller != nil && *ref.Controller {
			return ref, true
		}
	}
	return refs[0], true
}

// fetch returns the ownerReferences of the object ref points at.
func (o *ownerResolver) fetch(ctx context.Context, ref metav1.OwnerReference,
	namespace string) ([]metav1.OwnerReference, error) {
	refs, cached := o.objects[ref.UID]
	if cached {
		return refs, nil
	}
	object := &unstructured.Unstructured{}
	object.SetAPIVersion(ref.APIVersion)
	object.SetKind(ref.Kind)
	err := o.reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, object)
	if err != nil {
		return nil, err
	}
	refs = object.GetOwnerReferences()
	o.objects[ref.UID] = refs
	return refs, nil
}
