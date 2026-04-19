// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package rdmascraper

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
