// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func encodeSendEvent(event sendEvent) []byte {
	buf := make([]byte, sendEventSize)
	binary.LittleEndian.PutUint64(buf[0:8], event.TsNs)
	binary.LittleEndian.PutUint32(buf[8:12], event.Tgid)
	binary.LittleEndian.PutUint32(buf[12:16], event.QPN)
	binary.LittleEndian.PutUint64(buf[16:24], event.Bytes)
	binary.LittleEndian.PutUint32(buf[24:28], event.WRs)
	binary.LittleEndian.PutUint32(buf[28:32], event.Kind)
	return buf
}

func TestDecodeSendEventRoundTrip(t *testing.T) {
	want := sendEvent{TsNs: 123456789, Tgid: 4242, QPN: 15246, Bytes: 1 << 20, WRs: 3, Kind: callbackPostSend}
	got, err := decodeSendEvent(encodeSendEvent(want))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("decoded %+v, want %+v", got, want)
	}
	_, err = decodeSendEvent(make([]byte, sendEventSize-1))
	if err == nil {
		t.Fatal("short buffer must fail")
	}
}

func TestSendBucketsArePowersOfTwoUpToOneGiB(t *testing.T) {
	buckets := sendBuckets()
	if len(buckets) != 25 || buckets[0] != 64 || buckets[len(buckets)-1] != 1<<30 {
		t.Fatalf("buckets = %v", buckets)
	}
	for i := range buckets[1:] {
		if buckets[i+1] != buckets[i]*2 {
			t.Fatalf("bucket %d = %v is not double of %v", i+1, buckets[i+1], buckets[i])
		}
	}
}

func TestObserveSendCountsAndBucketsUnresolvedToo(t *testing.T) {
	m := newFlowMetrics(prometheus.NewRegistry())
	m.observeSend(testSrc, testDst, testEnds, 4096)
	m.observeSend(testSrc, testDst, testEnds, 8192)
	m.observeSend(peer{}, peer{}, flowEnds{}, 64)

	labels := flowLabels(testSrc, testDst, testEnds)
	expectValue(t, "sends for resolved flow", m.sends.WithLabelValues(labels...), 2)
	empty := flowLabels(peer{}, peer{}, flowEnds{})
	expectValue(t, "sends for unresolved flow", m.sends.WithLabelValues(empty...), 1)
	series := testutil.CollectAndCount(m.sendSize)
	if series != 2 {
		t.Fatalf("histogram series = %d, want 2", series)
	}
}

func TestSendPipelineAttributionSnapshot(t *testing.T) {
	m := newFlowMetrics(prometheus.NewRegistry())
	p := newSendPipeline("node-a", m, nil, sendOptions{queue: 8, batch: 2})
	key := qpKey{Tgid: 100, QPN: 42}
	_, ok := p.attribution.Load().byKey[key]
	if ok {
		t.Fatal("fresh pipeline must have no attribution")
	}
	p.publish(map[qpKey]qpAttribution{key: {src: testSrc, dst: testDst, ends: testEnds}})
	attr := p.attribution.Load().byKey[key]
	if attr.src != testSrc || attr.dst != testDst || attr.ends != testEnds {
		t.Fatalf("attribution = %+v", attr)
	}
	var disabled *sendPipeline
	disabled.publish(nil)
	disabled.syncGauges()
	disabled.close()
}

// testFactory is a sink factory that never opens, enough to give the
// pipeline a worker with a queue to inspect.
func testFactory(kind string) sinkFactory {
	return sinkFactory{kind: kind, open: func(context.Context) (sendSink, error) {
		return nil, errors.New("test sink never connects")
	}}
}

func drainRows(w *sinkWorker) []sendRow {
	var rows []sendRow
	for {
		select {
		case row := <-w.rows:
			rows = append(rows, row)
		default:
			return rows
		}
	}
}

