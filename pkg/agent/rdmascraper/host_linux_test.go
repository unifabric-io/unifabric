// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package rdmascraper

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRuntimeScraperCollectsHostSysfsSnapshot(t *testing.T) {
	root := t.TempDir()
	paths := createFakeHostSysfs(t, root)

	node := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
	}
	scraper := NewRuntimeScraper(fakeFabricNodeClient{node: node}, discardLogger(), config.NodeTopologyDiscoveryConfig{})
	scraper.paths = paths

	snapshot, err := scraper.Scrape(context.Background())
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	if snapshot.NodeName != "node-1" {
		t.Fatalf("node name = %q, want node-1", snapshot.NodeName)
	}
	if len(snapshot.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", snapshot.Warnings)
	}
	if len(snapshot.Devices.Devices) != 1 {
		t.Fatalf("devices = %#v, want one device", snapshot.Devices.Devices)
	}
	device := snapshot.Devices.Devices[0]
	if device.Name != "rxe_eth1" || device.Provider != DeviceProviderRXE || device.Ifname != "eth1" || device.ParentIfname != "eth1" {
		t.Fatalf("device = %#v, want rxe_eth1 eth1 root device", device)
	}
	if len(device.Ports) != 1 || device.Ports[0].Name != "1" {
		t.Fatalf("ports = %#v, want port 1", device.Ports)
	}

	assertSample(t, snapshot, sampleWant{
		name:         "port_rcv_data",
		value:        1024,
		source:       MetricSourceHWCounters,
		device:       "rxe_eth1",
		ifname:       "eth1",
		parentIfname: "eth1",
		port:         "1",
		isRoot:       true,
		kind:         rdmaInterfaceKindScaleOut,
	})
	assertSample(t, snapshot, sampleWant{
		name:         "port_xmit_packets",
		value:        7,
		source:       MetricSourceCounters,
		device:       "rxe_eth1",
		ifname:       "eth1",
		parentIfname: "eth1",
		port:         "1",
		isRoot:       true,
		kind:         rdmaInterfaceKindScaleOut,
	})
	assertSample(t, snapshot, sampleWant{
		name:         "port_speed_mbps",
		value:        100000,
		source:       MetricSourceInterface,
		device:       "rxe_eth1",
		ifname:       "eth1",
		parentIfname: "eth1",
		port:         "1",
		isRoot:       true,
		kind:         rdmaInterfaceKindScaleOut,
	})
	assertSample(t, snapshot, sampleWant{
		name:         "port_mtu",
		value:        9000,
		source:       MetricSourceInterface,
		device:       "rxe_eth1",
		ifname:       "eth1",
		parentIfname: "eth1",
		port:         "1",
		isRoot:       true,
		kind:         rdmaInterfaceKindScaleOut,
	})
	assertSample(t, snapshot, sampleWant{
		name:         "port_oper_state",
		value:        1,
		source:       MetricSourceInterface,
		device:       "rxe_eth1",
		ifname:       "eth1",
		parentIfname: "eth1",
		port:         "1",
		isRoot:       true,
		kind:         rdmaInterfaceKindScaleOut,
	})
	assertSample(t, snapshot, sampleWant{
		name:         "rdma_device_tos",
		value:        5,
		source:       MetricSourceDevice,
		device:       "rxe_eth1",
		ifname:       "eth1",
		parentIfname: "eth1",
		isRoot:       true,
		kind:         rdmaInterfaceKindScaleOut,
	})
}

func TestCollectHostSysfsSkipsUnattachedVFs(t *testing.T) {
	root := t.TempDir()
	paths := createFakeHostSysfs(t, root)
	addFakeHostVF(t, root, paths)

	scraper := NewRuntimeScraper(fakeFabricNodeClient{}, discardLogger(), config.NodeTopologyDiscoveryConfig{})
	scraper.paths = paths

	var snapshot ScrapeSnapshot
	collection, err := scraper.collectHostSysfs(&snapshot)
	if err != nil {
		t.Fatalf("collect host sysfs: %v", err)
	}
	if len(snapshot.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", snapshot.Warnings)
	}
	if len(collection.rootIfnames) != 1 || collection.rootIfnames[0] != "eth1" {
		t.Fatalf("root ifnames = %#v, want only the PF eth1", collection.rootIfnames)
	}
	if len(snapshot.Devices.Devices) != 1 || snapshot.Devices.Devices[0].Name != "rxe_eth1" {
		t.Fatalf("devices = %#v, want only rxe_eth1", snapshot.Devices.Devices)
	}
	for _, sample := range snapshot.Samples {
		if sample.Device == "mlx5_vf" || sample.Ifname == "eth1v0" {
			t.Fatalf("sample = %#v, want no samples for the unattached VF", sample)
		}
	}
	assertSample(t, snapshot, sampleWant{
		name:         "port_rcv_data",
		value:        1024,
		source:       MetricSourceHWCounters,
		device:       "rxe_eth1",
		ifname:       "eth1",
		parentIfname: "eth1",
		port:         "1",
		isRoot:       true,
		kind:         rdmaInterfaceKindScaleOut,
	})
}

