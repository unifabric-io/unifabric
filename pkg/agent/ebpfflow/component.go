// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

// Package ebpfflow attributes RDMA send traffic to Pod-to-Pod flows.
//
// Its ibv_modify_qp uprobe learns each user QP's destination GID and owning
// process while discovering provider callbacks. Other public libibverbs probes
// also discover callbacks from live verbs objects, then address-only uprobes
// count payload bytes on both the legacy and
// extended verbs submission paths. Learned callback file offsets are persisted
// per library content identity so restarts re-attach without new verbs objects.
// qp_infos and qp_stats are pinned under bpffs so a restarted agent keeps the
// QP destinations and byte totals learned by the previous instance.
// Every interval it joins both BPF maps and exposes "src pod -> dst pod" byte
// and WR totals as Prometheus counters on the agent metrics endpoint.
//
// Requirements: Linux with eBPF and uprobe support, privileged
// (or CAP_BPF+CAP_PERFMON+CAP_SYS_ADMIN) and hostPID when
// containerized. Provider debug symbols are not required.
// UD QPs are not covered.
package ebpfflow

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/unifabric-io/unifabric/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type qpKey struct {
	Tgid uint32
	QPN  uint32
}

// qpInfo mirrors struct qp_info in the BPF object.
type qpInfo struct {
	DGID     [16]byte
	Comm     [16]byte
	DestQPN  uint32
	DLID     uint16
	IsGlobal uint8
	PortNum  uint8
	Device   [64]byte
}

// device returns the RDMA device name recorded at RTR, empty when unknown.
func (info qpInfo) device() string {
	return cString(info.Device[:])
}

// lidAddressed reports whether the peer was given by LID only. RoCE always
// carries a GRH, InfiniBand inside one subnet usually does not.
func (info qpInfo) lidAddressed() bool {
	return info.IsGlobal == 0 && info.DLID != 0
}

type qpStat struct {
	Bytes uint64
	WRs   uint64
}

// options are the parsed settings of the component.
type options struct {
	bpfObject         string
	pinDir            string
	offsetCache       string
	discoveryInterval time.Duration
	reportInterval    time.Duration

	endpointEnabled     bool
	endpointInterval    time.Duration
	endpointMinInterval time.Duration
	endpointAllPods     bool

	sendsEnabled bool
	sends        sendOptions
	otlp         otelConfig
}

func optionsFromConfig(cfg config.EBPFFlowConfig, node string) (options, error) {
	var opts options
	var err error
	opts.bpfObject = cfg.BPFObject
	opts.pinDir = cfg.PinDir
	opts.offsetCache = cfg.OffsetCache
	if opts.discoveryInterval, err = time.ParseDuration(cfg.DiscoveryInterval); err != nil {
		return opts, fmt.Errorf("discoveryInterval: %w", err)
	}
	if opts.reportInterval, err = time.ParseDuration(cfg.ReportInterval); err != nil {
		return opts, fmt.Errorf("reportInterval: %w", err)
	}
	if opts.discoveryInterval <= 0 || opts.reportInterval <= 0 {
		return opts, errors.New("discoveryInterval and reportInterval must be greater than zero")
	}
	opts.endpointEnabled = cfg.Endpoint.Enabled != nil && *cfg.Endpoint.Enabled
	opts.endpointAllPods = cfg.Endpoint.AllPods
	if opts.endpointInterval, err = time.ParseDuration(cfg.Endpoint.Interval); err != nil {
		return opts, fmt.Errorf("endpoint.interval: %w", err)
	}
	if opts.endpointMinInterval, err = time.ParseDuration(cfg.Endpoint.MinInterval); err != nil {
		return opts, fmt.Errorf("endpoint.minInterval: %w", err)
	}
	if opts.endpointEnabled && opts.endpointInterval <= 0 {
		return opts, errors.New("endpoint.interval must be greater than zero")
	}
	opts.sends = sendOptions{
		queue:  cfg.FlowEvents.Queue,
		batch:  cfg.FlowEvents.Batch,
		sample: cfg.FlowEvents.Sample,
	}
	opts.sendsEnabled = cfg.FlowEvents.Enabled == nil || *cfg.FlowEvents.Enabled
	if opts.sends.flushIn, err = time.ParseDuration(cfg.FlowEvents.Flush); err != nil {
		return opts, fmt.Errorf("flowEvents.flush: %w", err)
	}
	if opts.sends.aggregate, err = time.ParseDuration(cfg.FlowEvents.Aggregate); err != nil {
		return opts, fmt.Errorf("flowEvents.aggregate: %w", err)
	}
	opts.otlp = otelConfig{
		endpoint: cfg.OTLP.Endpoint,
		insecure: cfg.OTLP.Insecure == nil || *cfg.OTLP.Insecure,
		node:     node,
	}
	if opts.otlp.timeout, err = time.ParseDuration(cfg.OTLP.Timeout); err != nil {
		return opts, fmt.Errorf("otlp.timeout: %w", err)
	}
	return opts, nil
}

