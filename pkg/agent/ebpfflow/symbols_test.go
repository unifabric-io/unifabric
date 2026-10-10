// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"debug/elf"
	"testing"
)

const hiddenVersionBit = 0x8000

func funcSymbol(name string, value uint64, version string, hidden bool) elf.Symbol {
	index := elf.VersionIndex(2)
	if hidden {
		index = elf.VersionIndex(2 | hiddenVersionBit)
	}
	return elf.Symbol{
		Name:         name,
		Info:         byte(elf.STB_GLOBAL)<<4 | byte(elf.STT_FUNC),
		Value:        value,
		Size:         64,
		HasVersion:   version != "",
		VersionIndex: index,
		Version:      version,
	}
}

func TestPickDefaultSymbolPrefersDefaultVersionRegardlessOfOrder(t *testing.T) {
	compat := funcSymbol("ibv_modify_qp", 0xe1a0, "IBVERBS_1.0", true)
	latest := funcSymbol("ibv_modify_qp", 0xb3e0, "IBVERBS_1.1", false)
	other := funcSymbol("ibv_create_qp", 0x1000, "IBVERBS_1.1", false)

	for name, order := range map[string][]elf.Symbol{
		"compat first": {other, compat, latest},
		"latest first": {other, latest, compat},
	} {
		got, ok := pickDefaultSymbol(order, "ibv_modify_qp")
		if !ok {
			t.Fatalf("%s: symbol not found", name)
		}
		if got.Value != latest.Value || got.Version != "IBVERBS_1.1" {
			t.Fatalf("%s: picked %#x %s, want %#x IBVERBS_1.1", name, got.Value, got.Version, latest.Value)
		}
	}
}

func TestPickDefaultSymbolFallsBackToOnlyDefinition(t *testing.T) {
	only := funcSymbol("ibv_qp_to_qp_ex", 0x2000, "IBVERBS_1.5", false)
	undefined := funcSymbol("ibv_import_device", 0, "", false)
	got, ok := pickDefaultSymbol([]elf.Symbol{undefined, only}, "ibv_qp_to_qp_ex")
	if !ok || got.Value != only.Value {
		t.Fatalf("picked %+v ok=%v, want %#x", got, ok, only.Value)
	}
	_, ok = pickDefaultSymbol([]elf.Symbol{undefined}, "ibv_import_device")
	if ok {
		t.Fatal("undefined symbol with value 0 must not be picked")
	}
	_, ok = pickDefaultSymbol([]elf.Symbol{only}, "ibv_modify_qp")
	if ok {
		t.Fatal("missing symbol must not be picked")
	}
}

func TestSymbolVersionLabels(t *testing.T) {
	plain := symbolVersion(funcSymbol("f", 1, "", false))
	if plain != "unversioned" {
		t.Fatalf("unversioned label = %q", plain)
	}
	versioned := symbolVersion(funcSymbol("f", 1, "IBVERBS_1.1", false))
	if versioned != "IBVERBS_1.1" {
		t.Fatalf("version label = %q", versioned)
	}
}
