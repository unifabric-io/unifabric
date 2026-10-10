// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package ebpfflow

import "syscall"

// bpfFSMagic is BPF_FS_MAGIC from linux/magic.h.
const bpfFSMagic = 0xcafe4a11

func isBPFFS(path string) (bool, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return false, err
	}
	return st.Type == bpfFSMagic, nil
}

func mountBPFFS(path string) error {
	return syscall.Mount("bpf", path, "bpf", 0, "")
}
