// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

const (
	discoveryProcessExec  uint32 = 0
	callbackPostSend      uint32 = 1
	callbackSetSGE        uint32 = 2
	callbackSetSGEList    uint32 = 3
	callbackSetInlineData uint32 = 4
	callbackSetInlineList uint32 = 5
	// eventQPRTR is emitted after qp_infos gained an entry, Address is the QPN.
	eventQPRTR uint32 = 6
)

var errBootstrapTargetGone = errors.New("bootstrap target file disappeared")

type callbackEvent struct {
	Tgid    uint32
	Kind    uint32
	Address uint64
}

type procMapping struct {
	start  uint64
	end    uint64
	offset uint64
	perms  string
	dev    string
	inode  string
	path   string
}

func (m procMapping) executable() bool {
	return strings.Contains(m.perms, "x")
}

func (m procMapping) readable() bool {
	return strings.Contains(m.perms, "r")
}

func (m procMapping) writable() bool {
	return strings.Contains(m.perms, "w")
}

func (m procMapping) identity() string {
	return m.dev + ":" + m.inode
}

func (m procMapping) fileOffset(address uint64) (uint64, bool) {
	if address < m.start || address >= m.end {
		return 0, false
	}
	return address - m.start + m.offset, true
}

// attacher first attaches bootstrap probes to public libibverbs symbols. Those
// probes publish provider callback virtual addresses from live verbs objects.
// The ibv_modify_qp probe also records each RTR request's destination GID.
// It then translates each address through the originating process's maps and
// attaches the corresponding data-plane program by file offset. Learned file
// offsets are persisted per library content identity so a restarted agent can
// re-attach without waiting for new verbs objects.
type attacher struct {
	progs   map[string]*ebpf.Program
	offsets *offsetCache

	scanMu               sync.Mutex
	mu                   sync.Mutex
	bootstrapLinks       map[string][]link.Link
	bootstrapUnsupported map[string]bool
	scannedRoots         map[string]bool
	contentIDs           map[string]string
	dataLinks            map[string]link.Link
	pending              map[callbackEvent]int
}

func newAttacher(progs map[string]*ebpf.Program, offsets *offsetCache) *attacher {
	return &attacher{
		progs:                progs,
		offsets:              offsets,
		bootstrapLinks:       make(map[string][]link.Link),
		bootstrapUnsupported: make(map[string]bool),
		scannedRoots:         make(map[string]bool),
		contentIDs:           make(map[string]string),
		dataLinks:            make(map[string]link.Link),
		pending:              make(map[callbackEvent]int),
	}
}

func (a *attacher) scan() {
	a.scanMu.Lock()
	defer a.scanMu.Unlock()

	entries, err := os.ReadDir("/proc")
	if err != nil {
		logDedup("process_directory_error", slog.LevelError, "read_process_directory", "path", "/proc", "reason", err)
		return
	}

	seenLibraries := make(map[string]bool)
	for _, entry := range entries {
		pid := entry.Name()
		if !isNumeric(pid) {
			continue
		}
		a.scanProcess(pid, seenLibraries)
	}
	a.retryPending()
	bootstrap, unsupported, data, pending := a.state()
	logDedup("process_scan_complete", slog.LevelDebug, "process_scan_complete",
		"discovered_libraries", len(seenLibraries), "attached_bootstrap_libraries", bootstrap,
		"unsupported_libraries", unsupported, "attached_data_callbacks", data,
		"pending_callbacks", pending)
}

func (a *attacher) scanTgid(tgid uint32) {
	if tgid == 0 {
		return
	}
	a.scanMu.Lock()
	defer a.scanMu.Unlock()

	a.scanProcess(strconv.FormatUint(uint64(tgid), 10), make(map[string]bool))
}

func (a *attacher) scanProcess(pid string, seenLibraries map[string]bool) {
	a.scanProcessRoot(pid, seenLibraries)

	mappings, err := readProcMappings(pid)
	if err != nil {
		return
	}
	for _, mapping := range mappings {
		if !mapping.executable() || mapping.inode == "0" {
			continue
		}
		if isLibibverbs(mapping.path) {
			target, err := mappingTarget(pid, mapping)
			if err != nil {
				logDedup("libibverbs_mapping:"+pid+":"+mapping.path, slog.LevelDebug, "libibverbs_mapping",
					"process", pid, "path", mapping.path, "status", "inaccessible", "reason", err)
				continue
			}
			a.attachBootstrapOnce(target, seenLibraries)
		}
		if isRDMAProviderLibrary(mapping.path) {
			a.replayCachedCallbacks(pid, mapping)
		}
	}
}

