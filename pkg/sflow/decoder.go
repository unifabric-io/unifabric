// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"bytes"
	"encoding/binary"
	"net"
	"time"

	sflowdecoder "github.com/netsampler/goflow2/v2/decoders/sflow"
)

type Decoder struct {
	Now func() time.Time
}

func (d Decoder) DecodeDatagram(payload []byte, remote *net.UDPAddr) ([]FlowRecord, error) {
	now := time.Now()
	if d.Now != nil {
		now = d.Now()
	}

	var packet sflowdecoder.Packet
	if err := sflowdecoder.DecodeMessageVersion(bytes.NewBuffer(payload), &packet); err != nil {
		return nil, err
	}

	samplerIP := net.IP(packet.AgentIP)
	if len(samplerIP) == 0 && remote != nil {
		samplerIP = remote.IP
	}

	records := make([]FlowRecord, 0)
	for _, sample := range packet.Samples {
		switch v := sample.(type) {
		case sflowdecoder.FlowSample:
			records = append(records, recordsFromFlowRecords(now, packet.SequenceNumber, uint64(v.SamplingRate), samplerIP, v.Records)...)
		case *sflowdecoder.FlowSample:
			records = append(records, recordsFromFlowRecords(now, packet.SequenceNumber, uint64(v.SamplingRate), samplerIP, v.Records)...)
		case sflowdecoder.ExpandedFlowSample:
			records = append(records, recordsFromFlowRecords(now, packet.SequenceNumber, uint64(v.SamplingRate), samplerIP, v.Records)...)
		case *sflowdecoder.ExpandedFlowSample:
			records = append(records, recordsFromFlowRecords(now, packet.SequenceNumber, uint64(v.SamplingRate), samplerIP, v.Records)...)
		}
	}
	return records, nil
}

func recordsFromFlowRecords(now time.Time, sequenceNum uint32, samplingRate uint64, samplerIP net.IP, flowRecords []sflowdecoder.FlowRecord) []FlowRecord {
	var srcAS uint32
	var dstAS uint32
	parsed := make([]parsedFlow, 0, len(flowRecords))

	for _, record := range flowRecords {
		switch data := record.Data.(type) {
		case sflowdecoder.SampledIPv4:
			parsed = append(parsed, parsedFromSampledIP(data.SampledIPBase, etherTypeIPv4))
		case *sflowdecoder.SampledIPv4:
			parsed = append(parsed, parsedFromSampledIP(data.SampledIPBase, etherTypeIPv4))
		case sflowdecoder.SampledIPv6:
			parsed = append(parsed, parsedFromSampledIP(data.SampledIPBase, etherTypeIPv6))
		case *sflowdecoder.SampledIPv6:
			parsed = append(parsed, parsedFromSampledIP(data.SampledIPBase, etherTypeIPv6))
		case sflowdecoder.SampledHeader:
			if pf, ok := parseSampledHeader(data.HeaderData, data.FrameLength); ok {
				parsed = append(parsed, pf)
			}
		case *sflowdecoder.SampledHeader:
			if pf, ok := parseSampledHeader(data.HeaderData, data.FrameLength); ok {
				parsed = append(parsed, pf)
			}
		case sflowdecoder.ExtendedGateway:
			srcAS = data.SrcAS
			dstAS = data.AS
		case *sflowdecoder.ExtendedGateway:
			srcAS = data.SrcAS
			dstAS = data.AS
		}
	}

	rows := make([]FlowRecord, 0, len(parsed))
	for _, pf := range parsed {
		if len(pf.SrcAddr) == 0 || len(pf.DstAddr) == 0 {
			continue
		}
		bytesCount := pf.Bytes
		if bytesCount == 0 {
			bytesCount = 1
		}
		rows = append(rows, FlowRecord{
			Type:           FlowTypeSFlow5,
			TimeReceived:   now,
			SequenceNum:    sequenceNum,
			SamplingRate:   samplingRate,
			SamplerAddress: cloneIP(samplerIP),
			TimeFlowStart:  now,
			TimeFlowEnd:    now,
			Bytes:          bytesCount,
			Packets:        1,
			SrcAddr:        cloneIP(pf.SrcAddr),
			DstAddr:        cloneIP(pf.DstAddr),
			SrcAS:          srcAS,
			DstAS:          dstAS,
			Etype:          pf.Etype,
			Proto:          pf.Proto,
			SrcPort:        pf.SrcPort,
			DstPort:        pf.DstPort,
		})
	}
	return rows
}

