// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package rdmascraper

import (
	"sync"
	"time"
)

// ethtoolStatsTTL bounds how long ethtool counters are reused before the
// scraper issues new ETHTOOL_GSTATS ioctls for an interface.
const ethtoolStatsTTL = 5 * time.Second

// hostNetNSID keys the host network namespace, which lives as long as the node.
const hostNetNSID = "host"

// netnsRef locates a network namespace for the cache. path is the
// /proc/<pid>/ns/net entry used to enter it. id keys the entries and must not
// be handed to a different namespace while an entry can still be served, so
// callers use hostNetNSID for the host and the container ID for pods. Neither
// the path nor the namespace inode qualify: PIDs are reused, and the kernel
// gives freed namespace inode numbers to the next namespace it creates.
type netnsRef struct {
	id   string
	path string
}

// ethtoolStatsResult holds the per-priority ethtool counters of one interface,
// or the error that prevented reading them.
type ethtoolStatsResult struct {
	stats map[string]uint64
	err   error
}

// ethtoolStatsFetcher reads the per-priority ethtool counters of ifnames inside
// netnsPath. Per-interface failures are reported through ethtoolStatsResult.err;
// the returned error covers failures that prevented reading any interface.
type ethtoolStatsFetcher func(netnsPath string, ifnames []string) (map[string]ethtoolStatsResult, error)

type ethtoolStatsKey struct {
	netnsID string
	ifname  string
}

type ethtoolStatsEntry struct {
	result    ethtoolStatsResult
	fetchedAt time.Time
}

// ethtoolStatsCache reuses ethtool counters across scrapes that arrive within
// ttl of each other and lets at most one fetch run at a time. Every
// ETHTOOL_GSTATS ioctl takes the kernel rtnl lock, and on SR-IOV hosts the VF
// interfaces multiply the ioctls issued per scrape.
type ethtoolStatsCache struct {
	ttl       time.Duration
	retention time.Duration
	now       func() time.Time
	fetch     ethtoolStatsFetcher

	mu      sync.Mutex
	entries map[ethtoolStatsKey]ethtoolStatsEntry

	// fetchMu serializes fetches; callers that find it held reuse cached values.
	fetchMu sync.Mutex
}

func newEthtoolStatsCache(ttl time.Duration, fetch ethtoolStatsFetcher) *ethtoolStatsCache {
	return &ethtoolStatsCache{
		ttl:       ttl,
		retention: 10 * ttl,
		now:       time.Now,
		fetch:     fetch,
		entries:   make(map[ethtoolStatsKey]ethtoolStatsEntry),
	}
}

// Stats returns the counters of ifnames in netns, fetching only the
// interfaces without a fresh cached value. While another fetch is in flight,
// callers get the last known values instead of waiting, unless an interface
// has never been read. Interfaces missing from the result could not be read
// and are covered by the returned error.
func (c *ethtoolStatsCache) Stats(netns netnsRef, ifnames []string) (map[string]ethtoolStatsResult, error) {
	results, missing := c.lookup(netns.id, ifnames, false)
	if len(missing) == 0 {
		return results, nil
	}

	if !c.fetchMu.TryLock() {
		if stale, unknown := c.lookup(netns.id, ifnames, true); len(unknown) == 0 {
			return stale, nil
		}
		c.fetchMu.Lock()
	}
	defer c.fetchMu.Unlock()

	// The fetch that held fetchMu may have covered these interfaces already.
	results, missing = c.lookup(netns.id, ifnames, false)
	if len(missing) == 0 {
		return results, nil
	}

	fetched, err := c.fetch(netns.path, missing)
	c.store(netns.id, fetched)
	for ifname, result := range fetched {
		results[ifname] = result
	}
	return results, err
}

// lookup returns the cached results of ifnames plus the ifnames it could not
// serve. Entries older than ttl are only served when allowStale is set.
func (c *ethtoolStatsCache) lookup(netnsID string, ifnames []string, allowStale bool) (map[string]ethtoolStatsResult, []string) {
	now := c.now()
	results := make(map[string]ethtoolStatsResult, len(ifnames))
	var missing []string

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ifname := range ifnames {
		entry, ok := c.entries[ethtoolStatsKey{netnsID: netnsID, ifname: ifname}]
		if !ok || (!allowStale && now.Sub(entry.fetchedAt) >= c.ttl) {
			missing = append(missing, ifname)
			continue
		}
		results[ifname] = entry.result
	}
	return results, missing
}

func (c *ethtoolStatsCache) store(netnsID string, fetched map[string]ethtoolStatsResult) {
	now := c.now()

	c.mu.Lock()
	defer c.mu.Unlock()
	for ifname, result := range fetched {
		c.entries[ethtoolStatsKey{netnsID: netnsID, ifname: ifname}] = ethtoolStatsEntry{result: result, fetchedAt: now}
	}
	// Interfaces that stopped being scraped, such as VFs of deleted pods, are never refreshed again.
	for key, entry := range c.entries {
		if now.Sub(entry.fetchedAt) >= c.retention {
			delete(c.entries, key)
		}
	}
}