// replayCachedCallbacks re-attaches previously learned callback offsets for a
// provider library already mapped by a running process.
func (a *attacher) replayCachedCallbacks(pid string, mapping procMapping) {
	target, err := mappingTarget(pid, mapping)
	if err != nil {
		return
	}
	contentID, err := a.libraryContentIDFor(mapping.identity(), target)
	if err != nil {
		logDedup("library_content_id:"+target, slog.LevelDebug, "library_content_id_failed",
			"process", pid, "file", target, "reason", err)
		return
	}
	for kind, fileOffset := range a.offsets.lookup(contentID) {
		attached, err := a.attachOffset(target, mapping.identity(), kind, fileOffset)
		if err != nil {
			logDedup(fmt.Sprintf("replay_cached_callback:%s:%d", target, kind), slog.LevelError, "replay_cached_callback",
				"file", target, "library", contentID, "callback_type", callbackKindName(kind),
				"offset", fmt.Sprintf("%#x", fileOffset), "reason", err)
			continue
		}
		if attached {
			logger.Info("provider_callback_restored",
				"source", "offset_cache", "callback_type", callbackKindName(kind), "file", target,
				"library", contentID, "offset", fmt.Sprintf("%#x", fileOffset))
		}
	}
}

func (a *attacher) libraryContentIDFor(identity, target string) (string, error) {
	a.mu.Lock()
	if contentID, ok := a.contentIDs[identity]; ok {
		a.mu.Unlock()
		return contentID, nil
	}
	a.mu.Unlock()
	contentID, err := libraryContentID(target)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	a.contentIDs[identity] = contentID
	a.mu.Unlock()
	return contentID, nil
}

var libibverbsGlobs = []string{
	"lib*/libibverbs.so*",
	"lib/*/libibverbs.so*",
	"usr/lib*/libibverbs.so*",
	"usr/lib/*/libibverbs.so*",
	"usr/local/lib*/libibverbs.so*",
	"usr/local/lib/*/libibverbs.so*",
	"opt/*/lib*/libibverbs.so*",
	"opt/*/lib/*/libibverbs.so*",
}

func (a *attacher) scanProcessRoot(pid string, seenLibraries map[string]bool) {
	root := filepath.Join("/proc", pid, "root")
	mountNamespaceID, err := fileIdentity(filepath.Join("/proc", pid, "ns/mnt"))
	if err != nil || !a.markRootScanned(mountNamespaceID) {
		return
	}
	for _, pattern := range libibverbsGlobs {
		matches, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			continue
		}
		for _, target := range matches {
			a.attachBootstrapOnce(target, seenLibraries)
		}
	}
}

func (a *attacher) attachBootstrapOnce(target string, seenLibraries map[string]bool) {
	id, err := fileIdentity(target)
	if err != nil || seenLibraries[id] || a.bootstrapKnown(id) {
		return
	}
	seenLibraries[id] = true

	links, names, err := a.attachBootstrap(target)
	if err != nil {
		if errors.Is(err, errBootstrapTargetGone) {
			logger.Debug("bootstrap_attach_deferred", "file", target, "reason", err)
			return
		}
		a.markBootstrapUnsupported(id)
		logger.Error("attach_bootstrap_probes", "file", target, "reason", err)
		return
	}
	a.storeBootstrap(id, links)
	logger.Info("bootstrap_probes_attached", "count", len(links), "file", target, "symbols", strings.Join(names, ","))
}

type bootstrapProbe struct {
	symbol  string
	program string
	ret     bool
}

