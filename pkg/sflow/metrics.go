// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"strconv"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
)

type CollectorMetrics struct {
	DatagramsAccepted atomic.Uint64
	DecodeErrors      atomic.Uint64
	RecordsDecoded    atomic.Uint64
	RecordsDropped    atomic.Uint64
	RecordsWritten    atomic.Uint64
	WriteErrors       atomic.Uint64
	QueueDepth        atomic.Int64
}

func (m *CollectorMetrics) Snapshot() StatsSnapshot {
	if m == nil {
		return StatsSnapshot{}
	}
	return StatsSnapshot{
		DatagramsAccepted: m.DatagramsAccepted.Load(),
		DecodeErrors:      m.DecodeErrors.Load(),
		RecordsDecoded:    m.RecordsDecoded.Load(),
		RecordsDropped:    m.RecordsDropped.Load(),
		RecordsWritten:    m.RecordsWritten.Load(),
		WriteErrors:       m.WriteErrors.Load(),
	}
}

func (m *CollectorMetrics) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range sflowMetricDescs {
		ch <- desc
	}
}

func (m *CollectorMetrics) Collect(ch chan<- prometheus.Metric) {
	snapshot := m.Snapshot()
	ch <- prometheus.MustNewConstMetric(sflowDatagramsAcceptedDesc, prometheus.CounterValue, float64(snapshot.DatagramsAccepted))
	ch <- prometheus.MustNewConstMetric(sflowDecodeErrorsDesc, prometheus.CounterValue, float64(snapshot.DecodeErrors))
	ch <- prometheus.MustNewConstMetric(sflowRecordsDecodedDesc, prometheus.CounterValue, float64(snapshot.RecordsDecoded))
	ch <- prometheus.MustNewConstMetric(sflowRecordsDroppedDesc, prometheus.CounterValue, float64(snapshot.RecordsDropped))
	ch <- prometheus.MustNewConstMetric(sflowRecordsWrittenDesc, prometheus.CounterValue, float64(snapshot.RecordsWritten))
	ch <- prometheus.MustNewConstMetric(sflowWriteErrorsDesc, prometheus.CounterValue, float64(snapshot.WriteErrors))
	ch <- prometheus.MustNewConstMetric(sflowQueueDepthDesc, prometheus.GaugeValue, float64(m.QueueDepth.Load()))
}

func (m *CollectorMetrics) addDropped(n int) {
	if m != nil && n > 0 {
		m.RecordsDropped.Add(uint64(n))
	}
}

func (m *CollectorMetrics) addWritten(n int) {
	if m != nil && n > 0 {
		m.RecordsWritten.Add(uint64(n))
	}
}

func boolLabel(v bool) string {
	return strconv.FormatBool(v)
}

var (
	sflowDatagramsAcceptedDesc = prometheus.NewDesc("unifabric_sflow_datagrams_accepted_total", "Total sFlow datagrams accepted by the collector.", nil, nil)
	sflowDecodeErrorsDesc      = prometheus.NewDesc("unifabric_sflow_decode_errors_total", "Total sFlow datagrams that failed decoding.", nil, nil)
	sflowRecordsDecodedDesc    = prometheus.NewDesc("unifabric_sflow_records_decoded_total", "Total normalized sFlow records decoded.", nil, nil)
	sflowRecordsDroppedDesc    = prometheus.NewDesc("unifabric_sflow_records_dropped_total", "Total normalized sFlow records dropped by overload handling.", nil, nil)
	sflowRecordsWrittenDesc    = prometheus.NewDesc("unifabric_sflow_records_written_total", "Total normalized sFlow records successfully written.", nil, nil)
	sflowWriteErrorsDesc       = prometheus.NewDesc("unifabric_sflow_write_errors_total", "Total ClickHouse write failures.", nil, nil)
	sflowQueueDepthDesc        = prometheus.NewDesc("unifabric_sflow_queue_depth", "Current number of records waiting to be written.", nil, nil)
	sflowMetricDescs           = []*prometheus.Desc{
		sflowDatagramsAcceptedDesc,
		sflowDecodeErrorsDesc,
		sflowRecordsDecodedDesc,
		sflowRecordsDroppedDesc,
		sflowRecordsWrittenDesc,
		sflowWriteErrorsDesc,
		sflowQueueDepthDesc,
	}
)
