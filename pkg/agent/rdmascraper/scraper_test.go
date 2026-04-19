// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package rdmascraper

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type fakeFabricNodeClient struct {
	node *v1beta1.FabricNode
}

func (f fakeFabricNodeClient) GetFabricNode() *v1beta1.FabricNode {
	return f.node
}

func (fakeFabricNodeClient) IsStorageLeader() bool {
	return false
}

func TestRuntimeScraperReturnsEmptySnapshotWithoutFabricNode(t *testing.T) {
	scraper := NewRuntimeScraper(fakeFabricNodeClient{}, discardLogger(), config.NodeTopologyDiscoveryConfig{})

	snapshot, err := scraper.Scrape(context.Background())
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	if snapshot.NodeName != "" {
		t.Fatalf("node name = %q, want empty", snapshot.NodeName)
	}
	if len(snapshot.Devices.Devices) != 0 || len(snapshot.Samples) != 0 || len(snapshot.Warnings) != 0 {
		t.Fatalf("snapshot = %#v, want empty", snapshot)
	}
}

func TestRuntimeScraperSeedsSnapshotFromFabricNode(t *testing.T) {
	node := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
	}
	scraper := NewRuntimeScraper(fakeFabricNodeClient{node: node}, discardLogger(), config.NodeTopologyDiscoveryConfig{
		StorageInterfaceSelector: "interface=eth9",
	})
	scraper.paths.infinibandClassPath = t.TempDir()
	scraper.paths.hostMountNSPID = 0
	scraper.paths.hostNetNSPath = ""

	snapshot, err := scraper.Scrape(context.Background())
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	if snapshot.NodeName != "node-1" {
		t.Fatalf("node name = %q, want node-1", snapshot.NodeName)
	}
	if got := interfaceKind(scraper.kindMatcher, "eth9", ""); got != rdmaInterfaceKindStorage {
		t.Fatalf("interface kind = %q, want %q", got, rdmaInterfaceKindStorage)
	}
}

func TestInterfaceKindSelectorOrder(t *testing.T) {
	tests := []struct {
		name       string
		cfg        config.NodeTopologyDiscoveryConfig
		ifname     string
		parentName string
		want       string
	}{
		{
			name: "storage wins over scale up and scale out",
			cfg: config.NodeTopologyDiscoveryConfig{
				StorageInterfaceSelector:  "interface=eth9",
				ScaleUpInterfaceSelector:  "interface=eth9",
				ScaleOutInterfaceSelector: "interface=eth*",
			},
			ifname: "eth9",
			want:   rdmaInterfaceKindStorage,
		},
		{
			name: "scale up wins over scale out",
			cfg: config.NodeTopologyDiscoveryConfig{
				ScaleUpInterfaceSelector:  "interface=eth1",
				ScaleOutInterfaceSelector: "interface=eth*",
			},
			ifname: "eth1",
			want:   rdmaInterfaceKindScaleUp,
		},
		{
			name: "explicit scale out",
			cfg: config.NodeTopologyDiscoveryConfig{
				ScaleOutInterfaceSelector: "interface=eth*",
			},
			ifname: "eth2",
			want:   rdmaInterfaceKindScaleOut,
		},
		{
			name: "default scale out after storage and scale up filters",
			cfg: config.NodeTopologyDiscoveryConfig{
				StorageInterfaceSelector: "interface=eth9",
				ScaleUpInterfaceSelector: "interface=eth8",
			},
			ifname: "eth2",
			want:   rdmaInterfaceKindScaleOut,
		},
		{
			name: "configured scale out must match",
			cfg: config.NodeTopologyDiscoveryConfig{
				ScaleOutInterfaceSelector: "interface=eth1",
			},
			ifname: "eth2",
			want:   "",
		},
		{
			name: "parent interface is checked first",
			cfg: config.NodeTopologyDiscoveryConfig{
				StorageInterfaceSelector:  "interface=eth9",
				ScaleOutInterfaceSelector: "interface=eth*",
			},
			ifname:     "net1",
			parentName: "eth9",
			want:       rdmaInterfaceKindStorage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher := buildInterfaceKindMatcher(tt.cfg)
			if got := interfaceKind(matcher, tt.ifname, tt.parentName); got != tt.want {
				t.Fatalf("interfaceKind() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRuntimeScraperReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	scraper := NewRuntimeScraper(fakeFabricNodeClient{}, discardLogger(), config.NodeTopologyDiscoveryConfig{})
	if _, err := scraper.Scrape(ctx); err == nil {
		t.Fatal("scrape error = nil, want context cancellation error")
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
