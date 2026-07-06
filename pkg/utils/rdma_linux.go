// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package utils

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Mellanox/rdmamap"
	"github.com/vishvananda/netlink"
)

const sysClassInfinibandDevicePath = "/sys/class/infiniband"

func GetRdmaInterfaces() ([]string, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}

	var interfaces []string
	for _, l := range links {
		if !IsSupportedRdmaNetdeviceType(l.Type()) {
			continue
		}
		if IsSriovVfForNetDev(l.Attrs().Name) {
			continue
		}
		if _, err := GetRdmaDeviceForNetdevice(l.Attrs().Name); err != nil {
			continue
		}
		interfaces = append(interfaces, l.Attrs().Name)
	}
	return interfaces, nil
}

func GetRdmaDeviceForNetdevice(netdevName string) (string, error) {
	if rdmaDevice, err := rdmamap.GetRdmaDeviceForNetdevice(netdevName); err == nil && rdmaDevice != "" {
		return rdmaDevice, nil
	}
	return getRdmaDeviceForNetdeviceBySysfsDevice(netdevName)
}

func getRdmaDeviceForNetdeviceBySysfsDevice(netdevName string) (string, error) {
	return getRdmaDeviceForNetdeviceBySysfsDeviceFromPaths(netdevName, SysClassNetDevicePath, sysClassInfinibandDevicePath)
}

func getRdmaDeviceForNetdeviceBySysfsDeviceFromPaths(netdevName, sysClassNetPath, sysClassInfinibandPath string) (string, error) {
	netdevDevicePath, err := filepath.EvalSymlinks(filepath.Join(sysClassNetPath, netdevName, "device"))
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(sysClassInfinibandPath)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		rdmaDevicePath, err := filepath.EvalSymlinks(filepath.Join(sysClassInfinibandPath, entry.Name(), "device"))
		if err != nil {
			continue
		}
		if rdmaDevicePath == netdevDevicePath {
			return entry.Name(), nil
		}
	}
	return "", fmt.Errorf("rdma device not found for netdev %s", netdevName)
}