// Component is the eBPF flow attribution component of the agent. It runs as
// a controller-runtime Runnable and owns the BPF collection, the uprobe
// attacher, the resolver, the RDMAEndpoint publisher, the metrics and the
// send pipeline for the lifetime of the manager.
type Component struct {
	opts      options
	node      string
	client    client.Client
	apiReader client.Reader
	podReader client.Reader
	metrics   *flowMetrics
}

// New validates the config and builds the component. client is the manager
// client used for RDMAEndpoint reads and Server-Side Apply writes, apiReader
// fetches owner objects without caching them, podReader lists Pods of the
// whole cluster for peer resolution and registerer receives the metrics.
func New(cfg config.EBPFFlowConfig, node string, log *slog.Logger, c client.Client,
	apiReader, podReader client.Reader, registerer prometheus.Registerer) (*Component, error) {
	opts, err := optionsFromConfig(cfg, node)
	if err != nil {
		return nil, fmt.Errorf("ebpfFlow config: %w", err)
	}
	if node == "" {
		return nil, errors.New("node name is required")
	}
	if log != nil {
		logger = log
	}
	return &Component{
		opts:      opts,
		node:      node,
		client:    c,
		apiReader: apiReader,
		podReader: podReader,
		metrics:   newFlowMetrics(registerer),
	}, nil
}

// NeedLeaderElection reports false, every node runs its own instance.
func (c *Component) NeedLeaderElection() bool {
	return false
}

