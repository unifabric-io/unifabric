// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cilium/ebpf/ringbuf"
)

// sendEvent mirrors struct send_event in the BPF object.
type sendEvent struct {
	TsNs  uint64
	Tgid  uint32
	QPN   uint32
	Bytes uint64
	WRs   uint32
	Kind  uint32
}

const sendEventSize = 32

func decodeSendEvent(data []byte) (sendEvent, error) {
	if len(data) < sendEventSize {
		return sendEvent{}, fmt.Errorf("event data is too short, actual_bytes=%d required_bytes=%d",
			len(data), sendEventSize)
	}
	return sendEvent{
		TsNs:  binary.LittleEndian.Uint64(data[0:8]),
		Tgid:  binary.LittleEndian.Uint32(data[8:12]),
		QPN:   binary.LittleEndian.Uint32(data[12:16]),
		Bytes: binary.LittleEndian.Uint64(data[16:24]),
		WRs:   binary.LittleEndian.Uint32(data[24:28]),
		Kind:  binary.LittleEndian.Uint32(data[28:32]),
	}, nil
}

// qpAttribution is what report() learned about one QP: both Pods and both
// devices. The send consumer reads it without touching the resolver.
type qpAttribution struct {
	src  peer
	dst  peer
	ends flowEnds
}

// attributionSnapshot is an immutable map published by report() after every
// round, swapped atomically so the consumer goroutine never locks.
type attributionSnapshot struct {
	byKey map[qpKey]qpAttribution
}

// sendRow is one stored row: a single submission, or with aggregation
// enabled the count of identical submissions inside one window. The Pod
// attribution is resolved at consumption time.
type sendRow struct {
	ts        time.Time
	node      string
	tgid      uint32
	qpn       uint32
	bytes     uint64
	wrs       uint32
	count     uint32
	kind      uint32
	srcDevice string
	dstDevice string
	src       peer
	dst       peer
}

// aggregateKey identifies submissions that collapse into one row inside an
// aggregation window. Size and WR count are part of the key so the row
// still describes the exact message shape.
type aggregateKey struct {
	tgid  uint32
	qpn   uint32
	kind  uint32
	bytes uint64
	wrs   uint32
}

// sendOptions tunes how events become rows.
type sendOptions struct {
	// queue bounds the rows waiting for insert, new rows are dropped when full.
	queue int
	// batch is the rows per insert and flushIn the longest a partial batch waits.
	batch   int
	flushIn time.Duration
	// sample keeps one event in every sample for storage, 1 stores every event.
	// Metrics always see every event.
	sample uint64
	// aggregate merges identical submissions inside this window into one row
	// with a count, 0 keeps one row per submission.
	aggregate time.Duration
}

// sendSink is one storage backend for send rows. Implementations own their
// connection and must be safe to call from a single flush goroutine.
type sendSink interface {
	// name labels metrics and logs, e.g. "otlp".
	name() string
	// insert writes one batch. An error drops the batch and marks the sink
	// disconnected so the next flush reconnects.
	insert(ctx context.Context, rows []sendRow) error
	close()
}

// sinkFactory opens a sink. It is retried every sinkRetry until it succeeds.
type sinkFactory struct {
	kind string
	open func(ctx context.Context) (sendSink, error)
	// describe is logged on connect.
	describe string
}

// sinkRetry spaces reconnect attempts while a backend is unreachable.
const sinkRetry = 30 * time.Second

// sinkWorker owns one backend: its bounded queue, its flush goroutine and
// its lazily established connection. Sinks are independent, a slow or down
// backend only loses its own rows.
type sinkWorker struct {
	factory sinkFactory
	metrics *flowMetrics
	opts    sendOptions
	rows    chan sendRow
	dropped atomic.Uint64
}

func newSinkWorker(factory sinkFactory, metrics *flowMetrics, opts sendOptions) *sinkWorker {
	return &sinkWorker{
		factory: factory,
		metrics: metrics,
		opts:    opts,
		rows:    make(chan sendRow, opts.queue),
	}
}

// enqueue hands a row to the flush goroutine, dropping it when the queue is
// full so the ring buffer consumer never blocks.
func (w *sinkWorker) enqueue(row sendRow) {
	select {
	case w.rows <- row:
	default:
		w.dropped.Add(1)
	}
}

