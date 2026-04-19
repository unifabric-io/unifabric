// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package utils

import (
	"github.com/Mellanox/rdmamap"
	"github.com/vishvananda/netlink"
)

func GetRdmaInterfaces() ([]string, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}

	var interfaces []string
	for _, l := range links {
		if l.Type() != "device" {
			continue
		}
		if IsSriovVfForNetDev(l.Attrs().Name) {
			continue
		}
		if !rdmamap.IsRDmaDeviceForNetdevice(l.Attrs().Name) {
			continue
		}
		interfaces = append(interfaces, l.Attrs().Name)
	}
	return interfaces, nil
}