// Start loads the BPF collection, attaches probes and runs the main loop
// until ctx is cancelled. Errors before the loop are returned so the manager
// fails fast, errors inside the loop terminate the component.
func (c *Component) Start(ctx context.Context) error {
	opts := c.opts
	if err := rlimit.RemoveMemlock(); err != nil {
		return fmt.Errorf("remove memlock limit: %w", err)
	}

	pinDir, err := preparePinDir(opts.pinDir)
	if err != nil {
		logger.Warn("prepare_pin_dir", "dir", opts.pinDir, "status", "pinning_disabled", "reason", err)
		pinDir = ""
	}

	coll, err := loadCollection(opts.bpfObject, pinDir)
	if err != nil {
		return fmt.Errorf("load bpf collection %q: %w", opts.bpfObject, err)
	}
	defer coll.Close()
	if err := setBPFSendEvents(coll.Maps["flow_config"], opts.sendsEnabled); err != nil {
		return fmt.Errorf("configure send events: %w", err)
	}
	logger.Debug("bpf_collection_loaded", "object", opts.bpfObject,
		"program_count", len(coll.Programs), "map_count", len(coll.Maps))

	execTracepoint, err := link.Tracepoint("sched", "sched_process_exec", coll.Programs["handle_process_exec"], nil)
	if err != nil {
		logger.Warn("attach_process_exec_tracepoint", "status", "failed", "fallback", "periodic_scan", "reason", err)
	} else {
		defer execTracepoint.Close()
		logger.Debug("process_exec_tracepoint_attached", "category", "sched", "name", "sched_process_exec")
	}

	offsets, err := newOffsetCache(opts.offsetCache)
	if err != nil {
		logger.Warn("load_callback_offset_cache", "file", opts.offsetCache,
			"status", "starting_with_empty_cache", "reason", err)
	} else {
		logger.Debug("callback_offset_cache_loaded", "file", opts.offsetCache,
			"library_count", offsets.libraryCount())
	}

	att := newAttacher(coll.Programs, offsets)
	defer att.close()
	callbackReader, err := ringbuf.NewReader(coll.Maps["callback_events"])
	if err != nil {
		return fmt.Errorf("open callback_events ring buffer: %w", err)
	}
	defer callbackReader.Close()
	logger.Debug("callback_event_queue_opened", "map", "callback_events")
	// qpChanges carries at most one pending notification, extra RTR events
	// are coalesced into it.
	qpChanges := make(chan struct{}, 1)
	callbackDone := make(chan error, 1)
	go func() {
		callbackDone <- consumeCallbackEvents(callbackReader, att, qpChanges)
	}()
	res := newResolver(ctx, c.client, c.apiReader, c.podReader)
	metrics := c.metrics

	var sends *sendPipeline
	var sendReader *ringbuf.Reader
	var sendDone <-chan error
	var flushDone <-chan struct{}
	if opts.sendsEnabled {
		var factories []sinkFactory
		if opts.otlp.endpoint != "" {
			factories = append(factories, otelFactory(opts.otlp, opts.sends))
		}
		for _, factory := range factories {
			logger.Info("sink_configured", "sink", factory.kind, "target", factory.describe)
		}
		sends = newSendPipeline(c.node, metrics, factories, opts.sends)
		openedReader, openErr := ringbuf.NewReader(coll.Maps["send_events"])
		if openErr != nil {
			return fmt.Errorf("open send_events ring buffer: %w", openErr)
		}
		sendReader = openedReader
		defer sendReader.Close()
		sendDoneCh := make(chan error, 1)
		sendDone = sendDoneCh
		go func() {
			sendDoneCh <- sends.consume(sendReader)
		}()
		flushDoneCh := make(chan struct{})
		flushDone = flushDoneCh
		go func() {
			sends.runSinks(context.Background())
			close(flushDoneCh)
		}()
		logger.Debug("send_event_queue_opened", "map", "send_events", "queue", opts.sends.queue,
			"batch", opts.sends.batch, "flush", opts.sends.flushIn,
			"sample", opts.sends.sample, "aggregate", opts.sends.aggregate)
	} else {
		logger.Info("send_events_disabled", "mode", "cumulative_only")
	}
	var publisher *endpointPublisher
	// Nil channels never fire, which keeps the select below uniform when
	// publishing is disabled.
	var endpointC <-chan time.Time
	var endpointDue <-chan time.Time
	if opts.endpointEnabled {
		publisher = newEndpointPublisher(res, c.node, opts.endpointMinInterval, opts.endpointAllPods)
		endpointTick := time.NewTicker(opts.endpointInterval)
		defer endpointTick.Stop()
		endpointC = endpointTick.C
	}
	// requestEndpointUpdate publishes now when allowed, otherwise arms a
	// single timer for the earliest allowed moment so bursts of RTR events
	// collapse into one update.
	requestEndpointUpdate := func() {
		if publisher == nil || endpointDue != nil {
			return
		}
		wait := publisher.delayUntilAllowed(time.Now())
		if wait == 0 {
			publisher.run(res, coll.Maps["qp_infos"])
			return
		}
		endpointDue = time.After(wait)
	}

	discoveryTick := time.NewTicker(opts.discoveryInterval)
	defer discoveryTick.Stop()
	reportTick := time.NewTicker(opts.reportInterval)
	defer reportTick.Stop()

	att.scan()
	res.refresh()
	publisher.run(res, coll.Maps["qp_infos"])
	report(coll.Maps["qp_stats"], coll.Maps["qp_infos"], res, metrics, sends)
	logger.Info("startup_complete", "component", "ebpf-flow",
		"discovery_interval", opts.discoveryInterval, "report_interval", opts.reportInterval,
		"send_events_enabled", opts.sendsEnabled,
		"endpoint_enabled", opts.endpointEnabled, "endpoint_interval", opts.endpointInterval,
		"endpoint_min_interval", opts.endpointMinInterval)
	shutdown := func() {
		_ = callbackReader.Close()
		if sendReader != nil {
			_ = sendReader.Close()
		}
		if err := <-callbackDone; err != nil {
			logger.Error("read_callback_event", "reason", err)
		}
		if sends != nil {
			if err := <-sendDone; err != nil {
				logger.Error("read_send_event", "reason", err)
			}
			sends.close()
			<-flushDone
		}
	}
	for {
		select {
		case <-ctx.Done():
			shutdown()
			return nil
		case err := <-callbackDone:
			if sendReader != nil {
				_ = sendReader.Close()
				<-sendDone
				sends.close()
				<-flushDone
			}
			if err != nil {
				return fmt.Errorf("read callback event: %w", err)
			}
			return errors.New("callback event reader stopped")
		case err := <-sendDone:
			_ = callbackReader.Close()
			<-callbackDone
			sends.close()
			<-flushDone
			if err != nil {
				return fmt.Errorf("read send event: %w", err)
			}
			return errors.New("send event reader stopped")
		case <-discoveryTick.C:
			att.scan()
		case <-qpChanges:
			requestEndpointUpdate()
		case <-endpointDue:
			endpointDue = nil
			publisher.run(res, coll.Maps["qp_infos"])
		case <-endpointC:
			publisher.run(res, coll.Maps["qp_infos"])
		case <-reportTick.C:
			res.refresh()
			removed := report(coll.Maps["qp_stats"], coll.Maps["qp_infos"], res, metrics, sends)
			if removed {
				requestEndpointUpdate()
			}
		}
	}
}

