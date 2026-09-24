// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/cilium/ebpf"
)

// preparePinDir makes sure dir lives on a bpffs mount and exists. The parent
// of dir is the expected bpffs mount point. When it is not bpffs yet the agent
// mounts one there, which propagates to the host when the container mounts
// /sys/fs/bpf with bidirectional propagation. An empty dir disables pinning.
func preparePinDir(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}
	dir = filepath.Clean(dir)
	mountPoint := filepath.Dir(dir)
	onBPFFS, err := isBPFFS(mountPoint)
	if err != nil {
		return "", fmt.Errorf("failed to check bpffs mount point %q: %w", mountPoint, err)
	}
	if !onBPFFS {
		if err := mountBPFFS(mountPoint); err != nil {
			return "", fmt.Errorf("failed to mount bpffs at %q: %w", mountPoint, err)
		}
		onBPFFS, err = isBPFFS(mountPoint)
		if err != nil {
			return "", fmt.Errorf("failed to recheck bpffs mount point %q: %w", mountPoint, err)
		}
		if !onBPFFS {
			return "", fmt.Errorf("%q is still not bpffs after mounting", mountPoint)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create pin directory %q: %w", dir, err)
	}
	return dir, nil
}

// pinnedMapNames returns the sorted names of maps the spec asks to pin by name.
func pinnedMapNames(spec *ebpf.CollectionSpec) []string {
	var names []string
	for name, mapSpec := range spec.Maps {
		if mapSpec.Pinning == ebpf.PinByName {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// disableMapPinning clears the pinning attribute so the collection can load
// without a pin path. It returns the maps that were changed.
func disableMapPinning(spec *ebpf.CollectionSpec) []string {
	names := pinnedMapNames(spec)
	for _, name := range names {
		spec.Maps[name].Pinning = ebpf.PinNone
	}
	return names
}

// existingPins reports which pinned maps already have a file in pinDir.
func existingPins(spec *ebpf.CollectionSpec, pinDir string) []string {
	var found []string
	for _, name := range pinnedMapNames(spec) {
		if _, err := os.Stat(filepath.Join(pinDir, name)); err == nil {
			found = append(found, name)
		}
	}
	return found
}

// removePins deletes the pin files of every pinned map in the spec so a
// layout change between versions can recreate them. Missing files are fine.
func removePins(spec *ebpf.CollectionSpec, pinDir string) ([]string, error) {
	var removed []string
	for _, name := range pinnedMapNames(spec) {
		err := os.Remove(filepath.Join(pinDir, name))
		if err == nil {
			removed = append(removed, name)
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return removed, fmt.Errorf("failed to remove stale pin %q: %w", name, err)
		}
	}
	return removed, nil
}
