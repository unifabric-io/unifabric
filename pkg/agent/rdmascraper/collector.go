// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package rdmascraper

import (
	"context"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
)

type Scraper interface {
	Scrape(ctx context.Context) (ScrapeSnapshot, error)
}

type Collector struct {
	scraper Scraper
	emitter *PrometheusEmitter
	logger  *slog.Logger
}

func NewCollector(scraper Scraper, logger *slog.Logger) *Collector {
	if logger == nil {
		logger = slog.Default()
	}
	return &Collector{
		scraper: scraper,
		emitter: NewPrometheusEmitter(),
		logger:  logger,
	}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	if c.scraper == nil {
		c.logger.Error("RDMA scraper is nil")
		return
	}
	snapshot, err := c.scraper.Scrape(context.Background())
	if err != nil {
		c.logger.Error("failed to scrape RDMA metrics", "error", err)
		return
	}
	for _, warning := range snapshot.Warnings {
		c.logger.Debug(
			"RDMA scrape warning",
			"scope", warning.Scope,
			"device", warning.Device,
			"ifname", warning.Ifname,
			"port", warning.Port,
			"path", warning.Path,
			"message", warning.Message,
			"error", warning.Error,
		)
	}
	c.emitter.Emit(snapshot, ch)
}
