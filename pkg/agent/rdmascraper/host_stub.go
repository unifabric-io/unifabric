// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package rdmascraper

import "context"

func (s *RuntimeScraper) collectHost(ctx context.Context, snapshot *ScrapeSnapshot) hostCollection {
	if err := ctx.Err(); err != nil {
		snapshot.AddWarning(scrapeWarning(MetricScopeHost, "", "", "", "", "host scrape skipped because context is done", err))
		return hostCollection{}
	}
	snapshot.AddWarning(scrapeWarning(MetricScopeHost, "", "", "", "", "host RDMA scraping is only supported on linux", nil))
	return hostCollection{}
}

func scrapeWarning(scope MetricScope, device, ifname, port, path, message string, err error) ScrapeWarning {
	warning := ScrapeWarning{
		Scope:   scope,
		Device:  device,
		Ifname:  ifname,
		Port:    port,
		Path:    path,
		Message: message,
	}
	if err != nil {
		warning.Error = err.Error()
	}
	return warning
}
