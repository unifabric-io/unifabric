// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package rdmascraper

import (
	"fmt"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/safchain/ethtool"
)

// readEthtoolStats enters netnsPath once and reads the per-priority ethtool
// counters of every ifname, keeping only the rx/tx_prio<N>_pause and
// rx/tx_prio<N>_discards entries the scraper emits.
func readEthtoolStats(netnsPath string, ifnames []string) (map[string]ethtoolStatsResult, error) {
	results := make(map[string]ethtoolStatsResult, len(ifnames))
	err := ns.WithNetNSPath(netnsPath, func(ns.NetNS) error {
		et, err := ethtool.NewEthtool()
		if err != nil {
			return fmt.Errorf("init ethtool: %w", err)
		}
		defer et.Close()

		for _, ifname := range ifnames {
			stats, err := et.Stats(ifname)
			if err != nil {
				results[ifname] = ethtoolStatsResult{err: err}
				continue
			}
			priorityStats := make(map[string]uint64)
			for statName, statValue := range stats {
				if isPriorityMetric(statName) {
					priorityStats[statName] = statValue
				}
			}
			results[ifname] = ethtoolStatsResult{stats: priorityStats}
		}
		return nil
	})
	return results, err
}
