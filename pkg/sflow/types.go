// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"net"
	"time"
)

const FlowTypeSFlow5 = int32(1)

const (
	etherTypeIPv4 = uint32(0x0800)
	etherTypeIPv6 = uint32(0x86DD)

	protoTCP  = uint8(6)
	protoUDP  = uint8(17)
	protoSCTP = uint8(132)
)

type EndpointAttribution struct {
	PodName           string
	PodNamespace      string
	NodeName          string
	TopOwnerKind      string
	TopOwnerName      string
	TopOwnerNamespace string
}

type OwnerRef struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
}

type PodInfo struct {
	Name      string
	Namespace string
	NodeName  string
	IPs       []net.IP
	TopOwner  *OwnerRef
}

type FlowRecord struct {
	Type           int32
	TimeReceived   time.Time
	SequenceNum    uint32
	SamplingRate   uint64
	SamplerAddress net.IP
	TimeFlowStart  time.Time
	TimeFlowEnd    time.Time
	Bytes          uint64
	Packets        uint64
	SrcAddr        net.IP
	DstAddr        net.IP
	SrcAS          uint32
	DstAS          uint32
	Etype          uint32
	Proto          uint32
	SrcPort        uint32
	DstPort        uint32
	SrcAttribution EndpointAttribution
	DstAttribution EndpointAttribution
}

type parsedFlow struct {
	Bytes   uint64
	SrcAddr net.IP
	DstAddr net.IP
	Etype   uint32
	Proto   uint32
	SrcPort uint32
	DstPort uint32
}

type StatsSnapshot struct {
	DatagramsAccepted uint64
	DecodeErrors      uint64
	RecordsDecoded    uint64
	RecordsDropped    uint64
	RecordsWritten    uint64
	WriteErrors       uint64
}

func Fixed16(ip net.IP) [16]byte {
	var out [16]byte
	if ip == nil {
		return out
	}
	if v4 := ip.To4(); v4 != nil {
		copy(out[12:], v4)
		return out
	}
	if v16 := ip.To16(); v16 != nil {
		copy(out[:], v16)
	}
	return out
}

func Fixed16String(ip net.IP) string {
	fixed := Fixed16(ip)
	return string(fixed[:])
}
