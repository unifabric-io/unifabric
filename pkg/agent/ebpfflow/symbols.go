// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"debug/elf"
	"fmt"
)

// resolvedSymbol is the attach point chosen for one exported function.
type resolvedSymbol struct {
	offset  uint64
	version string
}

// defaultSymbolOffset returns the file offset of the default version of an
// exported function. libibverbs exports ibv_modify_qp, ibv_create_qp and
// ibv_open_device twice, once as the IBVERBS_1.0 compatibility wrapper and
// once as the IBVERBS_1.1 implementation that every modern caller reaches.
// cilium/ebpf keeps whichever duplicate comes last in .dynsym, so the choice
// is made here instead.
func defaultSymbolOffset(path, name string) (resolvedSymbol, error) {
	f, err := elf.Open(path)
	if err != nil {
		return resolvedSymbol{}, err
	}
	defer f.Close()
	symbols, err := f.DynamicSymbols()
	if err != nil {
		return resolvedSymbol{}, err
	}
	chosen, ok := pickDefaultSymbol(symbols, name)
	if !ok {
		return resolvedSymbol{}, fmt.Errorf("symbol %q not found", name)
	}
	for _, prog := range f.Progs {
		if prog.Type != elf.PT_LOAD || prog.Flags&elf.PF_X == 0 {
			continue
		}
		if prog.Vaddr <= chosen.Value && chosen.Value < prog.Vaddr+prog.Memsz {
			return resolvedSymbol{
				offset:  chosen.Value - prog.Vaddr + prog.Off,
				version: symbolVersion(chosen),
			}, nil
		}
	}
	return resolvedSymbol{}, fmt.Errorf("symbol %q is outside executable segments", name)
}

// pickDefaultSymbol prefers a defined function whose version is not hidden.
// A hidden version is the "@" form that only satisfies old binaries, the
// default "@@" form is what new callers bind to.
func pickDefaultSymbol(symbols []elf.Symbol, name string) (elf.Symbol, bool) {
	var chosen elf.Symbol
	found := false
	for _, s := range symbols {
		if s.Name != name || elf.ST_TYPE(s.Info) != elf.STT_FUNC || s.Value == 0 {
			continue
		}
		if !found || (symbolHidden(chosen) && !symbolHidden(s)) {
			chosen = s
			found = true
		}
	}
	return chosen, found
}

func symbolHidden(s elf.Symbol) bool {
	return s.HasVersion && s.VersionIndex.IsHidden()
}

func symbolVersion(s elf.Symbol) string {
	if !s.HasVersion || s.Version == "" {
		return "unversioned"
	}
	return s.Version
}
