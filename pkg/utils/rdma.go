// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package utils

const (
	netlinkTypeDevice   = "device"
	netlinkTypeIPoIB    = "ipoib"
	encapTypeInfiniBand = "infiniband"
)

// IsSupportedRdmaNetdeviceType reports whether a netlink link type can map to
// an RDMA device. IPoIB interfaces report "ipoib" rather than "device".
func IsSupportedRdmaNetdeviceType(linkType string) bool {
	return linkType == netlinkTypeDevice || linkType == netlinkTypeIPoIB
}

// IsInfiniBandNetdeviceType reports whether a netdevice represents IP over
// InfiniBand. These interfaces do not expose LLDP neighbors like Ethernet
// RDMA interfaces do.
func IsInfiniBandNetdeviceType(linkType, encapType string) bool {
	return linkType == netlinkTypeIPoIB || encapType == encapTypeInfiniBand
}
