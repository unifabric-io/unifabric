// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
)

const SysClassNetDevicePath = "/sys/class/net"

// IsSriovVfForNetDev checks if the netdev is sriov vf or not by checking if
// the /sys/class/net/{ifName}/device/physfn exists.
func IsSriovVfForNetDev(iface string) bool {
	vfPhysfn := path.Join(SysClassNetDevicePath, iface, "device", "physfn")
	_, err := os.Lstat(vfPhysfn)
	return err == nil
}

func ContainsSlice(groupSw, nodeSw []v1beta1.ScaleOutSwitch) bool {
	if len(nodeSw) == 0 {
		return true
	}

	groupMap := make(map[string]bool)
	for _, sw := range groupSw {
		groupMap[sw.Name] = true
	}

	for _, sw := range nodeSw {
		if !groupMap[sw.Name] {
			return false
		}
	}
	return true
}

func HashNodesToShortSHA(logger *slog.Logger, elements []string) string {
	sort.Strings(elements)
	joined := strings.Join(elements, ",")
	hash := sha256.Sum256([]byte(joined))
	name := hex.EncodeToString(hash[:])[:7]
	logger.Debug("generated Scale group name", "elements", elements, "group", name)
	return name
}
