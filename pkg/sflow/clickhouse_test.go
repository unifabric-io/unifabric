// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"net"
	"testing"
	"time"
)

func TestRowFromRecordIncludesAttributionAndFixedIPs(t *testing.T) {
	now := time.Unix(1, 2)
	row := RowFromRecord(FlowRecord{
		Type:           FlowTypeSFlow5,
		TimeReceived:   now,
		SequenceNum:    9,
		SamplingRate:   1000,
		SamplerAddress: net.ParseIP("192.0.2.1"),
		TimeFlowStart:  now,
		TimeFlowEnd:    now,
		Bytes:          100,
		Packets:        1,
		SrcAddr:        net.ParseIP("10.0.0.1"),
		DstAddr:        net.ParseIP("2001:db8::1"),
		SrcAttribution: EndpointAttribution{
			PodName:           "src",
			PodNamespace:      "ns",
			NodeName:          "node-a",
			TopOwnerKind:      "Job",
			TopOwnerName:      "train",
			TopOwnerNamespace: "ns",
		},
	})
	if len(row.SamplerAddress) != 16 || len(row.SrcAddr) != 16 || len(row.DstAddr) != 16 {
		t.Fatalf("fixed lengths = %d/%d/%d", len(row.SamplerAddress), len(row.SrcAddr), len(row.DstAddr))
	}
	if row.SrcK8sPodName != "src" || row.SrcK8sTopOwnerName != "train" {
		t.Fatalf("row attribution = %#v", row)
	}
}

func TestInsertSQLUsesTable(t *testing.T) {
	sql := insertSQL("flows_raw")
	if sql == "" || sql[:11] != "INSERT INTO" {
		t.Fatalf("insertSQL() = %q", sql)
	}
}
