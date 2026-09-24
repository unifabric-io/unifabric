// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// callbackKindKeys maps callback kinds to stable names used in the cache file.
var callbackKindKeys = map[uint32]string{
	callbackPostSend:      "post_send",
	callbackSetSGE:        "wr_set_sge",
	callbackSetSGEList:    "wr_set_sge_list",
	callbackSetInlineData: "wr_set_inline_data",
	callbackSetInlineList: "wr_set_inline_data_list",
}

func callbackKindByName(name string) (uint32, bool) {
	for kind, key := range callbackKindKeys {
		if key == name {
			return kind, true
		}
	}
	return 0, false
}

// offsetCache persists provider callback file offsets keyed by library
// content identity, so a restarted agent can re-attach uprobes without
// waiting for the workload to create new verbs objects.
type offsetCache struct {
	mu      sync.Mutex
	path    string
	entries map[string]map[string]uint64
}

type offsetCacheFile struct {
	Version   int                          `json:"version"`
	Libraries map[string]map[string]uint64 `json:"libraries"`
}

func newOffsetCache(path string) (*offsetCache, error) {
	cache := &offsetCache{
		path:    path,
		entries: make(map[string]map[string]uint64),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cache, nil
		}
		return cache, err
	}
	var file offsetCacheFile
	if err := json.Unmarshal(data, &file); err != nil {
		return cache, fmt.Errorf("failed to decode offset cache, file=%q reason=%w", path, err)
	}
	for library, offsets := range file.Libraries {
		for name, offset := range offsets {
			if _, ok := callbackKindByName(name); !ok {
				continue
			}
			if cache.entries[library] == nil {
				cache.entries[library] = make(map[string]uint64)
			}
			cache.entries[library][name] = offset
		}
	}
	return cache, nil
}

func (c *offsetCache) libraryCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (c *offsetCache) lookup(library string) map[uint32]uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[uint32]uint64, len(c.entries[library]))
	for name, offset := range c.entries[library] {
		if kind, ok := callbackKindByName(name); ok {
			result[kind] = offset
		}
	}
	return result
}

// store records one offset and persists the cache. It reports whether the
// in-memory entry changed.
func (c *offsetCache) store(library string, kind uint32, offset uint64) (bool, error) {
	name, ok := callbackKindKeys[kind]
	if !ok {
		return false, fmt.Errorf("unknown callback type, type_id=%d", kind)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, exists := c.entries[library][name]; exists && existing == offset {
		return false, nil
	}
	if c.entries[library] == nil {
		c.entries[library] = make(map[string]uint64)
	}
	c.entries[library][name] = offset
	return true, c.persistLocked()
}

func (c *offsetCache) persistLocked() error {
	file := offsetCacheFile{Version: 1, Libraries: c.entries}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	temp := c.path + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temp, c.path)
}

// libraryContentID identifies a library file by content. GNU build-id is
// preferred and a SHA256 of the whole file is the fallback.
func libraryContentID(path string) (string, error) {
	buildID, err := gnuBuildID(path)
	if err == nil && buildID != "" {
		return "gnu:" + buildID, nil
	}
	hash, hashErr := fileSHA256(path)
	if hashErr != nil {
		return "", hashErr
	}
	return "sha256:" + hash, nil
}

func gnuBuildID(path string) (string, error) {
	file, err := elf.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	section := file.Section(".note.gnu.build-id")
	if section == nil {
		return "", nil
	}
	data, err := section.Data()
	if err != nil {
		return "", err
	}
	return parseGNUBuildID(data), nil
}

const noteTypeGNUBuildID = 3

func parseGNUBuildID(data []byte) string {
	for uint64(len(data)) >= 12 {
		nameSize := uint64(binary.LittleEndian.Uint32(data[0:4]))
		descSize := uint64(binary.LittleEndian.Uint32(data[4:8]))
		noteType := binary.LittleEndian.Uint32(data[8:12])
		nameEnd := 12 + alignUp4(nameSize)
		descEnd := nameEnd + alignUp4(descSize)
		if descEnd > uint64(len(data)) || nameEnd > uint64(len(data)) {
			return ""
		}
		if noteType == noteTypeGNUBuildID && descSize > 0 &&
			nameSize == 4 && string(data[12:16]) == "GNU\x00" {
			return hex.EncodeToString(data[nameEnd : nameEnd+descSize])
		}
		data = data[descEnd:]
	}
	return ""
}

func alignUp4(value uint64) uint64 {
	return (value + 3) &^ 3
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
