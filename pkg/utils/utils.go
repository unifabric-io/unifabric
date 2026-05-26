// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"os"
	"path"
)

const SysClassNetDevicePath = "/sys/class/net"

// IsSriovVfForNetDev checks if the netdev is sriov vf or not by checking if
// the /sys/class/net/{ifName}/device/physfn exists.
func IsSriovVfForNetDev(iface string) bool {
	vfPhysfn := path.Join(SysClassNetDevicePath, iface, "device", "physfn")
	_, err := os.Lstat(vfPhysfn)
	return err == nil
}
