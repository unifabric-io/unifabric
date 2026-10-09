// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

var (
	testSrc = peer{
		Namespace: "tenant-a",
		Name:      "vllm-decode-0",
		Owner:     ownerInfo{Kind: "StatefulSet", Namespace: "tenant-a", Name: "vllm-decode"},
	}
	testDst = peer{
		Namespace: "tenant-b",
		Name:      "receiver-7f9c",
		Owner:     ownerInfo{Kind: "Deployment", Namespace: "tenant-b", Name: "receiver"},
	}
)

var testEnds = flowEnds{srcDevice: "mlx5_2", dstDevice: "mlx5_3"}

func TestFlowLabelsOrder(t *testing.T) {
	got := flowLabels(testSrc, testDst, testEnds)
	want := []string{
		"tenant-a", "vllm-decode-0", "StatefulSet", "tenant-a", "vllm-decode",
		"tenant-b", "receiver-7f9c", "Deployment", "tenant-b", "receiver",
		"mlx5_2", "mlx5_3",
	}
	if len(got) != len(flowLabelNames) {
		t.Fatalf("label count = %d, want %d", len(got), len(flowLabelNames))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("label %s = %q, want %q", flowLabelNames[i], got[i], want[i])
		}
	}
}

func TestPeerString(t *testing.T) {
	cases := map[string]peer{
		"tenant-a/vllm-decode-0": testSrc,
		"pid:4242(ib_write_bw)":  {label: "pid:4242(ib_write_bw)"},
		"10.16.1.2":              {label: "10.16.1.2"},
		"unknown":                {},
	}
	for want, p := range cases {
		got := p.String()
		if got != want {
			t.Errorf("peer.String() = %q, want %q", got, want)
		}
	}
}

