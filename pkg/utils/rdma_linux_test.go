// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetRdmaDeviceForNetdeviceBySysfsDeviceFromPaths(t *testing.T) {
	tmp := t.TempDir()
	pciDevice := filepath.Join(tmp, "devices", "pci0000:15", "0000:19:00.0")
	sysClassNet := filepath.Join(tmp, "class", "net")
	sysClassInfiniband := filepath.Join(tmp, "class", "infiniband")

	mustMkdirAll(t, pciDevice)
	mustMkdirAll(t, filepath.Join(sysClassNet, "ibp25s0"))
	mustMkdirAll(t, filepath.Join(sysClassInfiniband, "mlx5_0"))
	mustSymlink(t, pciDevice, filepath.Join(sysClassNet, "ibp25s0", "device"))
	mustSymlink(t, pciDevice, filepath.Join(sysClassInfiniband, "mlx5_0", "device"))

	got, err := getRdmaDeviceForNetdeviceBySysfsDeviceFromPaths("ibp25s0", sysClassNet, sysClassInfiniband)
	if err != nil {
		t.Fatalf("getRdmaDeviceForNetdeviceBySysfsDeviceFromPaths() error = %v", err)
	}
	if got != "mlx5_0" {
		t.Fatalf("getRdmaDeviceForNetdeviceBySysfsDeviceFromPaths() = %q, want %q", got, "mlx5_0")
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Fatalf("symlink %s -> %s: %v", newname, oldname, err)
	}
}
