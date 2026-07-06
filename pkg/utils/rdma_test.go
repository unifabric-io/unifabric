// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package utils

import "testing"

func TestIsSupportedRdmaNetdeviceType(t *testing.T) {
	tests := []struct {
		name     string
		linkType string
		want     bool
	}{
		{
			name:     "ethernet device",
			linkType: "device",
			want:     true,
		},
		{
			name:     "IP over InfiniBand",
			linkType: "ipoib",
			want:     true,
		},
		{
			name:     "bond",
			linkType: "bond",
			want:     false,
		},
		{
			name:     "veth",
			linkType: "veth",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSupportedRdmaNetdeviceType(tt.linkType); got != tt.want {
				t.Fatalf("IsSupportedRdmaNetdeviceType(%q) = %v, want %v", tt.linkType, got, tt.want)
			}
		})
	}
}

func TestIsInfiniBandNetdeviceType(t *testing.T) {
	tests := []struct {
		name      string
		linkType  string
		encapType string
		want      bool
	}{
		{
			name:     "IP over InfiniBand link type",
			linkType: "ipoib",
			want:     true,
		},
		{
			name:      "InfiniBand encapsulation",
			linkType:  "device",
			encapType: "infiniband",
			want:      true,
		},
		{
			name:      "Ethernet device",
			linkType:  "device",
			encapType: "ether",
			want:      false,
		},
		{
			name:     "bond",
			linkType: "bond",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsInfiniBandNetdeviceType(tt.linkType, tt.encapType); got != tt.want {
				t.Fatalf("IsInfiniBandNetdeviceType(%q, %q) = %v, want %v", tt.linkType, tt.encapType, got, tt.want)
			}
		})
	}
}
