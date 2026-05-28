// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"context"
	"sync"
	"testing"
	"time"
)

type memoryWriter struct {
	mu      sync.Mutex
	records []FlowRecord
}

func (w *memoryWriter) Write(_ context.Context, records []FlowRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.records = append(w.records, records...)
	return nil
}

func TestWriteLoopFlushesBatchAndUpdatesMetrics(t *testing.T) {
	writer := &memoryWriter{}
	metrics := &CollectorMetrics{}
	collector := &Collector{
		Writer:        writer,
		BatchSize:     2,
		FlushInterval: time.Hour,
		Metrics:       metrics,
	}
	rowsCh := make(chan FlowRecord, 2)
	rowsCh <- FlowRecord{Bytes: 1}
	rowsCh <- FlowRecord{Bytes: 2}
	close(rowsCh)
	collector.writeLoop(context.Background(), rowsCh)
	if len(writer.records) != 2 {
		t.Fatalf("written records = %d, want 2", len(writer.records))
	}
	if metrics.Snapshot().RecordsWritten != 2 {
		t.Fatalf("records written metric = %d", metrics.Snapshot().RecordsWritten)
	}
}

func TestBoolLabel(t *testing.T) {
	if boolLabel(true) != "true" || boolLabel(false) != "false" {
		t.Fatalf("boolLabel broken")
	}
}
