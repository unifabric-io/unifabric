// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package ebpfflow

import "errors"

type unixTimespec struct {
	Sec  int64
	Nsec int64
}

const clockMonotonic = 0

// clockGettime is unavailable off Linux, callers fall back to wall clock.
func clockGettime(int32, *unixTimespec) error {
	return errors.New("clock_gettime is only supported on Linux")
}
