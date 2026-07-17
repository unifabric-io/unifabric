// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchtopology

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	registerSwitchTopologyMetricsOnce sync.Once

	switchCountMetric = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unifabric_switch_count",
		Help: "The number of switches currently managed by the control plane and included in topology computation.",
	})

	switchLLDPParseSuccessTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "unifabric_switch_lldp_parse_success_total",
		Help: "The number of times the controller successfully parsed and accepted LLDP neighbor data reported by switches.",
	})

	switchLLDPParseFailureTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "unifabric_switch_lldp_parse_failure_total",
		Help: "The number of times the controller failed to parse or validate LLDP neighbor data reported by switches.",
	})

	autoLabelReconcileTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "unifabric_auto_discovered_topology_label_reconcile_total",
		Help: "The number of auto-discovered topology label reconciliation attempts.",
	})

	autoLabelErrorTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "unifabric_auto_discovered_topology_label_error_total",
		Help: "The number of auto-discovered topology label reconciliation errors.",
	}, []string{"topology", "reason"})

	autoLabelLastSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "unifabric_auto_discovered_topology_label_last_success_timestamp_seconds",
		Help: "Unix timestamp of the last successful auto-discovered topology label reconciliation.",
	})
)

func init() {
	registerSwitchTopologyMetrics()
}

func registerSwitchTopologyMetrics() {
	registerSwitchTopologyMetricsOnce.Do(func() {
		ctrlmetrics.Registry.MustRegister(
			switchCountMetric,
			switchLLDPParseSuccessTotal,
			switchLLDPParseFailureTotal,
			autoLabelReconcileTotal,
			autoLabelErrorTotal,
			autoLabelLastSuccess,
		)
	})
}

func setManagedSwitchCount(count int) {
	switchCountMetric.Set(float64(count))
}

func observeLLDPParseSuccess() {
	switchLLDPParseSuccessTotal.Inc()
}

func observeLLDPParseFailure() {
	switchLLDPParseFailureTotal.Inc()
}