func (a *attacher) attachBootstrap(path string) ([]link.Link, []string, error) {
	executable, err := link.OpenExecutable(path)
	if err != nil {
		return nil, nil, err
	}
	probes := []bootstrapProbe{
		{symbol: "ibv_open_device", program: "handle_context_return", ret: true},
		{symbol: "ibv_import_device", program: "handle_context_return", ret: true},
		{symbol: "ibv_create_qp", program: "handle_qp_return", ret: true},
		{symbol: "ibv_modify_qp", program: "handle_modify_qp"},
		{symbol: "ibv_qp_to_qp_ex", program: "handle_qp_ex_return", ret: true},
	}

	var attached []link.Link
	var names []string
	var failures []string
	for _, probe := range probes {
		program := a.progs[probe.program]
		if program == nil {
			failures = append(failures, probe.program+": BPF program is missing")
			continue
		}
		resolved, err := defaultSymbolOffset(path, probe.symbol)
		if err != nil {
			failures = append(failures, probe.symbol+": "+err.Error())
			continue
		}
		opts := &link.UprobeOptions{Address: resolved.offset}
		var uprobe link.Link
		if probe.ret {
			uprobe, err = executable.Uretprobe(probe.symbol, program, opts)
		} else {
			uprobe, err = executable.Uprobe(probe.symbol, program, opts)
		}
		if err != nil {
			failures = append(failures, probe.symbol+": "+err.Error())
			continue
		}
		attached = append(attached, uprobe)
		names = append(names, fmt.Sprintf("%s@%s+%#x", probe.symbol, resolved.version, resolved.offset))
	}
	if len(failures) > 0 {
		if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
			for _, uprobe := range attached {
				_ = uprobe.Close()
			}
			return nil, nil, fmt.Errorf("%w, file=%q", errBootstrapTargetGone, path)
		}
	}
	if len(attached) == 0 {
		return nil, nil, fmt.Errorf("no usable public libibverbs symbols, details=%s",
			strings.Join(failures, ", "))
	}
	if len(failures) > 0 {
		logger.Warn("attach_bootstrap_probes",
			"file", path, "status", "partially_skipped", "details", strings.Join(failures, ", "))
	}
	return attached, names, nil
}

func (a *attacher) handleCallback(event callbackEvent) {
	if event.Tgid == 0 || event.Address == 0 {
		logger.Debug("callback_dropped",
			"process", event.Tgid, "callback_type", event.Kind, "address", fmt.Sprintf("%#x", event.Address),
			"reason", "invalid_event")
		return
	}
	logger.Debug("callback_received",
		"process", event.Tgid, "callback_type", callbackKindName(event.Kind),
		"address", fmt.Sprintf("%#x", event.Address))
	if err := a.attachCallback(event); err != nil {
		if !tgidAlive(event.Tgid) {
			a.mu.Lock()
			delete(a.pending, event)
			a.mu.Unlock()
			logger.Debug("callback_dropped",
				"process", event.Tgid, "callback_type", callbackKindName(event.Kind),
				"address", fmt.Sprintf("%#x", event.Address), "reason", "process_exited", "error", err)
			return
		}
		a.mu.Lock()
		attempt := a.pending[event] + 1
		a.pending[event] = attempt
		a.mu.Unlock()
		logger.Debug("callback_attach_deferred",
			"process", event.Tgid, "callback_type", callbackKindName(event.Kind),
			"address", fmt.Sprintf("%#x", event.Address), "attempts", attempt, "reason", err)
	}
}

