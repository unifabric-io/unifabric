// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"encoding/binary"
	"testing"
)

func TestParseProcMappingAndFileOffset(t *testing.T) {
	mapping, ok := parseProcMapping("7f2a10001000-7f2a10025000 r-xp 00005000 fd:01 12345 /usr/lib/libmlx5.so.1 (deleted)")
	if !ok {
		t.Fatal("parseProcMapping returned false")
	}
	if mapping.start != 0x7f2a10001000 || mapping.end != 0x7f2a10025000 {
		t.Fatalf("unexpected address range: %#x-%#x", mapping.start, mapping.end)
	}
	if mapping.offset != 0x5000 {
		t.Fatalf("unexpected mapping offset: %#x", mapping.offset)
	}
	if mapping.path != "/usr/lib/libmlx5.so.1 (deleted)" {
		t.Fatalf("unexpected path: %q", mapping.path)
	}

	offset, ok := mapping.fileOffset(0x7f2a10004234)
	if !ok {
		t.Fatal("fileOffset did not recognize an in-range address")
	}
	if offset != 0x8234 {
		t.Fatalf("unexpected file offset: got %#x, want %#x", offset, uint64(0x8234))
	}
	if _, ok := mapping.fileOffset(mapping.end); ok {
		t.Fatal("fileOffset accepted the exclusive end address")
	}
}

func TestParseProcMappingRejectsInvalidInput(t *testing.T) {
	for _, line := range []string{
		"",
		"not-a-range r-xp 00000000 00:00 0",
		"1000-2000 r-xp not-hex 00:00 0 /tmp/a.so",
	} {
		if _, ok := parseProcMapping(line); ok {
			t.Fatalf("parseProcMapping accepted %q", line)
		}
	}
}

func TestDecodeCallbackEvent(t *testing.T) {
	data := make([]byte, 16)
	binary.LittleEndian.PutUint32(data[0:4], 4321)
	binary.LittleEndian.PutUint32(data[4:8], callbackSetSGEList)
	binary.LittleEndian.PutUint64(data[8:16], 0x7f0012345678)

	event, err := decodeCallbackEvent(data)
	if err != nil {
		t.Fatalf("decodeCallbackEvent: %v", err)
	}
	want := (callbackEvent{Tgid: 4321, Kind: callbackSetSGEList, Address: 0x7f0012345678})
	if event != want {
		t.Fatalf("unexpected event: got %+v, want %+v", event, want)
	}
	if _, err := decodeCallbackEvent(data[:15]); err == nil {
		t.Fatal("decodeCallbackEvent accepted a short event")
	}
}

func TestCallbackPrograms(t *testing.T) {
	tests := map[uint32]string{
		callbackPostSend:      "handle_post_send",
		callbackSetSGE:        "handle_wr_set_sge",
		callbackSetSGEList:    "handle_wr_set_sge_list",
		callbackSetInlineData: "handle_wr_set_inline_data",
		callbackSetInlineList: "handle_wr_set_inline_data_list",
	}
	for kind, want := range tests {
		got, ok := callbackProgram(kind)
		if !ok || got != want {
			t.Fatalf("callbackProgram(%d) = %q, %t; want %q, true", kind, got, ok, want)
		}
	}
	if _, ok := callbackProgram(999); ok {
		t.Fatal("callbackProgram accepted an unknown kind")
	}
}

func TestCallbackKindNames(t *testing.T) {
	tests := map[uint32]string{
		callbackPostSend:      "post_send",
		callbackSetSGE:        "wr_set_sge",
		callbackSetSGEList:    "wr_set_sge_list",
		callbackSetInlineData: "wr_set_inline_data",
		callbackSetInlineList: "wr_set_inline_data_list",
	}
	for kind, want := range tests {
		got := callbackKindName(kind)
		if got != want {
			t.Fatalf("callbackKindName(%d) = %q, want %q", kind, got, want)
		}
	}
	got := callbackKindName(999)
	if got != "unknown_type_999" {
		t.Fatalf("callbackKindName(999) = %q", got)
	}
}

func TestIsRDMAProviderLibrary(t *testing.T) {
	accepted := []string{
		"/usr/lib/x86_64-linux-gnu/libmlx5.so.1",
		"/usr/lib64/libibverbs/libmlx5-rdmav34.so",
		"/usr/lib/libibverbs.so.1.14.39.0 (deleted)",
		"/opt/rdma/libefa.so.1",
	}
	for _, path := range accepted {
		if !isRDMAProviderLibrary(path) {
			t.Fatalf("isRDMAProviderLibrary(%q) = false, want true", path)
		}
	}
	rejected := []string{
		"/usr/bin/calico-node",
		"/usr/lib/libcuda.so.1",
		"/usr/lib/libc.so.6",
		"[heap]",
	}
	for _, path := range rejected {
		if isRDMAProviderLibrary(path) {
			t.Fatalf("isRDMAProviderLibrary(%q) = true, want false", path)
		}
	}
}
