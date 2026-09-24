// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package rdmascraper

import (
	"errors"
	"sync"
	"testing"
	"time"
)

var (
	testHostNS = netnsRef{id: hostNetNSID, path: "/host/proc/1/ns/net"}
	testPodNS  = netnsRef{id: "containerd://pod-a", path: "/host/proc/4242/ns/net"}
	testPod1NS = netnsRef{id: "containerd://pod-1", path: "/host/proc/5151/ns/net"}
)

type fakeEthtoolFetcher struct {
	mu      sync.Mutex
	calls   [][]string
	paths   []string
	value   uint64
	ifErr   map[string]error
	err     error
	release chan struct{}
}

func (f *fakeEthtoolFetcher) fetch(netnsPath string, ifnames []string) (map[string]ethtoolStatsResult, error) {
	if f.release != nil {
		<-f.release
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string(nil), ifnames...))
	f.paths = append(f.paths, netnsPath)
	if f.err != nil {
		return nil, f.err
	}
	results := make(map[string]ethtoolStatsResult, len(ifnames))
	for _, ifname := range ifnames {
		if err, ok := f.ifErr[ifname]; ok {
			results[ifname] = ethtoolStatsResult{err: err}
			continue
		}
		results[ifname] = ethtoolStatsResult{stats: map[string]uint64{"rx_prio3_pause": f.value}}
	}
	return results, nil
}

func (f *fakeEthtoolFetcher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func newTestEthtoolStatsCache(fetcher *fakeEthtoolFetcher) (*ethtoolStatsCache, *time.Time) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cache := newEthtoolStatsCache(5*time.Second, fetcher.fetch)
	cache.now = func() time.Time { return now }
	return cache, &now
}

func TestEthtoolStatsCacheReusesEntriesWithinTTL(t *testing.T) {
	fetcher := &fakeEthtoolFetcher{value: 1}
	cache, now := newTestEthtoolStatsCache(fetcher)

	results, err := cache.Stats(testHostNS, []string{"eth0", "eth1"})
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if results["eth0"].stats["rx_prio3_pause"] != 1 || results["eth1"].stats["rx_prio3_pause"] != 1 {
		t.Fatalf("results = %#v, want value 1 for eth0 and eth1", results)
	}
	if fetcher.callCount() != 1 {
		t.Fatalf("fetch calls = %d, want 1", fetcher.callCount())
	}

	fetcher.value = 2
	*now = now.Add(4 * time.Second)
	results, err = cache.Stats(testHostNS, []string{"eth0", "eth1"})
	if err != nil {
		t.Fatalf("stats within ttl: %v", err)
	}
	if fetcher.callCount() != 1 {
		t.Fatalf("fetch calls within ttl = %d, want 1", fetcher.callCount())
	}
	if results["eth0"].stats["rx_prio3_pause"] != 1 {
		t.Fatalf("eth0 within ttl = %#v, want cached value 1", results["eth0"])
	}

	*now = now.Add(time.Second)
	results, err = cache.Stats(testHostNS, []string{"eth0", "eth1"})
	if err != nil {
		t.Fatalf("stats after ttl: %v", err)
	}
	if fetcher.callCount() != 2 {
		t.Fatalf("fetch calls after ttl = %d, want 2", fetcher.callCount())
	}
	if results["eth0"].stats["rx_prio3_pause"] != 2 {
		t.Fatalf("eth0 after ttl = %#v, want refreshed value 2", results["eth0"])
	}
}

