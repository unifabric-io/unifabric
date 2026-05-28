// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	sflowdecoder "github.com/netsampler/goflow2/v2/decoders/sflow"
	"github.com/netsampler/goflow2/v2/decoders/utils"
)

func TestRecordsFromFlowRecordsIPv4IPv6AndGateway(t *testing.T) {
	now := time.Unix(10, 20)
	records := recordsFromFlowRecords(now, 7, 100, net.ParseIP("10.0.0.1"), []sflowdecoder.FlowRecord{
		{Data: sflowdecoder.ExtendedGateway{SrcAS: 64512, AS: 64513}},
		{Data: sflowdecoder.SampledIPv4{SampledIPBase: sflowdecoder.SampledIPBase{
			Length: 100, Protocol: 6, SrcIP: []byte{10, 1, 1, 1}, DstIP: []byte{10, 1, 1, 2}, SrcPort: 1234, DstPort: 443,
		}}},
		{Data: sflowdecoder.SampledIPv6{SampledIPBase: sflowdecoder.SampledIPBase{
			Length: 120, Protocol: 17, SrcIP: utils.IPAddress(net.ParseIP("2001:db8::1").To16()), DstIP: utils.IPAddress(net.ParseIP("2001:db8::2").To16()), SrcPort: 53, DstPort: 5353,
		}}},
	})
	if len(records) != 2 {
		t.Fatalf("records len = %d, want 2", len(records))
	}
	if records[0].SrcAS != 64512 || records[0].DstAS != 64513 {
		t.Fatalf("AS = %d/%d", records[0].SrcAS, records[0].DstAS)
	}
	if records[0].SrcPort != 1234 || records[0].DstPort != 443 || records[0].Etype != etherTypeIPv4 {
		t.Fatalf("ipv4 record = %#v", records[0])
	}
	if records[1].SrcPort != 53 || records[1].DstPort != 5353 || records[1].Etype != etherTypeIPv6 {
		t.Fatalf("ipv6 record = %#v", records[1])
	}
}

func TestParseSampledHeaderIPv4WithVLAN(t *testing.T) {
	header := append([]byte{
		0, 1, 2, 3, 4, 5,
		6, 7, 8, 9, 10, 11,
		0x81, 0x00,
		0x00, 0x64, 0x08, 0x00,
	}, ipv4Packet(17, 1111, 2222, net.IPv4(10, 0, 0, 1), net.IPv4(10, 0, 0, 2))...)
	flow, ok := parseSampledHeader(header, 128)
	if !ok {
		t.Fatalf("parseSampledHeader() ok = false")
	}
	if flow.Proto != 17 || flow.SrcPort != 1111 || flow.DstPort != 2222 || flow.Bytes != 128 {
		t.Fatalf("flow = %#v", flow)
	}
}

func TestParseSampledHeaderRejectsMalformed(t *testing.T) {
	if _, ok := parseSampledHeader([]byte{1, 2, 3}, 0); ok {
		t.Fatalf("parseSampledHeader() ok = true")
	}
}

func ipv4Packet(proto uint8, srcPort, dstPort uint16, src, dst net.IP) []byte {
	packet := make([]byte, 24)
	packet[0] = 0x45
	packet[2] = 0
	packet[3] = 24
	packet[9] = proto
	copy(packet[12:16], src.To4())
	copy(packet[16:20], dst.To4())
	binary.BigEndian.PutUint16(packet[20:22], srcPort)
	binary.BigEndian.PutUint16(packet[22:24], dstPort)
	return packet
}

func BenchmarkParseSampledHeaderIPv4(b *testing.B) {
	header := append([]byte{
		0, 1, 2, 3, 4, 5,
		6, 7, 8, 9, 10, 11,
		0x08, 0x00,
	}, ipv4Packet(6, 1234, 443, net.IPv4(10, 0, 0, 1), net.IPv4(10, 0, 0, 2))...)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := parseSampledHeader(header, 0); !ok {
			b.Fatal("parse failed")
		}
	}
}

func TestDecodeDatagramMalformed(t *testing.T) {
	_, err := (Decoder{}).DecodeDatagram(bytes.Repeat([]byte{0xff}, 16), &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err == nil {
		t.Fatalf("DecodeDatagram() error = nil")
	}
}
