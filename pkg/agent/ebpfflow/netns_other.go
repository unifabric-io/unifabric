// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package ebpfflow

// inHostNetns has no namespace to switch on other platforms and runs fn
// directly, which keeps unit tests portable.
func inHostNetns(fn func()) error {
	fn()
	return nil
}

// inNetnsOf runs fn directly on other platforms.
func inNetnsOf(_ string, fn func()) error {
	fn()
	return nil
}
