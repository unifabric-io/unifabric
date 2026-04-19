// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package rdmascraper

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"golang.org/x/sys/unix"
)

func withMountNamespace(hostProcPath string, pid int, fn func() error) (err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := unix.Unshare(unix.CLONE_FS); err != nil {
		return fmt.Errorf("unshare: %w", err)
	}

	originalNS, err := os.Open("/proc/self/ns/mnt")
	if err != nil {
		return fmt.Errorf("open original mount namespace: %w", err)
	}
	defer originalNS.Close()

	mntPath := filepath.Join(hostProcPath, strconv.Itoa(pid), "ns", "mnt")
	targetNS, err := os.Open(mntPath)
	if err != nil {
		return fmt.Errorf("open target mount namespace %s: %w", mntPath, err)
	}
	defer targetNS.Close()

	if err := unix.Setns(int(targetNS.Fd()), unix.CLONE_NEWNS); err != nil {
		return fmt.Errorf("switch to mount namespace %s: %w", mntPath, err)
	}
	defer func() {
		if restoreErr := unix.Setns(int(originalNS.Fd()), unix.CLONE_NEWNS); restoreErr != nil && err == nil {
			err = fmt.Errorf("switch back to original mount namespace: %w", restoreErr)
		}
	}()

	return fn()
}
