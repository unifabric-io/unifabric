// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package ebpfflow

import (
	"fmt"
	"path/filepath"
	"runtime"

	"golang.org/x/sys/unix"
)

// inHostNetns runs fn on a thread switched into the host network namespace.
// Some ib_core builds return the zero GID to sysfs readers outside the
// device's namespace, so GID tables must be read from there even though the
// agent itself is not hostNetwork.
func inHostNetns(fn func()) error {
	return inNetns(hostNetnsPath, fn)
}

// inNetnsOf runs fn inside the network namespace of the given process,
// which lets the agent read the RDMA devices a Pod owns exclusively.
func inNetnsOf(pid string, fn func()) error {
	return inNetns(filepath.Join("/proc", pid, "ns/net"), fn)
}

// inNetns runs fn on a fresh thread after setns into the namespace file.
//
// The goroutine locks its thread and never unlocks it, so the Go runtime
// destroys the thread when fn returns instead of reusing it. Since Go 1.10
// new threads are created from a template thread after LockOSThread, so the
// switched namespace does not leak into the rest of the process.
// An error means the switch failed and fn did not run.
func inNetns(path string, fn func()) error {
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			done <- fmt.Errorf("open %s: %w", path, err)
			return
		}
		defer unix.Close(fd)
		err = unix.Setns(fd, unix.CLONE_NEWNET)
		if err != nil {
			done <- fmt.Errorf("setns %s: %w", path, err)
			return
		}
		fn()
		done <- nil
	}()
	return <-done
}
