// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package fabricnode

import (
	"testing"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
)

func TestClassifyTopologyNICSelectorOrder(t *testing.T) {
	tests := []struct {
		name     string
		ifname   string
		storage  string
		scaleUp  string
		scaleOut string
		want     topologyNICClass
	}{
		{
			name:     "storage wins over scale up and scale out",
			ifname:   "eth9",
			storage:  "interface=eth9",
			scaleUp:  "interface=eth9",
			scaleOut: "interface=eth*",
			want:     topologyNICClassStorage,
		},
		{
			name:     "scale up is excluded from FabricNode status",
			ifname:   "eth1",
			scaleUp:  "interface=eth1",
			scaleOut: "interface=eth*",
			want:     topologyNICClassNone,
		},
		{
			name:    "default scale out after filters",
			ifname:  "eth2",
			storage: "interface=eth9",
			scaleUp: "interface=eth8",
			want:    topologyNICClassScaleOut,
		},
		{
			name:     "configured scale out must match",
			ifname:   "eth2",
			scaleOut: "interface=eth1",
			want:     topologyNICClassNone,
		},
		{
			name:     "explicit scale out match",
			ifname:   "eth2",
			scaleOut: "interface=eth*",
			want:     topologyNICClassScaleOut,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reconciler := &fabricNodeReconciler{
				storageRdmaPattern:  mustRdmaInterfacePattern(t, tt.storage),
				scaleUpRdmaPattern:  mustRdmaInterfacePattern(t, tt.scaleUp),
				scaleOutRdmaPattern: mustRdmaInterfacePattern(t, tt.scaleOut),
			}
			got := reconciler.classifyTopologyNIC(v1beta1.NicInfo{Name: tt.ifname}, nil)
			if got != tt.want {
				t.Fatalf("classifyTopologyNIC() = %q, want %q", got, tt.want)
			}
		})
	}
}

func mustRdmaInterfacePattern(t *testing.T, selector string) RdmaInterfaceMethod {
	t.Helper()
	var method RdmaInterfaceMethod
	if err := method.CheckOrParsePattern(selector); err != nil {
		t.Fatalf("parse selector %q: %v", selector, err)
	}
	return method
}
