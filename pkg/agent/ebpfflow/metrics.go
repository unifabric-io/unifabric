// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"github.com/prometheus/client_golang/prometheus"
)

// flowLabelNames is the label order of the flow metrics, matching the order
// returned by flowLabels.
var flowLabelNames = []string{
	"src_pod_namespace", "src_pod_name",
	"src_pod_top_owner_kind", "src_pod_top_owner_namespace", "src_pod_top_owner_name",
	"dst_pod_namespace", "dst_pod_name",
	"dst_pod_top_owner_kind", "dst_pod_top_owner_namespace", "dst_pod_top_owner_name",
	"src_device", "dst_device",
}

// flowEnds is everything that identifies one flow besides the two Pods:
// the RDMA device the sender posted on and, when known, the device the
// receiver's QP lives on. Unknown devices are empty strings.
type flowEnds struct {
	srcDevice string
	dstDevice string
}

// peerLabels returns the five labels of one flow end.
func peerLabels(p peer) []string {
	return []string{
		p.Namespace, p.Name,
		p.Owner.Kind, p.Owner.Namespace, p.Owner.Name,
	}
}

func flowLabels(src, dst peer, ends flowEnds) []string {
	labels := append(peerLabels(src), peerLabels(dst)...)
	return append(labels, ends.srcDevice, ends.dstDevice)
}

// reportSummary holds the counts of one report round, written to the
// rdma_flow_agent_* gauges.
type reportSummary struct {
	infos      int
	stats      int
	matched    int
	unmatched  int
	unresolved int
	flows      int
}

type flowMetrics struct {
	registry prometheus.Registerer
	bytes    *prometheus.CounterVec
	wrs      *prometheus.CounterVec
	qps      *prometheus.GaugeVec

	// sends counts individual submissions and sendSize buckets their
	// payload, both fed from the send_events ring buffer.
	sends    *prometheus.CounterVec
	sendSize *prometheus.HistogramVec

	sendReceived prometheus.Gauge
	sendSampled  prometheus.Gauge

	// per sink counters, labelled by sink name.
	sinkDropped       *prometheus.GaugeVec
	sinkQueue         *prometheus.GaugeVec
	sinkInserted      *prometheus.CounterVec
	sinkInsertErrors  *prometheus.CounterVec
	sinkInsertSeconds *prometheus.HistogramVec
	sinkConnected     *prometheus.GaugeVec

	qpInfos      prometheus.Gauge
	qpStats      prometheus.Gauge
	qpMatched    prometheus.Gauge
	qpUnmatched  prometheus.Gauge
	qpUnresolved prometheus.Gauge
	flows        prometheus.Gauge

	// last remembers the totals last observed per QP to compute deltas.
	last map[qpKey]qpStat
}