func parsedFromSampledIP(data sflowdecoder.SampledIPBase, etype uint32) parsedFlow {
	return parsedFlow{
		Bytes:   uint64(data.Length),
		SrcAddr: net.IP(data.SrcIP),
		DstAddr: net.IP(data.DstIP),
		Etype:   etype,
		Proto:   data.Protocol,
		SrcPort: data.SrcPort,
		DstPort: data.DstPort,
	}
}

func parseSampledHeader(header []byte, frameLength uint32) (parsedFlow, bool) {
	if len(header) < 14 {
		return parsedFlow{}, false
	}

	offset := 14
	etype := uint32(binary.BigEndian.Uint16(header[12:14]))
	for i := 0; i < 2; i++ {
		if etype != 0x8100 && etype != 0x88A8 && etype != 0x9100 {
			break
		}
		if len(header) < offset+4 {
			return parsedFlow{}, false
		}
		etype = uint32(binary.BigEndian.Uint16(header[offset+2 : offset+4]))
		offset += 4
	}

	switch etype {
	case etherTypeIPv4:
		return parseIPv4(header[offset:], frameLength)
	case etherTypeIPv6:
		return parseIPv6(header[offset:], frameLength)
	default:
		return parsedFlow{}, false
	}
}

func parseIPv4(ip []byte, frameLength uint32) (parsedFlow, bool) {
	if len(ip) < 20 || ip[0]>>4 != 4 {
		return parsedFlow{}, false
	}
	ihl := int(ip[0]&0x0F) * 4
	if ihl < 20 || len(ip) < ihl {
		return parsedFlow{}, false
	}

	proto := ip[9]
	var srcPort uint32
	var dstPort uint32
	if proto == protoTCP || proto == protoUDP || proto == protoSCTP {
		if len(ip) >= ihl+4 {
			srcPort = uint32(binary.BigEndian.Uint16(ip[ihl : ihl+2]))
			dstPort = uint32(binary.BigEndian.Uint16(ip[ihl+2 : ihl+4]))
		}
	}
	bytesCount := uint64(frameLength)
	if bytesCount == 0 && len(ip) >= 4 {
		bytesCount = uint64(binary.BigEndian.Uint16(ip[2:4]))
	}
	return parsedFlow{
		Bytes:   bytesCount,
		SrcAddr: cloneIP(net.IP(ip[12:16])),
		DstAddr: cloneIP(net.IP(ip[16:20])),
		Etype:   etherTypeIPv4,
		Proto:   uint32(proto),
		SrcPort: srcPort,
		DstPort: dstPort,
	}, true
}

func parseIPv6(ip []byte, frameLength uint32) (parsedFlow, bool) {
	if len(ip) < 40 || ip[0]>>4 != 6 {
		return parsedFlow{}, false
	}

	nextHeader := ip[6]
	offset := 40
	for i := 0; i < 8; i++ {
		switch nextHeader {
		case 0, 43, 60:
			if len(ip) < offset+2 {
				return parsedFlow{}, false
			}
			hdrLen := int(ip[offset+1]+1) * 8
			if len(ip) < offset+hdrLen {
				return parsedFlow{}, false
			}
			nextHeader = ip[offset]
			offset += hdrLen
		case 44:
			if len(ip) < offset+8 {
				return parsedFlow{}, false
			}
			nextHeader = ip[offset]
			offset += 8
		case 51:
			if len(ip) < offset+2 {
				return parsedFlow{}, false
			}
			hdrLen := int(ip[offset+1]+2) * 4
			if len(ip) < offset+hdrLen {
				return parsedFlow{}, false
			}
			nextHeader = ip[offset]
			offset += hdrLen
		default:
			goto done
		}
	}

done:
	var srcPort uint32
	var dstPort uint32
	if nextHeader == protoTCP || nextHeader == protoUDP || nextHeader == protoSCTP {
		if len(ip) >= offset+4 {
			srcPort = uint32(binary.BigEndian.Uint16(ip[offset : offset+2]))
			dstPort = uint32(binary.BigEndian.Uint16(ip[offset+2 : offset+4]))
		}
	}
	bytesCount := uint64(frameLength)
	if bytesCount == 0 && len(ip) >= 6 {
		bytesCount = 40 + uint64(binary.BigEndian.Uint16(ip[4:6]))
	}
	return parsedFlow{
		Bytes:   bytesCount,
		SrcAddr: cloneIP(net.IP(ip[8:24])),
		DstAddr: cloneIP(net.IP(ip[24:40])),
		Etype:   etherTypeIPv6,
		Proto:   uint32(nextHeader),
		SrcPort: srcPort,
		DstPort: dstPort,
	}, true
}

func cloneIP(ip net.IP) net.IP {
	if ip == nil {
		return nil
	}
	out := make(net.IP, len(ip))
	copy(out, ip)
	return out
}