func TestAggregateMergesIdenticalSubmissionsPerWindow(t *testing.T) {
	m := newFlowMetrics(prometheus.NewRegistry())
	p := newSendPipeline("node-a", m, []sinkFactory{testFactory("test")}, sendOptions{queue: 16, batch: 4, aggregate: 100 * time.Millisecond})
	w := p.sinks[0]
	base := time.Unix(1000, 0)
	attr := qpAttribution{src: testSrc, dst: testDst, ends: testEnds}
	small := sendEvent{Tgid: 1, QPN: 7, Bytes: 4096, WRs: 1, Kind: callbackPostSend}
	large := sendEvent{Tgid: 1, QPN: 7, Bytes: 1 << 20, WRs: 1, Kind: callbackPostSend}

	p.aggregate(base.Add(10*time.Millisecond), small, attr)
	p.aggregate(base.Add(20*time.Millisecond), small, attr)
	p.aggregate(base.Add(30*time.Millisecond), large, attr)
	if len(drainRows(w)) != 0 {
		t.Fatal("rows must stay in the window until it closes")
	}
	// An event in the next window flushes the previous one.
	p.aggregate(base.Add(150*time.Millisecond), small, attr)
	rows := drainRows(w)
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want 2 merged rows", rows)
	}
	for _, row := range rows {
		if !row.ts.Equal(base) {
			t.Fatalf("row ts = %v, want window start %v", row.ts, base)
		}
		if row.bytes == 4096 && row.count != 2 {
			t.Fatalf("small row count = %d, want 2", row.count)
		}
		if row.bytes == 1<<20 && row.count != 1 {
			t.Fatalf("large row count = %d, want 1", row.count)
		}
		if row.src != testSrc || row.dst != testDst {
			t.Fatalf("row attribution = %+v", row)
		}
	}
	p.flushWindow()
	rest := drainRows(w)
	if len(rest) != 1 || rest[0].count != 1 || !rest[0].ts.Equal(base.Add(100*time.Millisecond)) {
		t.Fatalf("second window = %+v", rest)
	}
}

func TestEnqueueFansOutAndDropsPerSink(t *testing.T) {
	m := newFlowMetrics(prometheus.NewRegistry())
	p := newSendPipeline("node-a", m, []sinkFactory{testFactory("a"), testFactory("b")},
		sendOptions{queue: 2, batch: 2})
	// Fill sink b so its drops do not affect sink a.
	for range 3 {
		p.sinks[1].enqueue(sendRow{count: 9})
	}
	for range 5 {
		p.enqueue(sendRow{count: 1})
	}
	a, b := p.sinks[0], p.sinks[1]
	if a.dropped.Load() != 3 || len(a.rows) != 2 {
		t.Fatalf("sink a dropped = %d queued = %d, want 3 and 2", a.dropped.Load(), len(a.rows))
	}
	if b.dropped.Load() != 6 || len(b.rows) != 2 {
		t.Fatalf("sink b dropped = %d queued = %d, want 6 and 2", b.dropped.Load(), len(b.rows))
	}
	p.syncGauges()
	expectValue(t, "dropped gauge a", m.sinkDropped.WithLabelValues("a"), 3)
	expectValue(t, "dropped gauge b", m.sinkDropped.WithLabelValues("b"), 6)
	expectValue(t, "queue gauge a", m.sinkQueue.WithLabelValues("a"), 2)
	if p.storing() != true {
		t.Fatal("pipeline with sinks must report storing")
	}
	none := newSendPipeline("node-a", m, nil, sendOptions{queue: 1, batch: 1})
	if none.storing() {
		t.Fatal("pipeline without sinks must not report storing")
	}
}

func TestSendOptionsSampleDefaultsToOne(t *testing.T) {
	p := newSendPipeline("node-a", newFlowMetrics(prometheus.NewRegistry()), nil, sendOptions{queue: 1, batch: 1, sample: 0})
	if p.opts.sample != 1 {
		t.Fatalf("sample = %d, want 1", p.opts.sample)
	}
}