type bpfFlowConfig struct {
	EmitSendEvents uint32
}

func setBPFSendEvents(configMap *ebpf.Map, enabled bool) error {
	if configMap == nil {
		return errors.New("flow_config map is missing")
	}
	var config bpfFlowConfig
	if enabled {
		config.EmitSendEvents = 1
	}
	return configMap.Update(uint32(0), config, ebpf.UpdateAny)
}

// consumeCallbackEvents dispatches ring buffer events. QP RTR events only
// nudge qpChanges, dropping the nudge when one is already pending, and the
// main loop turns the nudge into an RDMAEndpoint update.
func consumeCallbackEvents(reader *ringbuf.Reader, att *attacher, qpChanges chan<- struct{}) error {
	for {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, os.ErrClosed) {
				return nil
			}
			return err
		}
		event, err := decodeCallbackEvent(record.RawSample)
		if err != nil {
			logger.Error("decode_callback_event", "reason", err)
			continue
		}
		if event.Kind == discoveryProcessExec {
			att.scanTgid(event.Tgid)
			continue
		}
		if event.Kind == eventQPRTR {
			select {
			case qpChanges <- struct{}{}:
			default:
			}
			continue
		}
		att.handleCallback(event)
	}
}

func decodeCallbackEvent(data []byte) (callbackEvent, error) {
	if len(data) < 16 {
		return callbackEvent{}, fmt.Errorf("event data is too short, actual_bytes=%d required_bytes=16", len(data))
	}
	return callbackEvent{
		Tgid:    binary.LittleEndian.Uint32(data[0:4]),
		Kind:    binary.LittleEndian.Uint32(data[4:8]),
		Address: binary.LittleEndian.Uint64(data[8:16]),
	}, nil
}