func expectValue(t *testing.T, name string, collector prometheus.Collector, want float64) {
	t.Helper()
	got := testutil.ToFloat64(collector)
	if got != want {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func TestFlowMetricsAddsDeltasAndSurvivesQPRemoval(t *testing.T) {
	m := newFlowMetrics(prometheus.NewRegistry())
	key := qpKey{Tgid: 100, QPN: 42}
	labels := flowLabels(testSrc, testDst, testEnds)

	m.beginReport()
	m.observe(key, qpStat{Bytes: 1000, WRs: 10}, testSrc, testDst, testEnds)
	m.endReport(map[qpKey]bool{key: true}, reportSummary{infos: 1, stats: 1, matched: 1, flows: 1})
	expectValue(t, "bytes after first report", m.bytes.WithLabelValues(labels...), 1000)

	m.beginReport()
	m.observe(key, qpStat{Bytes: 1500, WRs: 12}, testSrc, testDst, testEnds)
	m.endReport(map[qpKey]bool{key: true}, reportSummary{infos: 1, stats: 1, matched: 1, flows: 1})
	expectValue(t, "bytes after second report", m.bytes.WithLabelValues(labels...), 1500)
	expectValue(t, "wrs after second report", m.wrs.WithLabelValues(labels...), 12)
	expectValue(t, "qps after second report", m.qps.WithLabelValues(labels...), 1)

	m.beginReport()
	m.endReport(map[qpKey]bool{}, reportSummary{})
	expectValue(t, "bytes after QP removal", m.bytes.WithLabelValues(labels...), 1500)
	expectValue(t, "qps after QP removal", m.qps.WithLabelValues(labels...), 0)
	_, stillTracked := m.last[key]
	if stillTracked {
		t.Fatal("removed QP still has a delta baseline")
	}
}

func TestFlowMetricsTreatsCounterRegressionAsNewEntry(t *testing.T) {
	m := newFlowMetrics(prometheus.NewRegistry())
	key := qpKey{Tgid: 100, QPN: 42}
	labels := flowLabels(testSrc, testDst, testEnds)

	m.beginReport()
	m.observe(key, qpStat{Bytes: 1000, WRs: 10}, testSrc, testDst, testEnds)
	m.endReport(map[qpKey]bool{key: true}, reportSummary{infos: 1, stats: 1, matched: 1, flows: 1})

	m.beginReport()
	m.observe(key, qpStat{Bytes: 300, WRs: 3}, testSrc, testDst, testEnds)
	m.endReport(map[qpKey]bool{key: true}, reportSummary{infos: 1, stats: 1, matched: 1, flows: 1})
	expectValue(t, "bytes after regression", m.bytes.WithLabelValues(labels...), 1300)
}

func TestFlowMetricsSkipsUnresolvedPeerUntilResolved(t *testing.T) {
	m := newFlowMetrics(prometheus.NewRegistry())
	key := qpKey{Tgid: 100, QPN: 42}
	unresolved := peer{label: "10.16.1.2"}
	labels := flowLabels(testSrc, testDst, testEnds)

	m.beginReport()
	recorded := m.observe(key, qpStat{Bytes: 1000, WRs: 10}, testSrc, unresolved, testEnds)
	if recorded {
		t.Fatal("observe recorded a flow whose destination is not a Pod")
	}
	m.endReport(map[qpKey]bool{key: true}, reportSummary{stats: 1, matched: 1, unresolved: 1, flows: 1})
	bytesSeries := testutil.CollectAndCount(m.bytes)
	if bytesSeries != 0 {
		t.Fatalf("bytes series after unresolved report = %d, want 0", bytesSeries)
	}
	qpsSeries := testutil.CollectAndCount(m.qps)
	if qpsSeries != 0 {
		t.Fatalf("qps series after unresolved report = %d, want 0", qpsSeries)
	}
	expectValue(t, "unresolved gauge", m.qpUnresolved, 1)
	_, tracked := m.last[key]
	if tracked {
		t.Fatal("unresolved QP must not establish a delta baseline")
	}

	m.beginReport()
	recorded = m.observe(key, qpStat{Bytes: 1500, WRs: 12}, testSrc, testDst, testEnds)
	if !recorded {
		t.Fatal("observe skipped a flow whose both ends are Pods")
	}
	m.endReport(map[qpKey]bool{key: true}, reportSummary{stats: 1, matched: 1, flows: 1})
	expectValue(t, "bytes after resolution", m.bytes.WithLabelValues(labels...), 1500)
	expectValue(t, "wrs after resolution", m.wrs.WithLabelValues(labels...), 12)
	expectValue(t, "unresolved gauge after resolution", m.qpUnresolved, 0)
}

func TestFlowMetricsSkipsNonPodSource(t *testing.T) {
	m := newFlowMetrics(prometheus.NewRegistry())
	key := qpKey{Tgid: 100, QPN: 42}
	host := peer{label: "pid:4242(ib_write_bw)"}

	m.beginReport()
	if m.observe(key, qpStat{Bytes: 1000, WRs: 10}, host, testDst, flowEnds{}) {
		t.Fatal("observe recorded a flow whose source is not a Pod")
	}
	bytesSeries := testutil.CollectAndCount(m.bytes)
	if bytesSeries != 0 {
		t.Fatalf("bytes series for host source = %d, want 0", bytesSeries)
	}
}

// TestFlowMetricsRegistersEveryFamily guards the constructor against a
// field that is declared and registered but never built, which panics
// inside the registry goroutine only at startup.
func TestFlowMetricsRegistersEveryFamily(t *testing.T) {
	m := newFlowMetrics(prometheus.NewRegistry())
	m.sends.WithLabelValues(flowLabels(testSrc, testDst, testEnds)...).Inc()
	m.sendSize.WithLabelValues(flowLabels(testSrc, testDst, testEnds)...).Observe(1)
	for _, sink := range []string{"otlp"} {
		m.sinkDropped.WithLabelValues(sink).Set(0)
		m.sinkQueue.WithLabelValues(sink).Set(0)
		m.sinkInserted.WithLabelValues(sink).Add(0)
		m.sinkInsertErrors.WithLabelValues(sink).Add(0)
		m.sinkInsertSeconds.WithLabelValues(sink).Observe(0)
		m.sinkConnected.WithLabelValues(sink).Set(0)
	}
	families, err := m.registry.(*prometheus.Registry).Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	got := map[string]bool{}
	for _, f := range families {
		got[f.GetName()] = true
	}
	want := []string{
		"rdma_flow_sends_total",
		"rdma_flow_send_bytes",
		"rdma_flow_agent_send_events_received",
		"rdma_flow_agent_send_events_sampled",
		"rdma_flow_agent_send_events_dropped",
		"rdma_flow_agent_send_queue_length",
		"rdma_flow_agent_send_rows_inserted_total",
		"rdma_flow_agent_send_rows_failed_total",
		"rdma_flow_agent_send_insert_seconds",
		"rdma_flow_agent_send_sink_connected",
		"rdma_flow_agent_qp_infos",
		"rdma_flow_agent_qp_stats",
		"rdma_flow_agent_qp_matched",
		"rdma_flow_agent_qp_unmatched",
		"rdma_flow_agent_qp_unresolved",
		"rdma_flow_agent_flows",
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("metric family %s not registered", name)
		}
	}
}
