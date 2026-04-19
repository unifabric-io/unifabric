// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package rdmametrics

import (
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/unifabric-io/unifabric/pkg/agent/fabricnode"
)

type noopCollector struct{}

func (noopCollector) Describe(ch chan<- *prometheus.Desc) {}

func (noopCollector) Collect(ch chan<- prometheus.Metric) {}

func NewMetrics(_ fabricnode.Interface, _ *slog.Logger) prometheus.Collector {
	return noopCollector{}
}
