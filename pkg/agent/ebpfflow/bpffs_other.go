// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package ebpfflow

import "errors"

var errBPFFSUnsupported = errors.New("bpffs is only supported on Linux")

func isBPFFS(string) (bool, error) {
	return false, errBPFFSUnsupported
}

func mountBPFFS(string) error {
	return errBPFFSUnsupported
}
