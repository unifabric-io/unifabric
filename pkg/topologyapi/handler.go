// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

// Package topologyapi implements a small read-only HTTP API that serves this
// project's cluster-scoped CRDs from one or more Kubernetes clusters, using
// /apis/{group}/{version}/{kind} paths under a /clusters/{cluster} prefix.
// Unlike the Kubernetes API it addresses types by singular kind, not plural
// resource, so no discovery round-trip is needed.
//
// The open-source controller serves its own cluster via SingleCluster;
// multi-cluster builds plug in their own ClusterResolver implementation.
package topologyapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// NewHandler returns the HTTP handler serving the topology API for the given
// clusters. The API is read-only and has no authentication of its own.
func NewHandler(clusters ClusterResolver, log *slog.Logger) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	h := &handler{clusters: clusters, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.handleHealthz)
	mux.HandleFunc("GET /clusters", h.handleClusters)
	mux.HandleFunc("GET /clusters/{cluster}/apis/{group}/{version}/{kind}", h.handleList)
	mux.HandleFunc("GET /clusters/{cluster}/apis/{group}/{version}/{kind}/{name}", h.handleGet)
	return mux
}

type handler struct {
	clusters ClusterResolver
	log      *slog.Logger
}

func (h *handler) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

type clusterItem struct {
	Name string `json:"name"`
}

// handleClusters lists the clusters this API can serve resources for.
func (h *handler) handleClusters(w http.ResponseWriter, r *http.Request) {
	names, err := h.clusters.ListClusters(r.Context())
	if err != nil {
		h.log.Error("failed to list clusters", "error", err)
		http.Error(w, "failed to list clusters", http.StatusInternalServerError)
		return
	}

	items := make([]clusterItem, 0, len(names))
	for _, name := range names {
		items = append(items, clusterItem{Name: name})
	}
	h.writeJSON(w, items, "clusters")
}

func (h *handler) writeJSON(w http.ResponseWriter, value any, resource string) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		h.log.Error("failed to encode resource", "resource", resource, "error", err)
	}
}
