// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package rdmascraper

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	defaultMetricNamespace = "unifabric"
	nodeInfoMetricName     = "unifabric_node_info"
	nodeInfoLabelName      = "unifabric_node_name"
)

var rdmaSampleLabelNames = []string{
	"node_name",
	"device",
	"ifname",
	"parent_ifname",
	"port",
	"priority",
	"pod_name",
	"pod_namespace",
	"topowner_api_version",
	"topowner_kind",
	"topowner_namespace",
	"topowner_name",
	"host_rdma",
	"is_root",
	"kind",
	"scope",
	"source",
}

type PrometheusEmitter struct {
	namespace string

	mu    sync.Mutex
	descs map[string]*prometheus.Desc
}

func NewPrometheusEmitter() *PrometheusEmitter {
	return &PrometheusEmitter{
		namespace: defaultMetricNamespace,
		descs:     make(map[string]*prometheus.Desc),
	}
}

func (e *PrometheusEmitter) Emit(snapshot ScrapeSnapshot, ch chan<- prometheus.Metric) {
	e.emitNodeInfo(snapshot.NodeName, ch)
	for _, sample := range snapshot.Samples {
		e.emitSample(snapshot.NodeName, sample, ch)
	}
}

func (e *PrometheusEmitter) emitNodeInfo(nodeName string, ch chan<- prometheus.Metric) {
	desc := e.desc(nodeInfoMetricName, "Unifabric node information", []string{nodeInfoLabelName})
	ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, 1, nodeName)
}

func (e *PrometheusEmitter) emitSample(nodeName string, sample MetricSample, ch chan<- prometheus.Metric) {
	metricName := e.metricName(sample.Name)
	desc := e.desc(metricName, "", rdmaSampleLabelNames)
	ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, sample.Value, sampleLabelValues(nodeName, sample)...)
}

func (e *PrometheusEmitter) metricName(name string) string {
	if strings.HasPrefix(name, e.namespace+"_") {
		return name
	}
	return e.namespace + "_" + name
}

func (e *PrometheusEmitter) desc(name, help string, labelNames []string) *prometheus.Desc {
	key := name + "\xff" + strings.Join(labelNames, "\xff")

	e.mu.Lock()
	defer e.mu.Unlock()

	desc, ok := e.descs[key]
	if ok {
		return desc
	}
	desc = prometheus.NewDesc(name, help, labelNames, nil)
	e.descs[key] = desc
	return desc
}

func sampleLabelValues(nodeName string, sample MetricSample) []string {
	return []string{
		nodeName,
		sample.Device,
		sample.Ifname,
		sample.ParentIfname,
		sample.Port,
		sample.Priority,
		sample.Workload.PodName,
		sample.Workload.PodNamespace,
		sample.Workload.TopOwner.APIVersion,
		sample.Workload.TopOwner.Kind,
		sample.Workload.TopOwner.Namespace,
		sample.Workload.TopOwner.Name,
		strconv.FormatBool(sample.Workload.HostRDMA),
		strconv.FormatBool(sample.IsRoot),
		sample.Kind,
		string(sample.Scope),
		string(sample.Source),
	}
}

func (e *PrometheusEmitter) String() string {
	return fmt.Sprintf("PrometheusEmitter{namespace:%q}", e.namespace)
}