func (a *attacher) attachCallback(event callbackEvent) error {
	pid := strconv.FormatUint(uint64(event.Tgid), 10)
	mappings, err := readProcMappings(pid)
	if err != nil {
		return err
	}
	var provider procMapping
	var fileOffset uint64
	found := false
	for _, mapping := range mappings {
		if !mapping.executable() || mapping.inode == "0" {
			continue
		}
		if offset, contains := mapping.fileOffset(event.Address); contains {
			provider = mapping
			fileOffset = offset
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("callback address is not in a file-backed executable mapping")
	}

	target, err := mappingTarget(pid, provider)
	if err != nil {
		return err
	}
	attached, err := a.attachOffset(target, provider.identity(), event.Kind, fileOffset)
	if err != nil {
		return err
	}
	a.mu.Lock()
	delete(a.pending, event)
	a.mu.Unlock()
	if attached {
		logger.Info("provider_callback_attached",
			"callback_type", callbackKindName(event.Kind), "file", target,
			"offset", fmt.Sprintf("%#x", fileOffset), "source_process", event.Tgid)
	} else {
		logger.Debug("provider_callback_already_attached",
			"process", event.Tgid, "callback_type", callbackKindName(event.Kind),
			"address", fmt.Sprintf("%#x", event.Address), "file", provider.path,
			"offset", fmt.Sprintf("%#x", fileOffset))
	}
	a.rememberOffset(provider.identity(), target, event.Kind, fileOffset)
	return nil
}

// attachOffset attaches one data-plane program by file offset. It reports
// whether a new uprobe was created.
func (a *attacher) attachOffset(target, identity string, kind uint32,
	fileOffset uint64) (bool, error) {
	programName, ok := callbackProgram(kind)
	if !ok {
		return false, fmt.Errorf("unknown callback type, type_id=%d", kind)
	}
	program := a.progs[programName]
	if program == nil {
		return false, fmt.Errorf("BPF program is missing, program=%s", programName)
	}
	key := fmt.Sprintf("%s:%x:%d", identity, fileOffset, kind)
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.dataLinks[key]; exists {
		return false, nil
	}
	executable, err := link.OpenExecutable(target)
	if err != nil {
		return false, err
	}
	uprobe, err := executable.Uprobe("", program, &link.UprobeOptions{Address: fileOffset})
	if err != nil {
		return false, err
	}
	a.dataLinks[key] = uprobe
	return true, nil
}

func (a *attacher) rememberOffset(identity, target string, kind uint32, fileOffset uint64) {
	contentID, err := a.libraryContentIDFor(identity, target)
	if err != nil {
		logger.Warn("calculate_library_content_id", "file", target, "reason", err)
		return
	}
	changed, err := a.offsets.store(contentID, kind, fileOffset)
	if err != nil {
		logger.Warn("persist_callback_offset", "file", target, "library", contentID, "reason", err)
	}
	if changed {
		logger.Debug("callback_offset_cached",
			"library", contentID, "callback_type", callbackKindName(kind),
			"offset", fmt.Sprintf("%#x", fileOffset))
	}
}

func (a *attacher) retryPending() {
	a.mu.Lock()
	events := make([]callbackEvent, 0, len(a.pending))
	dropped := 0
	for event, attempts := range a.pending {
		if !tgidAlive(event.Tgid) {
			logger.Debug("callback_retry_dropped",
				"process", event.Tgid, "callback_type", callbackKindName(event.Kind),
				"address", fmt.Sprintf("%#x", event.Address), "reason", "process_exited", "attempts", attempts)
			delete(a.pending, event)
			dropped++
			continue
		}
		if attempts >= 20 {
			logger.Debug("callback_retry_dropped",
				"process", event.Tgid, "callback_type", callbackKindName(event.Kind),
				"address", fmt.Sprintf("%#x", event.Address), "reason", "retry_limit_reached",
				"attempts", attempts)
			delete(a.pending, event)
			dropped++
			continue
		}
		events = append(events, event)
	}
	a.mu.Unlock()
	logDedup("callback_retry_summary", slog.LevelDebug, "callback_retry_summary",
		"pending", len(events), "dropped", dropped)
	for _, event := range events {
		a.handleCallback(event)
	}
}

func (a *attacher) state() (bootstrap, unsupported, data, pending int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.bootstrapLinks), len(a.bootstrapUnsupported), len(a.dataLinks), len(a.pending)
}

func (a *attacher) bootstrapKnown(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, attached := a.bootstrapLinks[id]
	return attached || a.bootstrapUnsupported[id]
}

func (a *attacher) markBootstrapUnsupported(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.bootstrapUnsupported[id] = true
}

func (a *attacher) storeBootstrap(id string, links []link.Link) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.bootstrapLinks[id] = links
}

func (a *attacher) markRootScanned(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.scannedRoots[id] {
		return false
	}
	a.scannedRoots[id] = true
	return true
}

func callbackProgram(kind uint32) (string, bool) {
	switch kind {
	case callbackPostSend:
		return "handle_post_send", true
	case callbackSetSGE:
		return "handle_wr_set_sge", true
	case callbackSetSGEList:
		return "handle_wr_set_sge_list", true
	case callbackSetInlineData:
		return "handle_wr_set_inline_data", true
	case callbackSetInlineList:
		return "handle_wr_set_inline_data_list", true
	default:
		return "", false
	}
}

