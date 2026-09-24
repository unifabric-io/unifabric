// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cilium/ebpf"
)

func pinTestSpec() *ebpf.CollectionSpec {
	return &ebpf.CollectionSpec{
		Maps: map[string]*ebpf.MapSpec{
			"qp_infos":        {Pinning: ebpf.PinByName},
			"qp_stats":        {Pinning: ebpf.PinByName},
			"callback_events": {Pinning: ebpf.PinNone},
		},
	}
}

func TestPinnedMapNamesAndDisable(t *testing.T) {
	spec := pinTestSpec()
	names := pinnedMapNames(spec)
	if len(names) != 2 || names[0] != "qp_infos" || names[1] != "qp_stats" {
		t.Fatalf("unexpected pinned maps: %v", names)
	}

	disabled := disableMapPinning(spec)
	if len(disabled) != 2 {
		t.Fatalf("unexpected disabled maps: %v", disabled)
	}
	if len(pinnedMapNames(spec)) != 0 {
		t.Fatal("pinning attribute still present after disable")
	}
}

func TestExistingAndRemovePins(t *testing.T) {
	spec := pinTestSpec()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "qp_infos"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	found := existingPins(spec, dir)
	if len(found) != 1 || found[0] != "qp_infos" {
		t.Fatalf("unexpected existing pins: %v", found)
	}

	removed, err := removePins(spec, dir)
	if err != nil {
		t.Fatalf("removePins: %v", err)
	}
	if len(removed) != 1 || removed[0] != "qp_infos" {
		t.Fatalf("unexpected removed pins: %v", removed)
	}
	if len(existingPins(spec, dir)) != 0 {
		t.Fatal("pin file still present after removal")
	}
}

func TestPreparePinDirDisabled(t *testing.T) {
	dir, err := preparePinDir("")
	if err != nil || dir != "" {
		t.Fatalf("empty pin dir should disable pinning, got %q err=%v", dir, err)
	}
}
