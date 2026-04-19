// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package utils

import "fmt"

func GetRdmaInterfaces() ([]string, error) {
	return nil, fmt.Errorf("rdma interface discovery is only supported on linux")
}
