// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestOffsetCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "offsets.json")
	cache, err := newOffsetCache(path)
	if err != nil {
		t.Fatalf("newOffsetCache: %v", err)
	}
	changed, err := cache.store("gnu:abc123", callbackPostSend, 0x1234)
	if err != nil || !changed {
		t.Fatalf("store returned changed=%t err=%v", changed, err)
	}
	changed, err = cache.store("gnu:abc123", callbackSetSGE, 0x5678)
	if err != nil || !changed {
		t.Fatalf("store returned changed=%t err=%v", changed, err)
	}
	changed, err = cache.store("gnu:abc123", callbackPostSend, 0x1234)
	if err != nil || changed {
		t.Fatalf("duplicate store returned changed=%t err=%v", changed, err)
	}

	reloaded, err := newOffsetCache(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.libraryCount() != 1 {
		t.Fatalf("library count = %d, want 1", reloaded.libraryCount())
	}
	offsets := reloaded.lookup("gnu:abc123")
	if len(offsets) != 2 || offsets[callbackPostSend] != 0x1234 ||
		offsets[callbackSetSGE] != 0x5678 {
		t.Fatalf("unexpected offsets after reload: %+v", offsets)
	}
	if len(reloaded.lookup("gnu:other")) != 0 {
		t.Fatal("lookup returned offsets for an unknown library")
	}
}

func TestOffsetCacheIgnoresUnknownKindNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "offsets.json")
	content := `{
  "version": 1,
  "libraries": {
    "gnu:abc": {
      "post_send": 16,
      "made_up_callback": 32
    }
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cache, err := newOffsetCache(path)
	if err != nil {
		t.Fatalf("newOffsetCache: %v", err)
	}
	offsets := cache.lookup("gnu:abc")
	if len(offsets) != 1 || offsets[callbackPostSend] != 16 {
		t.Fatalf("unexpected offsets: %+v", offsets)
	}
}

func TestOffsetCacheRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "offsets.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cache, err := newOffsetCache(path)
	if err == nil {
		t.Fatal("newOffsetCache accepted corrupt content")
	}
	if cache == nil || cache.libraryCount() != 0 {
		t.Fatal("corrupt cache should still yield an empty usable cache")
	}
}

func TestCallbackKindKeysMatchPrograms(t *testing.T) {
	for kind, name := range callbackKindKeys {
		if _, ok := callbackProgram(kind); !ok {
			t.Fatalf("cache kind %d (%s) has no BPF program", kind, name)
		}
		back, ok := callbackKindByName(name)
		if !ok || back != kind {
			t.Fatalf("callbackKindByName(%q) = %d, %t", name, back, ok)
		}
	}
	if _, ok := callbackKindByName("unknown"); ok {
		t.Fatal("callbackKindByName accepted an unknown name")
	}
}

func TestParseGNUBuildID(t *testing.T) {
	id := []byte{0xde, 0xad, 0xbe, 0xef, 0x01}
	note := make([]byte, 12+4+8)
	binary.LittleEndian.PutUint32(note[0:4], 4)
	binary.LittleEndian.PutUint32(note[4:8], uint32(len(id)))
	binary.LittleEndian.PutUint32(note[8:12], noteTypeGNUBuildID)
	copy(note[12:16], "GNU\x00")
	copy(note[16:], id)
	if got := parseGNUBuildID(note); got != hex.EncodeToString(id) {
		t.Fatalf("parseGNUBuildID = %q, want %q", got, hex.EncodeToString(id))
	}
	if parseGNUBuildID(nil) != "" {
		t.Fatal("parseGNUBuildID accepted empty data")
	}
	if parseGNUBuildID(note[:10]) != "" {
		t.Fatal("parseGNUBuildID accepted a truncated note")
	}
}