func loadCollection(path, pinDir string) (*ebpf.Collection, error) {
	spec, opts, err := loadSpec(path, pinDir)
	if err != nil {
		return nil, err
	}
	if pinDir == "" {
		disabled := disableMapPinning(spec)
		logger.Debug("map_pinning_disabled", "maps", strings.Join(disabled, ","))
		return ebpf.NewCollectionWithOptions(spec, opts)
	}

	reused := existingPins(spec, pinDir)
	coll, err := ebpf.NewCollectionWithOptions(spec, opts)
	if err == nil {
		logger.Debug("maps_pinned",
			"dir", pinDir, "reused", strings.Join(reused, ","),
			"all", strings.Join(pinnedMapNames(spec), ","))
		return coll, nil
	}
	if !errors.Is(err, ebpf.ErrMapIncompatible) {
		return nil, err
	}

	// A layout change between versions leaves incompatible pins behind.
	// Drop them and start fresh rather than refusing to run.
	removed, rmErr := removePins(spec, pinDir)
	if rmErr != nil {
		return nil, fmt.Errorf("pinned maps are incompatible and cleanup failed: %w (reason=%v)",
			rmErr, err)
	}
	logger.Warn("reuse_pinned_maps", "status", "layout_incompatible", "removed", strings.Join(removed, ","), "reason", err)
	spec, opts, err = loadSpec(path, pinDir)
	if err != nil {
		return nil, err
	}
	coll, err = ebpf.NewCollectionWithOptions(spec, opts)
	if err != nil {
		return nil, err
	}
	logger.Debug("maps_pinned", "dir", pinDir, "reused", "", "all", strings.Join(pinnedMapNames(spec), ","))
	return coll, nil
}

func loadSpec(path, pinDir string) (*ebpf.CollectionSpec, ebpf.CollectionOptions, error) {
	var opts ebpf.CollectionOptions
	spec, err := ebpf.LoadCollectionSpec(path)
	if err != nil {
		return nil, opts, err
	}
	opts.Maps.PinPath = pinDir
	return spec, opts, nil
}

// logger is the component logger, replaced by New. Free functions in this
// package log through it because the component has a single instance per
// process.
var logger = slog.Default()

// periodic summary logs are printed only when their content changes.
var (
	dedupLogMu   sync.Mutex
	dedupLogLast = make(map[string]string)
)

// logDedup logs msg with attrs at level unless the previous log under the
// same topic had identical content.
func logDedup(topic string, level slog.Level, msg string, attrs ...any) {
	fingerprint := msg + fmt.Sprint(attrs...)
	dedupLogMu.Lock()
	repeated := dedupLogLast[topic] == fingerprint
	if !repeated {
		dedupLogLast[topic] = fingerprint
	}
	dedupLogMu.Unlock()
	if !repeated {
		logger.Log(context.Background(), level, msg, attrs...)
	}
}

type qpInfoIndex struct {
	exact     map[qpKey]qpInfo
	byQPN     map[uint32]qpInfo
	ambiguous map[uint32]bool
}

func newQPInfoIndex() *qpInfoIndex {
	return &qpInfoIndex{
		exact:     make(map[qpKey]qpInfo),
		byQPN:     make(map[uint32]qpInfo),
		ambiguous: make(map[uint32]bool),
	}
}

func (index *qpInfoIndex) add(key qpKey, info qpInfo) {
	index.exact[key] = info
	if _, exists := index.byQPN[key.QPN]; exists {
		index.ambiguous[key.QPN] = true
		return
	}
	index.byQPN[key.QPN] = info
}