func TestEthtoolStatsCacheFetchesOnlyMissingInterfaces(t *testing.T) {
	fetcher := &fakeEthtoolFetcher{value: 1}
	cache, _ := newTestEthtoolStatsCache(fetcher)

	if _, err := cache.Stats(testHostNS, []string{"eth0"}); err != nil {
		t.Fatalf("stats eth0: %v", err)
	}
	results, err := cache.Stats(testHostNS, []string{"eth0", "eth1"})
	if err != nil {
		t.Fatalf("stats eth0 eth1: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %#v, want eth0 and eth1", results)
	}
	if len(fetcher.calls) != 2 || len(fetcher.calls[1]) != 1 || fetcher.calls[1][0] != "eth1" {
		t.Fatalf("fetch calls = %#v, want second call for eth1 only", fetcher.calls)
	}

	if _, err := cache.Stats(testPodNS, []string{"eth0"}); err != nil {
		t.Fatalf("stats pod eth0: %v", err)
	}
	if fetcher.callCount() != 3 {
		t.Fatalf("fetch calls = %d, want a separate fetch per net namespace", fetcher.callCount())
	}
}

func TestEthtoolStatsCacheKeysByIDNotPath(t *testing.T) {
	fetcher := &fakeEthtoolFetcher{value: 1}
	cache, _ := newTestEthtoolStatsCache(fetcher)

	if _, err := cache.Stats(testPodNS, []string{"net1"}); err != nil {
		t.Fatalf("stats pod: %v", err)
	}

	// The pod died and a new container got the same PID and ifname.
	fetcher.value = 2
	reusedPID := netnsRef{id: "containerd://pod-b", path: testPodNS.path}
	results, err := cache.Stats(reusedPID, []string{"net1"})
	if err != nil {
		t.Fatalf("stats reused pid: %v", err)
	}
	if fetcher.callCount() != 2 || results["net1"].stats["rx_prio3_pause"] != 2 {
		t.Fatalf("fetch calls = %d, results = %#v, want the new container fetched instead of the old pod's counters", fetcher.callCount(), results)
	}
	if got := fetcher.paths[1]; got != testPodNS.path {
		t.Fatalf("fetch path = %q, want the namespace entered through %q", got, testPodNS.path)
	}

	// The same container reached through another path shares its entries.
	fetcher.value = 3
	sameContainerOtherPath := netnsRef{id: reusedPID.id, path: "/host/proc/9999/ns/net"}
	results, err = cache.Stats(sameContainerOtherPath, []string{"net1"})
	if err != nil {
		t.Fatalf("stats same id other path: %v", err)
	}
	if fetcher.callCount() != 2 || results["net1"].stats["rx_prio3_pause"] != 2 {
		t.Fatalf("fetch calls = %d, results = %#v, want cached counters shared across paths of one id", fetcher.callCount(), results)
	}
}

func TestEthtoolStatsCacheKeepsInterfaceErrorsAndDropsFetchErrors(t *testing.T) {
	ifErr := errors.New("no stats")
	fetcher := &fakeEthtoolFetcher{value: 1, ifErr: map[string]error{"eth1": ifErr}}
	cache, _ := newTestEthtoolStatsCache(fetcher)

	results, err := cache.Stats(testHostNS, []string{"eth0", "eth1"})
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if !errors.Is(results["eth1"].err, ifErr) {
		t.Fatalf("eth1 = %#v, want cached interface error", results["eth1"])
	}
	results, _ = cache.Stats(testHostNS, []string{"eth0", "eth1"})
	if fetcher.callCount() != 1 || !errors.Is(results["eth1"].err, ifErr) {
		t.Fatalf("fetch calls = %d, eth1 = %#v, want interface error served from cache", fetcher.callCount(), results["eth1"])
	}

	fetchErr := errors.New("enter netns")
	fetcher.err = fetchErr
	results, err = cache.Stats(testHostNS, []string{"eth0", "eth2"})
	if !errors.Is(err, fetchErr) {
		t.Fatalf("stats error = %v, want %v", err, fetchErr)
	}
	if _, ok := results["eth0"]; !ok {
		t.Fatalf("results = %#v, want cached eth0 alongside fetch error", results)
	}
	if _, ok := results["eth2"]; ok {
		t.Fatalf("results = %#v, want no eth2 entry after fetch error", results)
	}

	fetcher.err = nil
	if _, err := cache.Stats(testHostNS, []string{"eth2"}); err != nil {
		t.Fatalf("stats eth2: %v", err)
	}
	if fetcher.callCount() != 3 {
		t.Fatalf("fetch calls = %d, want failed fetch not cached", fetcher.callCount())
	}
}

