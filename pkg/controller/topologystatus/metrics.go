// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package topologystatus

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	registerMetricsOnce sync.Once

	topologyStatusReconcileTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "unifabric_topology_status_reconcile_total",
		Help: "The number of Topology status reconciliation attempts.",
	}, []string{"topology"})
	topologyStatusErrorTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "unifabric_topology_status_error_total",
		Help: "The number of Topology status reconciliation errors.",
	}, []string{"topology", "reason"})
	topologyStatusPending = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "unifabric_topology_status_pending_members",
		Help: "The number of Switch member labels without a matching Node performance domain in the latest build.",
	}, []string{"topology"})
	topologyStatusLastSuccess = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "unifabric_topology_status_last_success_timestamp_seconds",
		Help: "Unix timestamp of the last successful Topology status reconciliation.",
	}, []string{"topology"})
	topologyStatusStale = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "unifabric_topology_status_stale",
		Help: "Whether the published Topology status is stale because the latest reconciliation failed.",
	}, []string{"topology"})
)

func init() {
	registerMetricsOnce.Do(func() {
		ctrlmetrics.Registry.MustRegister(topologyStatusReconcileTotal, topologyStatusErrorTotal, topologyStatusPending, topologyStatusLastSuccess, topologyStatusStale)
	})
}
