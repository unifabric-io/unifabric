// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package ebpfflow

import "golang.org/x/sys/unix"

type unixTimespec = unix.Timespec

const clockMonotonic = unix.CLOCK_MONOTONIC

// clockGettime reads a POSIX clock, used to align bpf_ktime_get_ns with
// wall clock time.
func clockGettime(clock int32, ts *unixTimespec) error {
	return unix.ClockGettime(clock, ts)
}
