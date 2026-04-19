// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package rdmascraper

import (
	"context"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
)

func (s *RuntimeScraper) collectPods(ctx context.Context, snapshot *ScrapeSnapshot, pods []v1beta1.RdmaPod, hostSamples []MetricSample, host hostCollection) {
	if len(pods) == 0 {
		return
	}
	if err := ctx.Err(); err != nil {
		snapshot.AddWarning(scrapeWarning(MetricScopePod, "", "", "", "", "pod scrape stopped because context is done", err))
		return
	}
	snapshot.AddWarning(scrapeWarning(MetricScopePod, "", "", "", "", "pod RDMA scraping is only supported on linux", nil))
}
