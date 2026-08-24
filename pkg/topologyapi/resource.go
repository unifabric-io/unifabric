// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package topologyapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// resolveKind resolves the {cluster} path segment to a client and matches the
// {kind} segment case-insensitively against the types registered in that
// client's scheme. Only this project's group and version are accepted so the
// unauthenticated API can never serve foreign resources.
func (h *handler) resolveKind(w http.ResponseWriter, r *http.Request) (client.Client, schema.GroupVersionKind, bool) {
	cluster := r.PathValue("cluster")
	c, err := h.clusters.GetClusterClient(cluster)
	if err != nil {
		http.Error(w, fmt.Sprintf("unknown cluster %q", cluster), http.StatusNotFound)
		return nil, schema.GroupVersionKind{}, false
	}

	gv := schema.GroupVersion{Group: r.PathValue("group"), Version: r.PathValue("version")}
	if gv != v1beta1.GroupVersion {
		http.Error(w, fmt.Sprintf("unknown API group/version %q", gv.String()), http.StatusNotFound)
		return nil, schema.GroupVersionKind{}, false
	}

	kind := r.PathValue("kind")
	for known := range c.Scheme().KnownTypes(gv) {
		if strings.EqualFold(known, kind) && isGettable(c.Scheme(), gv.WithKind(known)) {
			return c, gv.WithKind(known), true
		}
	}
	http.Error(w, fmt.Sprintf("unknown kind %q", kind), http.StatusNotFound)
	return nil, schema.GroupVersionKind{}, false
}

// isGettable filters out the list and option types that share the group
// version in the scheme but are not addressable objects.
func isGettable(scheme *runtime.Scheme, gvk schema.GroupVersionKind) bool {
	obj, err := scheme.New(gvk)
	if err != nil {
		return false
	}
	_, ok := obj.(client.Object)
	return ok
}

// newObject instantiates the typed object registered in the scheme for gvk.
func newObject(scheme *runtime.Scheme, gvk schema.GroupVersionKind) (client.Object, error) {
	obj, err := scheme.New(gvk)
	if err != nil {
		return nil, err
	}
	clientObj, ok := obj.(client.Object)
	if !ok {
		return nil, fmt.Errorf("%s is not a client.Object", gvk.Kind)
	}
	return clientObj, nil
}

// newList instantiates the list type registered in the scheme for gvk.
func newList(scheme *runtime.Scheme, gvk schema.GroupVersionKind) (client.ObjectList, error) {
	obj, err := scheme.New(gvk.GroupVersion().WithKind(gvk.Kind + "List"))
	if err != nil {
		return nil, err
	}
	list, ok := obj.(client.ObjectList)
	if !ok {
		return nil, fmt.Errorf("%sList is not a client.ObjectList", gvk.Kind)
	}
	return list, nil
}

// handleList serves every object of the requested kind as the standard
// Kubernetes list JSON shape.
func (h *handler) handleList(w http.ResponseWriter, r *http.Request) {
	c, gvk, ok := h.resolveKind(w, r)
	if !ok {
		return
	}
	kind := strings.ToLower(gvk.Kind)

	list, err := newList(c.Scheme(), gvk)
	if err == nil {
		err = c.List(r.Context(), list)
	}
	if err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			http.Error(w, fmt.Sprintf("%s not found", kind), http.StatusNotFound)
			return
		}
		h.log.Error("failed to list resource", "resource", kind, "error", err)
		http.Error(w, fmt.Sprintf("failed to list %s", kind), http.StatusInternalServerError)
		return
	}

	// Typed reads clear TypeMeta; real Kubernetes list responses always carry
	// kind/apiVersion, so stamp it back on before serializing.
	list.GetObjectKind().SetGroupVersionKind(gvk.GroupVersion().WithKind(gvk.Kind + "List"))
	h.writeJSON(w, list, kind)
}

// handleGet serves a single object looked up by the {name} path segment.
func (h *handler) handleGet(w http.ResponseWriter, r *http.Request) {
	c, gvk, ok := h.resolveKind(w, r)
	if !ok {
		return
	}
	kind := strings.ToLower(gvk.Kind)

	obj, err := newObject(c.Scheme(), gvk)
	if err != nil {
		h.log.Error("failed to instantiate resource", "resource", kind, "error", err)
		http.Error(w, fmt.Sprintf("failed to get %s", kind), http.StatusInternalServerError)
		return
	}

	if err := c.Get(r.Context(), client.ObjectKey{Name: r.PathValue("name")}, obj); err != nil {
		h.writeGetError(w, err, kind)
		return
	}

	// Typed reads clear TypeMeta; stamp it back on for the same reason as handleList.
	obj.GetObjectKind().SetGroupVersionKind(gvk)
	h.writeJSON(w, obj, kind)
}

func (h *handler) writeGetError(w http.ResponseWriter, err error, resource string) {
	if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
		http.Error(w, fmt.Sprintf("%s not found", resource), http.StatusNotFound)
		return
	}

	h.log.Error("failed to get resource", "resource", resource, "error", err)
	http.Error(w, fmt.Sprintf("failed to get %s", resource), http.StatusInternalServerError)
}