// run batches rows and writes them through the sink. It exits when rows is
// closed, flushing what remains. While no connection exists it reconnects
// every sinkRetry and drops the rows that arrive in between. Failed inserts
// drop the batch and force a reconnect.
func (w *sinkWorker) run(ctx context.Context) {
	name := w.factory.kind
	var sink sendSink
	var lastAttempt time.Time
	connect := func() {
		if sink != nil || time.Since(lastAttempt) < sinkRetry {
			return
		}
		lastAttempt = time.Now()
		dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		opened, err := w.factory.open(dialCtx)
		if err != nil {
			logDedup("sink_connect_error:"+name, slog.LevelWarn, "sink_connect",
				"sink", name, "status", fmt.Sprintf("retrying_in_%s", sinkRetry), "reason", err)
			w.metrics.sinkConnected.WithLabelValues(name).Set(0)
			return
		}
		sink = opened
		w.metrics.sinkConnected.WithLabelValues(name).Set(1)
		logger.Info("sink_connected", "sink", name, "target", w.factory.describe)
	}
	connect()
	defer func() {
		if sink != nil {
			sink.close()
		}
	}()

	ticker := time.NewTicker(w.opts.flushIn)
	defer ticker.Stop()
	pending := make([]sendRow, 0, w.opts.batch)
	flush := func() {
		if len(pending) == 0 {
			return
		}
		if sink == nil {
			connect()
		}
		if sink == nil {
			w.dropped.Add(uint64(len(pending)))
			pending = pending[:0]
			return
		}
		started := time.Now()
		err := sink.insert(ctx, pending)
		w.metrics.sinkInsertSeconds.WithLabelValues(name).Observe(time.Since(started).Seconds())
		if err != nil {
			logDedup("sink_insert_error:"+name, slog.LevelError, "sink_insert",
				"sink", name, "rows", len(pending), "reason", err)
			w.metrics.sinkInsertErrors.WithLabelValues(name).Add(float64(len(pending)))
			sink.close()
			sink = nil
			w.metrics.sinkConnected.WithLabelValues(name).Set(0)
		} else {
			w.metrics.sinkInserted.WithLabelValues(name).Add(float64(len(pending)))
		}
		pending = pending[:0]
	}
	for {
		select {
		case row, ok := <-w.rows:
			if !ok {
				flush()
				return
			}
			pending = append(pending, row)
			if len(pending) >= w.opts.batch {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// sendPipeline consumes send_events, updates the size distribution metrics
// and fans rows out to every configured sink.
type sendPipeline struct {
	node    string
	metrics *flowMetrics
	// bootNs converts the BPF monotonic timestamp into wall clock time.
	bootNs int64
	opts   sendOptions

	attribution atomic.Pointer[attributionSnapshot]

	sinks []*sinkWorker

	// window holds the rows of the open aggregation window, flushed to the
	// sinks when windowEnd passes. Only the consumer goroutine touches it.
	window    map[aggregateKey]*sendRow
	windowEnd time.Time

	received atomic.Uint64
	sampled  atomic.Uint64
}

// newSendPipeline wires the consumer. With no factories events only feed
// the metrics.
func newSendPipeline(node string, metrics *flowMetrics, factories []sinkFactory, opts sendOptions) *sendPipeline {
	if opts.sample < 1 {
		opts.sample = 1
	}
	p := &sendPipeline{
		node:    node,
		metrics: metrics,
		bootNs:  bootTimeNs(),
		opts:    opts,
		window:  map[aggregateKey]*sendRow{},
	}
	for _, factory := range factories {
		p.sinks = append(p.sinks, newSinkWorker(factory, metrics, opts))
	}
	p.attribution.Store(&attributionSnapshot{byKey: map[qpKey]qpAttribution{}})
	return p
}

// storing reports whether any sink is configured.
func (p *sendPipeline) storing() bool {
	return len(p.sinks) > 0
}

// bootTimeNs returns the wall clock time of CLOCK_MONOTONIC zero, which is
// what bpf_ktime_get_ns counts from.
func bootTimeNs() int64 {
	var ts unixTimespec
	err := clockGettime(clockMonotonic, &ts)
	if err != nil {
		return time.Now().UnixNano()
	}
	return time.Now().UnixNano() - (ts.Sec*1e9 + ts.Nsec)
}

// publish replaces the attribution snapshot after a report round.
func (p *sendPipeline) publish(byKey map[qpKey]qpAttribution) {
	if p == nil {
		return
	}
	p.attribution.Store(&attributionSnapshot{byKey: byKey})
}

// consume reads the ring buffer until the reader is closed. Every event
// updates the metrics. Storage sees one event in every opts.sample, merged
// per aggregation window when enabled. Events for QPs without attribution
// still count under empty labels so the size distribution is complete.
func (p *sendPipeline) consume(reader *ringbuf.Reader) error {
	for {
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, os.ErrClosed) {
				p.flushWindow()
				return nil
			}
			return err
		}
		event, err := decodeSendEvent(record.RawSample)
		if err != nil {
			logDedup("send_event_decode_error", slog.LevelError, "decode_send_event", "reason", err)
			continue
		}
		n := p.received.Add(1)
		key := qpKey{Tgid: event.Tgid, QPN: event.QPN}
		attr := p.attribution.Load().byKey[key]
		p.metrics.observeSend(attr.src, attr.dst, attr.ends, event.Bytes)
		if !p.storing() {
			continue
		}
		if n%p.opts.sample != 0 {
			continue
		}
		p.sampled.Add(1)
		ts := time.Unix(0, p.bootNs+int64(event.TsNs))
		if p.opts.aggregate > 0 {
			p.aggregate(ts, event, attr)
			continue
		}
		p.enqueue(sendRow{
			ts: ts, node: p.node, tgid: event.Tgid, qpn: event.QPN,
			bytes: event.Bytes, wrs: event.WRs, count: 1, kind: event.Kind,
			srcDevice: attr.ends.srcDevice, dstDevice: attr.ends.dstDevice,
			src: attr.src, dst: attr.dst,
		})
	}
}

// aggregate folds an event into the open window, flushing the window first
// when the event falls past its end. Windows are aligned to event time so
// rows carry the window start.
func (p *sendPipeline) aggregate(ts time.Time, event sendEvent, attr qpAttribution) {
	if !ts.Before(p.windowEnd) {
		p.flushWindow()
		start := ts.Truncate(p.opts.aggregate)
		p.windowEnd = start.Add(p.opts.aggregate)
	}
	key := aggregateKey{tgid: event.Tgid, qpn: event.QPN, kind: event.Kind, bytes: event.Bytes, wrs: event.WRs}
	row := p.window[key]
	if row == nil {
		row = &sendRow{
			ts: p.windowEnd.Add(-p.opts.aggregate), node: p.node, tgid: event.Tgid, qpn: event.QPN,
			bytes: event.Bytes, wrs: event.WRs, kind: event.Kind,
			srcDevice: attr.ends.srcDevice, dstDevice: attr.ends.dstDevice,
			src: attr.src, dst: attr.dst,
		}
		p.window[key] = row
	}
	row.count++
}

// flushWindow moves the aggregated rows to the insert queue.
func (p *sendPipeline) flushWindow() {
	for key, row := range p.window {
		p.enqueue(*row)
		delete(p.window, key)
	}
}

// enqueue fans a row out to every sink. Each sink drops independently when
// its queue is full so the ring buffer consumer never blocks.
func (p *sendPipeline) enqueue(row sendRow) {
	for _, sink := range p.sinks {
		sink.enqueue(row)
	}
}

// runSinks starts one flush goroutine per sink and returns when all exited.
func (p *sendPipeline) runSinks(ctx context.Context) {
	var wg sync.WaitGroup
	for _, sink := range p.sinks {
		wg.Add(1)
		go func(w *sinkWorker) {
			defer wg.Done()
			w.run(ctx)
		}(sink)
	}
	wg.Wait()
}

// syncGauges copies the pipeline counters into the agent metrics.
func (p *sendPipeline) syncGauges() {
	if p == nil {
		return
	}
	p.metrics.sendReceived.Set(float64(p.received.Load()))
	p.metrics.sendSampled.Set(float64(p.sampled.Load()))
	for _, sink := range p.sinks {
		name := sink.factory.kind
		p.metrics.sinkDropped.WithLabelValues(name).Set(float64(sink.dropped.Load()))
		p.metrics.sinkQueue.WithLabelValues(name).Set(float64(len(sink.rows)))
	}
}

// close stops accepting rows so every sink drains and exits. The consumer
// must have returned first, it flushes its window on reader close.
func (p *sendPipeline) close() {
	if p == nil {
		return
	}
	for _, sink := range p.sinks {
		close(sink.rows)
	}
}

// sendBuckets are powers of two from 64 B to 1 GiB, matching typical RDMA
// message sizes from small control messages to large tensor transfers.
func sendBuckets() []float64 {
	buckets := make([]float64, 0, 25)
	size := 64.0
	for size <= 1<<30 {
		buckets = append(buckets, size)
		size *= 2
	}
	return buckets
}
