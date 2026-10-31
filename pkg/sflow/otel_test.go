// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"net"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

func TestFlowLogRecordDerivedFields(t *testing.T) {
	r := FlowRecord{
		TimeFlowStart:  time.Unix(1700000000, 0),
		SamplingRate:   256,
		SamplerAddress: net.ParseIP("10.0.0.1"),
		Bytes:          1000,
		Packets:        2,
		Proto:          6,
		SrcAddr:        net.ParseIP("10.1.0.5"),
		DstAddr:        net.ParseIP("10.1.0.6"),
		SrcAttribution: EndpointAttribution{
			PodName: "a-0", PodNamespace: "ns1", NodeName: "node1",
			TopOwnerKind: "StatefulSet", TopOwnerName: "a", TopOwnerNamespace: "ns1",
		},
	}
	attrs := map[string]attribute.Value{}
	record := flowLogRecord(&r)
	record.WalkAttributes(func(kv attribute.KeyValue) bool {
		attrs[string(kv.Key)] = kv.Value
		return true
	})
	if got := attrs["scaled_bytes"].AsInt64(); got != 256000 {
		t.Fatalf("scaled_bytes = %d, want 256000", got)
	}
	if got := attrs["scaled_packets"].AsInt64(); got != 512 {
		t.Fatalf("scaled_packets = %d, want 512", got)
	}
	if got := attrs["sampler_ip"].AsString(); got != "10.0.0.1" {
		t.Fatalf("sampler_ip = %q", got)
	}
	if got := attrs["protocol"].AsString(); got != "TCP" {
		t.Fatalf("protocol = %q", got)
	}
	if got := attrs["src_workload"].AsString(); got != "ns1/StatefulSet/a" {
		t.Fatalf("src_workload = %q", got)
	}
	if got := attrs["dst_workload"].AsString(); got != "" {
		t.Fatalf("dst_workload = %q, want empty", got)
	}
	if !record.Timestamp().Equal(r.TimeFlowStart) {
		t.Fatalf("timestamp = %v", record.Timestamp())
	}
}

func TestFlowLogRecordZeroSamplingRateScalesByOne(t *testing.T) {
	r := FlowRecord{Bytes: 42, Packets: 1}
	attrs := map[string]attribute.Value{}
	record := flowLogRecord(&r)
	record.WalkAttributes(func(kv attribute.KeyValue) bool {
		attrs[string(kv.Key)] = kv.Value
		return true
	})
	if got := attrs["scaled_bytes"].AsInt64(); got != 42 {
		t.Fatalf("scaled_bytes = %d, want 42", got)
	}
}