// newFlowMetrics builds the metric families and registers them on
// registerer, the manager registry in the agent and a fresh registry in
// tests. Go runtime and process collectors are already provided by the
// manager.
func newFlowMetrics(registerer prometheus.Registerer) *flowMetrics {
	m := &flowMetrics{
		registry: registerer,
		bytes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "rdma_flow_bytes_total",
			Help: "RDMA payload bytes posted from the source to the destination.",
		}, flowLabelNames),
		wrs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "rdma_flow_wrs_total",
			Help: "RDMA work requests posted from the source to the destination.",
		}, flowLabelNames),
		qps: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "rdma_flow_qps",
			Help: "Active queue pairs currently attributed to the flow.",
		}, flowLabelNames),
		sends: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "rdma_flow_sends_total",
			Help: "RDMA send submissions observed on the data plane, one per post_send call or extended verbs setter.",
		}, flowLabelNames),
		sendSize: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "rdma_flow_send_bytes",
			Help:    "Payload bytes per RDMA send submission.",
			Buckets: sendBuckets(),
		}, flowLabelNames),
		sendReceived: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "rdma_flow_agent_send_events_received",
			Help: "Send events read from the send_events ring buffer since start.",
		}),
		sendSampled: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "rdma_flow_agent_send_events_sampled",
			Help: "Send events selected for storage after sampling since start.",
		}),
		sinkDropped: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "rdma_flow_agent_send_events_dropped",
			Help: "Rows dropped because the sink queue was full or the sink was disconnected.",
		}, []string{"sink"}),
		sinkQueue: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "rdma_flow_agent_send_queue_length",
			Help: "Rows waiting to be written to the sink.",
		}, []string{"sink"}),
		sinkInserted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "rdma_flow_agent_send_rows_inserted_total",
			Help: "Rows written to the sink.",
		}, []string{"sink"}),
		sinkInsertErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "rdma_flow_agent_send_rows_failed_total",
			Help: "Rows whose sink insert failed.",
		}, []string{"sink"}),
		sinkInsertSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "rdma_flow_agent_send_insert_seconds",
			Help:    "Duration of sink batch inserts.",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 14),
		}, []string{"sink"}),
		sinkConnected: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "rdma_flow_agent_send_sink_connected",
			Help: "1 while the agent holds a working connection to the sink.",
		}, []string{"sink"}),
		qpInfos: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "rdma_flow_agent_qp_infos",
			Help: "Entries in the qp_infos BPF map.",
		}),
		qpStats: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "rdma_flow_agent_qp_stats",
			Help: "Entries in the qp_stats BPF map.",
		}),
		qpMatched: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "rdma_flow_agent_qp_matched",
			Help: "qp_stats entries joined with a qp_infos destination.",
		}),
		qpUnmatched: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "rdma_flow_agent_qp_unmatched",
			Help: "qp_stats entries without a qp_infos destination.",
		}),
		qpUnresolved: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "rdma_flow_agent_qp_unresolved",
			Help: "qp_stats entries skipped because the source or destination is not a known Pod.",
		}),
		flows: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "rdma_flow_agent_flows",
			Help: "Distinct source and destination pairs in the last report.",
		}),
		last: make(map[qpKey]qpStat),
	}
	m.registry.MustRegister(
		m.bytes, m.wrs, m.qps,
		m.sends, m.sendSize,
		m.sendReceived, m.sendSampled, m.sinkDropped, m.sinkQueue,
		m.sinkInserted, m.sinkInsertErrors, m.sinkInsertSeconds, m.sinkConnected,
		m.qpInfos, m.qpStats, m.qpMatched, m.qpUnmatched, m.qpUnresolved, m.flows,
	)
	return m
}

// observeSend records one submission. Unlike observe it accepts unresolved
// ends, which appear with empty labels, because the size distribution is
// only useful when it covers every submission. The consumer runs on its
// own goroutine, and Prometheus vectors are safe for concurrent use.
func (m *flowMetrics) observeSend(src, dst peer, ends flowEnds, bytes uint64) {
	labels := flowLabels(src, dst, ends)
	m.sends.WithLabelValues(labels...).Inc()
	m.sendSize.WithLabelValues(labels...).Observe(float64(bytes))
}

// beginReport clears the active QP gauge at the start of a round, observe
// then accumulates it per flow.
func (m *flowMetrics) beginReport() {
	m.qps.Reset()
}

// observe adds the delta of one QP since the last round to the counters of
// its flow and reports whether it was recorded. Only flows with both ends
// resolved to Pods are recorded, otherwise the QP is skipped without moving
// its baseline, so traffic seen before the Pod index caught up is credited
// once after resolution and no series with empty labels appears. A counter
// going backwards means the map entry was recreated and is treated as new.
func (m *flowMetrics) observe(key qpKey, st qpStat, src, dst peer, ends flowEnds) bool {
	if !src.isPod() || !dst.isPod() {
		return false
	}
	labels := flowLabels(src, dst, ends)
	prev := m.last[key]
	deltaBytes := st.Bytes
	deltaWRs := st.WRs
	if st.Bytes >= prev.Bytes && st.WRs >= prev.WRs {
		deltaBytes = st.Bytes - prev.Bytes
		deltaWRs = st.WRs - prev.WRs
	}
	m.last[key] = st
	if deltaBytes > 0 {
		m.bytes.WithLabelValues(labels...).Add(float64(deltaBytes))
	}
	if deltaWRs > 0 {
		m.wrs.WithLabelValues(labels...).Add(float64(deltaWRs))
	}
	m.qps.WithLabelValues(labels...).Inc()
	return true
}

// endReport writes the summary gauges and drops the baselines of QPs that
// did not appear in this round.
func (m *flowMetrics) endReport(seen map[qpKey]bool, s reportSummary) {
	for key := range m.last {
		if !seen[key] {
			delete(m.last, key)
		}
	}
	m.qpInfos.Set(float64(s.infos))
	m.qpStats.Set(float64(s.stats))
	m.qpMatched.Set(float64(s.matched))
	m.qpUnmatched.Set(float64(s.unmatched))
	m.qpUnresolved.Set(float64(s.unresolved))
	m.flows.Set(float64(s.flows))
}