type sampleWant struct {
	name         string
	value        float64
	source       MetricSource
	device       string
	ifname       string
	parentIfname string
	port         string
	isRoot       bool
	kind         string
}

func TestRuntimeScraperCollectsHostEthtoolThroughCache(t *testing.T) {
	root := t.TempDir()
	paths := createFakeHostSysfs(t, root)
	paths.hostNetNSPath = filepath.Join(root, "host", "proc", "1", "ns", "net")

	node := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
	}
	scraper := NewRuntimeScraper(fakeFabricNodeClient{node: node}, discardLogger(), config.NodeTopologyDiscoveryConfig{})
	scraper.paths = paths
	fetcher := &fakeEthtoolFetcher{value: 42}
	scraper.ethtoolStats.fetch = fetcher.fetch

	for i := 0; i < 2; i++ {
		snapshot, err := scraper.Scrape(context.Background())
		if err != nil {
			t.Fatalf("scrape %d: %v", i, err)
		}
		if len(snapshot.Warnings) != 0 {
			t.Fatalf("scrape %d warnings = %#v, want none", i, snapshot.Warnings)
		}
		var ethtoolSamples []MetricSample
		for _, sample := range snapshot.Samples {
			if sample.Source == MetricSourceEthtool {
				ethtoolSamples = append(ethtoolSamples, sample)
			}
		}
		if len(ethtoolSamples) != 1 {
			t.Fatalf("scrape %d ethtool samples = %#v, want one rx_pause sample", i, ethtoolSamples)
		}
		sample := ethtoolSamples[0]
		if sample.Name != "rx_pause" || sample.Value != 42 || sample.Priority != "3" || sample.Scope != MetricScopeHost {
			t.Fatalf("scrape %d ethtool sample = %#v, want host rx_pause priority 3 value 42", i, sample)
		}
		if sample.Ifname != "eth1" || sample.ParentIfname != "eth1" || !sample.IsRoot || sample.Kind != rdmaInterfaceKindScaleOut {
			t.Fatalf("scrape %d ethtool sample labels = %#v, want root eth1 scale-out", i, sample)
		}
	}
	if len(fetcher.calls) != 1 || len(fetcher.calls[0]) != 1 || fetcher.calls[0][0] != "eth1" {
		t.Fatalf("fetch calls = %#v, want a single fetch for eth1 across both scrapes", fetcher.calls)
	}
	if fetcher.paths[0] != paths.hostNetNSPath {
		t.Fatalf("fetch path = %q, want %q", fetcher.paths[0], paths.hostNetNSPath)
	}
	if _, ok := scraper.ethtoolStats.entries[ethtoolStatsKey{netnsID: hostNetNSID, ifname: "eth1"}]; !ok {
		t.Fatalf("cache entries = %#v, want host eth1 keyed by %q", scraper.ethtoolStats.entries, hostNetNSID)
	}

	fetcher.ifErr = map[string]error{"eth1": errors.New("no stats")}
	scraper.ethtoolStats.now = func() time.Time { return time.Now().Add(time.Minute) }
	snapshot, err := scraper.Scrape(context.Background())
	if err != nil {
		t.Fatalf("scrape after ttl: %v", err)
	}
	if len(snapshot.Warnings) != 1 || snapshot.Warnings[0].Message != "failed to read ethtool stats" || snapshot.Warnings[0].Ifname != "eth1" {
		t.Fatalf("warnings = %#v, want one ethtool stats warning for eth1", snapshot.Warnings)
	}
	for _, sample := range snapshot.Samples {
		if sample.Source == MetricSourceEthtool {
			t.Fatalf("ethtool sample = %#v, want none after read error", sample)
		}
	}
}

func assertSample(t *testing.T, snapshot ScrapeSnapshot, want sampleWant) {
	t.Helper()
	for _, sample := range snapshot.Samples {
		if sample.Name != want.name || sample.Source != want.source {
			continue
		}
		if sample.Value != want.value {
			t.Fatalf("%s value = %v, want %v", want.name, sample.Value, want.value)
		}
		if sample.Scope != MetricScopeHost {
			t.Fatalf("%s scope = %q, want %q", want.name, sample.Scope, MetricScopeHost)
		}
		if sample.Device != want.device || sample.Ifname != want.ifname || sample.ParentIfname != want.parentIfname || sample.Port != want.port {
			t.Fatalf("%s labels = %#v, want device=%q ifname=%q parent=%q port=%q", want.name, sample, want.device, want.ifname, want.parentIfname, want.port)
		}
		if sample.IsRoot != want.isRoot || sample.Kind != want.kind {
			t.Fatalf("%s root/kind = %v/%q, want %v/%q", want.name, sample.IsRoot, sample.Kind, want.isRoot, want.kind)
		}
		return
	}
	t.Fatalf("sample %s source %s not found in %#v", want.name, want.source, snapshot.Samples)
}

