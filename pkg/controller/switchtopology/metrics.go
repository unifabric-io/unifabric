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
