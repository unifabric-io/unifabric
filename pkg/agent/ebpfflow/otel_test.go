// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
)

func TestSendRecordCarriesFlatTypedAttributes(t *testing.T) {
	ts := time.Unix(1700000000, 123456789)
	row := sendRow{
		ts: ts, node: "node-a", tgid: 4242, qpn: 15246, bytes: 16384, wrs: 1, count: 3,
		kind: callbackPostSend, srcDevice: "mlx5_0", dstDevice: "mlx5_2",
		src: testSrc, dst: testDst,
	}
	record := sendRecord(&row)
	if !record.Timestamp().Equal(ts) {
		t.Fatalf("timestamp = %v, want %v", record.Timestamp(), ts)
	}
	if record.Severity() != otellog.SeverityInfo || record.EventName() != "rdma.send" {
		t.Fatalf("severity/event = %v/%q", record.Severity(), record.EventName())
	}
	got := map[attribute.Key]attribute.Value{}
	record.WalkAttributes(func(kv attribute.KeyValue) bool {
		got[kv.Key] = kv.Value
		return true
	})
	wantInt := map[attribute.Key]int64{"tgid": 4242, "qpn": 15246, "bytes": 16384, "wrs": 1, "count": 3}
	for key, want := range wantInt {
		v, ok := got[key]
		if !ok || v.Type() != attribute.INT64 || v.AsInt64() != want {
			t.Fatalf("attribute %s = %v, want int64 %d", key, v, want)
		}
	}
	wantStr := map[attribute.Key]string{
		"node": "node-a", "kind": "post_send", "src_device": "mlx5_0", "dst_device": "mlx5_2",
		"src_pod_namespace": "tenant-a", "src_pod_name": "vllm-decode-0",
		"src_pod_top_owner_kind": "StatefulSet", "src_pod_top_owner_name": "vllm-decode",
		"dst_pod_namespace": "tenant-b", "dst_pod_name": "receiver-7f9c",
		"dst_pod_top_owner_kind": "Deployment", "dst_pod_top_owner_name": "receiver",
	}
	for key, want := range wantStr {
		v, ok := got[key]
		if !ok || v.Type() != attribute.STRING || v.AsString() != want {
			t.Fatalf("attribute %s = %v, want string %q", key, v, want)
		}
	}
	if len(got) != 19 {
		t.Fatalf("attribute count = %d, want 19 matching the ClickHouse columns", len(got))
	}
}