func createFakeHostSysfs(t *testing.T, root string) scraperPaths {
	t.Helper()

	infinibandPath := filepath.Join(root, "sys", "class", "infiniband")
	netPath := filepath.Join(root, "sys", "class", "net")
	realDevicePath := filepath.Join(root, "devices", "virtual", "infiniband", "rxe_eth1")
	mkdirAll(t, infinibandPath, realDevicePath)

	devicePath := filepath.Join(infinibandPath, "rxe_eth1")
	mkdirAll(t,
		filepath.Join(realDevicePath, "ports", "1", "hw_counters"),
		filepath.Join(realDevicePath, "ports", "1", "counters"),
		filepath.Join(realDevicePath, "ports", "1", "gid_attrs", "ndevs"),
		filepath.Join(realDevicePath, "tc", "1"),
		filepath.Join(netPath, "eth1"),
	)
	symlink(t, realDevicePath, devicePath)
	writeFile(t, filepath.Join(devicePath, "ports", "1", "gid_attrs", "ndevs", "0"), "eth1\n")
	writeFile(t, filepath.Join(devicePath, "ports", "1", "hw_counters", "port_rcv_data"), "256\n")
	writeFile(t, filepath.Join(devicePath, "ports", "1", "counters", "port_xmit_packets"), "7\n")
	writeFile(t, filepath.Join(devicePath, "tc", "1", "traffic_class"), "Global tclass=5\n")
	writeFile(t, filepath.Join(netPath, "eth1", "speed"), "100000\n")
	writeFile(t, filepath.Join(netPath, "eth1", "mtu"), "9000\n")
	writeFile(t, filepath.Join(netPath, "eth1", "operstate"), "up\n")

	return scraperPaths{
		infinibandClassPath: infinibandPath,
		netClassPath:        netPath,
		hostNetNSPath:       "",
		hostProcPath:        filepath.Join(root, "host", "proc"),
		containerdTaskPath:  filepath.Join(root, "containerd"),
		hostMountNSPID:      0,
	}
}

// addFakeHostVF adds an SR-IOV VF mlx5_vf/eth1v0 whose physfn is eth1's PCI
// device, with its netdev still visible in the host netns.
func addFakeHostVF(t *testing.T, root string, paths scraperPaths) {
	t.Helper()

	pfPciPath := filepath.Join(root, "devices", "pci", "0000:01:00.0")
	vfPciPath := filepath.Join(root, "devices", "pci", "0000:01:00.1")
	vfDevicePath := filepath.Join(vfPciPath, "infiniband", "mlx5_vf")
	mkdirAll(t,
		pfPciPath,
		filepath.Join(vfDevicePath, "ports", "1", "hw_counters"),
		filepath.Join(vfDevicePath, "ports", "1", "gid_attrs", "ndevs"),
		filepath.Join(vfDevicePath, "tc", "1"),
		filepath.Join(paths.netClassPath, "eth1v0"),
	)
	symlink(t, pfPciPath, filepath.Join(paths.netClassPath, "eth1", "device"))
	symlink(t, vfPciPath, filepath.Join(paths.netClassPath, "eth1v0", "device"))
	symlink(t, pfPciPath, filepath.Join(vfPciPath, "physfn"))
	symlink(t, vfDevicePath, filepath.Join(paths.infinibandClassPath, "mlx5_vf"))
	writeFile(t, filepath.Join(vfDevicePath, "ports", "1", "gid_attrs", "ndevs", "0"), "eth1v0\n")
	writeFile(t, filepath.Join(vfDevicePath, "ports", "1", "hw_counters", "port_rcv_data"), "128\n")
	writeFile(t, filepath.Join(vfDevicePath, "tc", "1", "traffic_class"), "Global tclass=3\n")
	writeFile(t, filepath.Join(paths.netClassPath, "eth1v0", "speed"), "100000\n")
	writeFile(t, filepath.Join(paths.netClassPath, "eth1v0", "mtu"), "9000\n")
	writeFile(t, filepath.Join(paths.netClassPath, "eth1v0", "operstate"), "up\n")
}

func mkdirAll(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
	}
}

func symlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Fatalf("symlink %s -> %s: %v", newname, oldname, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