func (index *qpInfoIndex) lookup(key qpKey) (qpInfo, bool) {
	if info, ok := index.exact[key]; ok {
		return info, true
	}
	if index.ambiguous[key.QPN] {
		return qpInfo{}, false
	}
	info, ok := index.byQPN[key.QPN]
	return info, ok
}

func tgidAlive(tgid uint32) bool {
	_, err := os.Stat(filepath.Join("/proc", strconv.FormatUint(uint64(tgid), 10)))
	return err == nil
}

// cleanupDeadQPs drops map entries owned by exited processes and reports
// whether any qp_infos entry was removed. It runs after reporting so a
// short-lived flow's final totals are printed at least once.
// Info entries are kept while a live stats entry still references their QPN
// (parent-created QP, child posting).
func cleanupDeadQPs(stats, infos *ebpf.Map) bool {
	liveQPNs := make(map[uint32]bool)
	var deadStats []qpKey
	var key qpKey
	var st qpStat
	it := stats.Iterate()
	for it.Next(&key, &st) {
		if tgidAlive(key.Tgid) {
			liveQPNs[key.QPN] = true
		} else {
			deadStats = append(deadStats, key)
		}
	}
	if err := it.Err(); err != nil {
		logDedup("cleanup_iterate_qp_stats_error", slog.LevelError, "iterate_qp_stats", "purpose", "cleanup", "reason", err)
		return false
	}
	var info qpInfo
	var deadInfos []qpKey
	infoIt := infos.Iterate()
	for infoIt.Next(&key, &info) {
		if !tgidAlive(key.Tgid) && !liveQPNs[key.QPN] {
			deadInfos = append(deadInfos, key)
		}
	}
	if err := infoIt.Err(); err != nil {
		logDedup("cleanup_iterate_qp_info_error", slog.LevelError, "iterate_qp_info", "purpose", "cleanup", "reason", err)
	}
	for i := range deadStats {
		_ = stats.Delete(&deadStats[i])
	}
	for i := range deadInfos {
		_ = infos.Delete(&deadInfos[i])
	}
	if len(deadStats)+len(deadInfos) > 0 {
		logger.Info("cleanup_complete",
			"reason", "process_exited", "qp_stats_count", len(deadStats), "qp_info_count", len(deadInfos))
	}
	return len(deadInfos) > 0
}

