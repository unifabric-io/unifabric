// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package rdmascraper

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type fakeScraper struct{}

func (fakeScraper) Scrape(ctx context.Context) (ScrapeSnapshot, error) {
	return ScrapeSnapshot{
		NodeName: "node-1",
		Devices: DeviceInventory{
			Devices: []RDMADevice{
				{
					Name:         "rxe_eth1",
					Provider:     DeviceProviderRXE,
					Ifname:       "eth1",
					ParentIfname: "eth1",
					Ports:        []RDMAPort{{Name: "1"}},
				},
			},
		},
		Samples: []MetricSample{
			{
				Name:         "port_rcv_data",
				Value:        1024,
				Scope:        MetricScopeHost,
				Source:       MetricSourceHWCounters,
				Device:       "rxe_eth1",
				Ifname:       "eth1",
				ParentIfname: "eth1",
				Port:         "1",
				IsRoot:       true,
				Kind:         "scaleOut",
			},
		},
	}, nil
}

func TestCollectorEmitsPrometheusMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry.MustRegister(NewCollector(fakeScraper{}, logger))

	metricFamilies, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	nodeInfo := findMetricFamily(t, metricFamilies, "unifabric_node_info")
	if got := nodeInfo.GetMetric()[0].GetGauge().GetValue(); got != 1 {
		t.Fatalf("node info value = %v, want 1", got)
	}
	assertLabel(t, nodeInfo.GetMetric()[0], "unifabric_node_name", "node-1")

	rdmaMetric := findMetricFamily(t, metricFamilies, "unifabric_port_rcv_data")
	if got := rdmaMetric.GetMetric()[0].GetGauge().GetValue(); got != 1024 {
		t.Fatalf("port_rcv_data value = %v, want 1024", got)
	}
	assertLabel(t, rdmaMetric.GetMetric()[0], "node_name", "node-1")
	assertLabel(t, rdmaMetric.GetMetric()[0], "device", "rxe_eth1")
	assertLabel(t, rdmaMetric.GetMetric()[0], "ifname", "eth1")
	assertLabel(t, rdmaMetric.GetMetric()[0], "parent_ifname", "eth1")
	assertLabel(t, rdmaMetric.GetMetric()[0], "port", "1")
	assertLabel(t, rdmaMetric.GetMetric()[0], "is_root", "true")
	assertLabel(t, rdmaMetric.GetMetric()[0], "kind", "scaleOut")
	assertLabel(t, rdmaMetric.GetMetric()[0], "scope", "host")
	assertLabel(t, rdmaMetric.GetMetric()[0], "source", "hw_counters")
}

func findMetricFamily(t *testing.T, metricFamilies []*dto.MetricFamily, name string) *dto.MetricFamily {
	t.Helper()
	for _, mf := range metricFamilies {
		if mf.GetName() == name {
			return mf
		}
	}
	t.Fatalf("metric family %q not found", name)
	return nil
}

func assertLabel(t *testing.T, metric *dto.Metric, name, want string) {
	t.Helper()
	for _, label := range metric.GetLabel() {
		if label.GetName() == name {
			if got := label.GetValue(); got != want {
				t.Fatalf("label %s = %q, want %q", name, got, want)
			}
			return
		}
	}
	t.Fatalf("label %s not found", name)
}
