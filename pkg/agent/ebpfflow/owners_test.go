// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func boolPtr(v bool) *bool {
	return &v
}

// countingReader counts Get calls so tests can assert on caching.
type countingReader struct {
	client.Reader
	gets int
}

func (c *countingReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	c.gets++
	return c.Reader.Get(ctx, key, obj, opts...)
}

func ownerObject(apiVersion, kind, namespace, name, uid string, refs ...metav1.OwnerReference) *unstructured.Unstructured {
	object := &unstructured.Unstructured{}
	object.SetAPIVersion(apiVersion)
	object.SetKind(kind)
	object.SetNamespace(namespace)
	object.SetName(name)
	object.SetUID(types.UID(uid))
	object.SetOwnerReferences(refs)
	return object
}

func fakeOwnerReader(t *testing.T, objects ...*unstructured.Unstructured) *countingReader {
	t.Helper()
	scheme := runtime.NewScheme()
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, object := range objects {
		builder = builder.WithRuntimeObjects(object)
	}
	return &countingReader{Reader: builder.Build()}
}

func TestTopOwnerWalksControllerChain(t *testing.T) {
	reader := fakeOwnerReader(t,
		ownerObject("apps/v1", "ReplicaSet", "tenant-a", "web-7f9c", "rs-uid", metav1.OwnerReference{
			APIVersion: "apps/v1", Kind: "Deployment", Name: "web",
			UID: "deploy-uid", Controller: boolPtr(true),
		}),
		ownerObject("apps/v1", "Deployment", "tenant-a", "web", "deploy-uid"),
	)

	o := newOwnerResolver(reader)
	refs := []metav1.OwnerReference{{
		APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "web-7f9c",
		UID: "rs-uid", Controller: boolPtr(true),
	}}
	want := ownerInfo{Kind: "Deployment", Namespace: "tenant-a", Name: "web"}
	got := o.topOwner(context.Background(), "tenant-a", "web-7f9c-abcde", refs)
	if got != want {
		t.Fatalf("topOwner = %+v, want %+v", got, want)
	}
	if reader.gets != 2 {
		t.Fatalf("first lookup issued %d requests, want 2", reader.gets)
	}
	got = o.topOwner(context.Background(), "tenant-a", "web-7f9c-zzzzz", refs)
	if got != want {
		t.Fatalf("cached topOwner = %+v, want %+v", got, want)
	}
	if reader.gets != 2 {
		t.Fatalf("second lookup issued %d extra requests, want 0", reader.gets-2)
	}
}

func TestTopOwnerFollowsKubeflowJob(t *testing.T) {
	reader := fakeOwnerReader(t,
		ownerObject("kubeflow.org/v1", "PyTorchJob", "tenant-a", "train", "job-uid"),
	)

	o := newOwnerResolver(reader)
	refs := []metav1.OwnerReference{{
		APIVersion: "kubeflow.org/v1", Kind: "PyTorchJob", Name: "train",
		UID: "job-uid", Controller: boolPtr(true),
	}}
	want := ownerInfo{Kind: "PyTorchJob", Namespace: "tenant-a", Name: "train"}
	got := o.topOwner(context.Background(), "tenant-a", "train-worker-0", refs)
	if got != want {
		t.Fatalf("topOwner = %+v, want %+v", got, want)
	}
}

func TestTopOwnerWithoutReferencesIsPodItself(t *testing.T) {
	o := newOwnerResolver(nil)
	want := ownerInfo{Kind: "Pod", Namespace: "tenant-a", Name: "standalone"}
	got := o.topOwner(context.Background(), "tenant-a", "standalone", nil)
	if got != want {
		t.Fatalf("topOwner = %+v, want %+v", got, want)
	}
}

func TestTopOwnerStopsAtUnknownKindWithoutRequest(t *testing.T) {
	reader := fakeOwnerReader(t)

	o := newOwnerResolver(reader)
	refs := []metav1.OwnerReference{{
		APIVersion: "argoproj.io/v1alpha1", Kind: "Workflow", Name: "pipeline",
		UID: "wf-uid", Controller: boolPtr(true),
	}}
	want := ownerInfo{Kind: "Workflow", Namespace: "tenant-a", Name: "pipeline"}
	got := o.topOwner(context.Background(), "tenant-a", "pipeline-step-1", refs)
	if got != want {
		t.Fatalf("topOwner = %+v, want %+v", got, want)
	}
	if reader.gets != 0 {
		t.Fatalf("unknown kind issued %d requests, want 0", reader.gets)
	}
}

func TestTopOwnerStopsWhenFetchFails(t *testing.T) {
	reader := fakeOwnerReader(t)

	o := newOwnerResolver(reader)
	refs := []metav1.OwnerReference{{
		APIVersion: "apps/v1", Kind: "ReplicaSet", Name: "web-7f9c",
		UID: "rs-uid", Controller: boolPtr(true),
	}}
	want := ownerInfo{Kind: "ReplicaSet", Namespace: "tenant-a", Name: "web-7f9c"}
	got := o.topOwner(context.Background(), "tenant-a", "web-7f9c-abcde", refs)
	if got != want {
		t.Fatalf("topOwner = %+v, want %+v", got, want)
	}
}

func TestTopOwnerClusterScopedOwnerHasNoNamespace(t *testing.T) {
	reader := fakeOwnerReader(t)

	o := newOwnerResolver(reader)
	refs := []metav1.OwnerReference{{APIVersion: "v1", Kind: "Node", Name: "sh-cube-g1-03", UID: "node-uid"}}
	want := ownerInfo{Kind: "Node", Namespace: "", Name: "sh-cube-g1-03"}
	got := o.topOwner(context.Background(), "kube-system", "kube-apiserver-sh-cube-g1-03", refs)
	if got != want {
		t.Fatalf("topOwner = %+v, want %+v", got, want)
	}
	if reader.gets != 0 {
		t.Fatalf("cluster-scoped owner issued %d requests, want 0", reader.gets)
	}
}