func TestEthtoolStatsCacheServesLastKnownValuesDuringFetch(t *testing.T) {
	fetcher := &fakeEthtoolFetcher{value: 1}
	cache, now := newTestEthtoolStatsCache(fetcher)

	if _, err := cache.Stats(testHostNS, []string{"eth0"}); err != nil {
		t.Fatalf("stats: %v", err)
	}
	*now = now.Add(10 * time.Second)

	cache.fetchMu.Lock()
	results, err := cache.Stats(testHostNS, []string{"eth0"})
	if err != nil {
		t.Fatalf("stats during fetch: %v", err)
	}
	if fetcher.callCount() != 1 || results["eth0"].stats["rx_prio3_pause"] != 1 {
		t.Fatalf("fetch calls = %d, results = %#v, want stale eth0 without a new fetch", fetcher.callCount(), results)
	}

	done := make(chan map[string]ethtoolStatsResult, 1)
	go func() {
		results, _ := cache.Stats(testHostNS, []string{"eth0", "eth9"})
		done <- results
	}()
	cache.fetchMu.Unlock()

	results = <-done
	if len(results) != 2 || fetcher.callCount() != 2 {
		t.Fatalf("fetch calls = %d, results = %#v, want unknown eth9 fetched after in-flight fetch", fetcher.callCount(), results)
	}
	if got := fetcher.calls[1]; len(got) != 2 {
		t.Fatalf("second fetch = %#v, want stale eth0 and unknown eth9", got)
	}
}

func TestEthtoolStatsCacheDedupesConcurrentScrapes(t *testing.T) {
	fetcher := &fakeEthtoolFetcher{value: 1, release: make(chan struct{})}
	cache, _ := newTestEthtoolStatsCache(fetcher)

	const scrapers = 8
	started := make(chan struct{}, scrapers)
	var wg sync.WaitGroup
	for i := 0; i < scrapers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			started <- struct{}{}
			results, err := cache.Stats(testHostNS, []string{"eth0", "eth1"})
			if err != nil || len(results) != 2 {
				t.Errorf("stats = %#v, %v, want eth0 and eth1", results, err)
			}
		}()
	}
	for i := 0; i < scrapers; i++ {
		<-started
	}
	close(fetcher.release)
	wg.Wait()

	if fetcher.callCount() != 1 {
		t.Fatalf("fetch calls = %d, want 1 shared fetch", fetcher.callCount())
	}
}

func TestEthtoolStatsCacheEvictsUnrefreshedEntries(t *testing.T) {
	fetcher := &fakeEthtoolFetcher{value: 1}
	cache, now := newTestEthtoolStatsCache(fetcher)

	if _, err := cache.Stats(testPod1NS, []string{"net1"}); err != nil {
		t.Fatalf("stats pod-1: %v", err)
	}
	*now = now.Add(cache.retention - time.Second)
	if _, err := cache.Stats(testHostNS, []string{"eth0"}); err != nil {
		t.Fatalf("stats host: %v", err)
	}
	if len(cache.entries) != 2 {
		t.Fatalf("entries = %d, want pod-1 kept within retention", len(cache.entries))
	}

	*now = now.Add(time.Second)
	if _, err := cache.Stats(testHostNS, []string{"eth1"}); err != nil {
		t.Fatalf("stats host eth1: %v", err)
	}
	if _, ok := cache.entries[ethtoolStatsKey{netnsID: testPod1NS.id, ifname: "net1"}]; ok {
		t.Fatalf("entries = %#v, want pod-1 evicted after retention", cache.entries)
	}
	if len(cache.entries) != 2 {
		t.Fatalf("entries = %d, want host eth0 and eth1 kept", len(cache.entries))
	}
}