func callbackKindName(kind uint32) string {
	switch kind {
	case callbackPostSend:
		return "post_send"
	case callbackSetSGE:
		return "wr_set_sge"
	case callbackSetSGEList:
		return "wr_set_sge_list"
	case callbackSetInlineData:
		return "wr_set_inline_data"
	case callbackSetInlineList:
		return "wr_set_inline_data_list"
	default:
		return "unknown_type_" + strconv.FormatUint(uint64(kind), 10)
	}
}

func isLibibverbs(path string) bool {
	path = strings.TrimSuffix(path, " (deleted)")
	return strings.Contains(filepath.Base(path), "libibverbs.so")
}

func isRDMAProviderLibrary(path string) bool {
	cleanPath := strings.ToLower(strings.TrimSuffix(path, " (deleted)"))
	base := filepath.Base(cleanPath)
	if strings.Contains(cleanPath, "/libibverbs/") {
		return true
	}
	for _, prefix := range []string{
		"libibverbs",
		"libbnxt_re",
		"libcxgb4",
		"libefa",
		"liberdma",
		"libhfi1verbs",
		"libhns",
		"libipathverbs",
		"libirdma",
		"libionic",
		"libmana",
		"libmlx4",
		"libmlx5",
		"libmthca",
		"libocrdma",
		"libqedr",
		"librxe",
		"libsiw",
		"libvmw_pvrdma",
	} {
		if strings.HasPrefix(base, prefix) {
			return true
		}
	}
	return false
}

func readProcMappings(pid string) ([]procMapping, error) {
	data, err := os.ReadFile(filepath.Join("/proc", pid, "maps"))
	if err != nil {
		return nil, err
	}
	var mappings []procMapping
	for _, line := range strings.Split(string(data), "\n") {
		if mapping, ok := parseProcMapping(line); ok {
			mappings = append(mappings, mapping)
		}
	}
	return mappings, nil
}

func parseProcMapping(line string) (procMapping, bool) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return procMapping{}, false
	}
	rangeFields := strings.SplitN(fields[0], "-", 2)
	if len(rangeFields) != 2 {
		return procMapping{}, false
	}
	start, err := strconv.ParseUint(rangeFields[0], 16, 64)
	if err != nil {
		return procMapping{}, false
	}
	end, err := strconv.ParseUint(rangeFields[1], 16, 64)
	if err != nil {
		return procMapping{}, false
	}
	offset, err := strconv.ParseUint(fields[2], 16, 64)
	if err != nil {
		return procMapping{}, false
	}
	path := ""
	if len(fields) > 5 {
		path = strings.Join(fields[5:], " ")
	}
	return procMapping{
		start:  start,
		end:    end,
		offset: offset,
		perms:  fields[1],
		dev:    fields[3],
		inode:  fields[4],
		path:   path,
	}, true
}

func mappingTarget(pid string, mapping procMapping) (string, error) {
	deleted := strings.HasSuffix(mapping.path, " (deleted)")
	path := strings.TrimSuffix(mapping.path, " (deleted)")
	if !deleted && filepath.IsAbs(path) {
		target := filepath.Join("/proc", pid, "root", path)
		if _, err := os.Stat(target); err == nil {
			return target, nil
		}
	}
	mapFile := filepath.Join("/proc", pid, "map_files",
		fmt.Sprintf("%x-%x", mapping.start, mapping.end))
	if _, err := os.Stat(mapFile); err == nil {
		return mapFile, nil
	}
	return "", fmt.Errorf("mapped file is inaccessible, file=%q", mapping.path)
}

func fileIdentity(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("unexpected file status type, file=%q actual_type=%T",
			path, info.Sys())
	}
	return fmt.Sprintf("%x:%x", uint64(stat.Dev), uint64(stat.Ino)), nil
}

func (a *attacher) close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, links := range a.bootstrapLinks {
		for _, uprobe := range links {
			_ = uprobe.Close()
		}
	}
	for _, uprobe := range a.dataLinks {
		_ = uprobe.Close()
	}
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