// report joins the maps into metrics, publishes the per QP attribution for
// the send pipeline and returns whether cleanup removed qp_infos entries,
// which changes this node's RDMAEndpoints.
func report(stats, infos *ebpf.Map, res *resolver, metrics *flowMetrics, sends *sendPipeline) bool {
	flows := map[[2]string]bool{}
	seen := map[qpKey]bool{}
	attribution := map[qpKey]qpAttribution{}
	metrics.beginReport()
	infoIndex := newQPInfoIndex()
	infoCount := 0
	var infoKey qpKey
	var indexedInfo qpInfo
	infoIterator := infos.Iterate()
	for infoIterator.Next(&infoKey, &indexedInfo) {
		infoCount++
		infoIndex.add(infoKey, indexedInfo)
		logDedup(fmt.Sprintf("qp_info:%d:%d", infoKey.Tgid, infoKey.QPN), slog.LevelDebug, "qp_info",
			"process", infoKey.Tgid, "qp", infoKey.QPN, "process_name", comm(indexedInfo.Comm),
			"device", indexedInfo.device(), "port", indexedInfo.PortNum,
			"destination_gid", gidToIP(indexedInfo.DGID), "destination_lid", indexedInfo.DLID,
			"is_global", indexedInfo.IsGlobal, "destination_qpn", indexedInfo.DestQPN)
	}
	if err := infoIterator.Err(); err != nil {
		logDedup("iterate_qp_info_error", slog.LevelError, "iterate_qp_info", "reason", err)
	}

	var key qpKey
	var st qpStat
	summary := reportSummary{infos: infoCount}
	it := stats.Iterate()
	for it.Next(&key, &st) {
		summary.stats++
		commName := ""
		dst := peer{}
		dgid := ""
		var destQPN uint32
		var ends flowEnds
		infoFound := false
		if info, ok := infoIndex.lookup(key); ok {
			infoFound = true
			summary.matched++
			commName = comm(info.Comm)
			dgid = gidToIP(info.DGID)
			destQPN = info.DestQPN
			ends.srcDevice = info.device()
			switch {
			case info.lidAddressed():
				// InfiniBand inside one subnet, the GRH is absent.
				dst, ends.dstDevice = res.podForLID(ends.srcDevice, info.PortNum, info.DLID, destQPN)
				dgid = fmt.Sprintf("lid:%d", info.DLID)
			case dgid != "":
				dst, ends.dstDevice = res.podForPeer(dgid, destQPN)
			}
		} else {
			summary.unmatched++
		}
		src := res.podForTgid(key.Tgid, commName)
		infoStatus := "missing"
		if infoFound {
			infoStatus = "found"
		}
		logDedup(fmt.Sprintf("qp_stats:%d:%d", key.Tgid, key.QPN), slog.LevelDebug, "qp_stats",
			"process", key.Tgid, "qp", key.QPN, "wr_count", st.WRs, "byte_count", st.Bytes,
			"size", humanBytes(st.Bytes), "control_info", infoStatus, "process_name", commName,
			"device", ends.srcDevice, "destination_gid", dgid, "destination_qpn", destQPN,
			"destination_device", ends.dstDevice, "source", src.String(), "destination", dst.String())
		flows[[2]string{src.String(), dst.String()}] = true
		seen[key] = true
		attribution[key] = qpAttribution{src: src, dst: dst, ends: ends}
		if !metrics.observe(key, st, src, dst, ends) {
			summary.unresolved++
		}
	}
	// QPs that never posted yet have no qp_stats entry but may already be
	// sending. Their attribution comes from qp_infos alone so the first
	// events are not left unlabeled until the next round.
	for infoKey, info := range infoIndex.exact {
		_, done := attribution[infoKey]
		if done {
			continue
		}
		ends := flowEnds{srcDevice: info.device()}
		dst := peer{}
		switch {
		case info.lidAddressed():
			dst, ends.dstDevice = res.podForLID(ends.srcDevice, info.PortNum, info.DLID, info.DestQPN)
		case gidToIP(info.DGID) != "":
			dst, ends.dstDevice = res.podForPeer(gidToIP(info.DGID), info.DestQPN)
		}
		attribution[infoKey] = qpAttribution{
			src:  res.podForTgid(infoKey.Tgid, comm(info.Comm)),
			dst:  dst,
			ends: ends,
		}
	}
	if sends != nil {
		sends.publish(attribution)
		sends.syncGauges()
	}
	if err := it.Err(); err != nil {
		logDedup("iterate_qp_stats_error", slog.LevelError, "iterate_qp_stats", "reason", err)
	}
	summary.flows = len(flows)
	logDedup("report_summary", slog.LevelDebug, "report_summary",
		"qp_info_count", summary.infos, "qp_stats_count", summary.stats, "matched_count", summary.matched,
		"unmatched_count", summary.unmatched, "unresolved_count", summary.unresolved,
		"flow_count", summary.flows)
	metrics.endReport(seen, summary)
	return cleanupDeadQPs(stats, infos)
}

func humanBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func comm(b [16]byte) string {
	return cString(b[:])
}

// cString returns the bytes before the first NUL as a string.
func cString(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func gidToIP(gid [16]byte) string {
	var zero [16]byte
	if gid == zero {
		return ""
	}
	v4 := true
	for _, b := range gid[:10] {
		if b != 0 {
			v4 = false
			break
		}
	}
	if v4 && gid[10] == 0xff && gid[11] == 0xff {
		return net.IP(gid[12:16]).String()
	}
	return net.IP(gid[:]).String()
}
