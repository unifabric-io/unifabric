// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package rdmascraper

import (
	"context"
	"log/slog"

	"github.com/unifabric-io/unifabric/pkg/agent/fabricnode"
	"github.com/unifabric-io/unifabric/pkg/config"
)

type RuntimeScraper struct {
	fabricNode  fabricnode.Interface
	logger      *slog.Logger
	kindMatcher interfaceKindMatcher
	paths       scraperPaths
}

type hostCollection struct {
	pciToPfIfname map[string]string
	rootIfnames   []string
}

func NewRuntimeScraper(fabricNode fabricnode.Interface, logger *slog.Logger, topologyConfig config.NodeTopologyDiscoveryConfig) *RuntimeScraper {
	if logger == nil {
		logger = slog.Default()
	}
	return &RuntimeScraper{
		fabricNode:  fabricNode,
		logger:      logger,
		kindMatcher: buildInterfaceKindMatcher(topologyConfig),
		paths:       defaultScraperPaths(),
	}
}

func (s *RuntimeScraper) Scrape(ctx context.Context) (ScrapeSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return ScrapeSnapshot{}, err
	}
	if s == nil || s.fabricNode == nil {
		return ScrapeSnapshot{}, nil
	}

	node := s.fabricNode.GetFabricNode()
	if node == nil {
		return ScrapeSnapshot{}, nil
	}

	snapshot := ScrapeSnapshot{
		NodeName: node.Name,
	}
	hostSampleStart := len(snapshot.Samples)
	hostCollection := s.collectHost(ctx, &snapshot)
	hostSamples := append([]MetricSample(nil), snapshot.Samples[hostSampleStart:]...)
	s.collectPods(ctx, &snapshot, node.Status.RdmaPods, hostSamples, hostCollection)
	return snapshot, nil
}
