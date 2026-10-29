// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

// Mock sFlow v5 sender used by E2E tests. It crafts flow samples whose
// sampled Ethernet/IPv4 headers carry the given pod IPs and sends the
// datagrams over UDP to an sFlow collector, so the full decode -> pod
// attribution -> OTLP -> Elasticsearch path can be validated without
// real switch traffic.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strings"
	"time"
)

type mockFlow struct {
	src, dst         net.IP
	srcPort, dstPort uint16
	proto            uint8
	frameLen         uint32
}

func writeBE(buf *bytes.Buffer, v any) { _ = binary.Write(buf, binary.BigEndian, v) }

// mockSampledHeader builds an Ethernet+IPv4+L4 header blob for a sampled packet.
func mockSampledHeader(f mockFlow) []byte {
	h := &bytes.Buffer{}
	h.Write([]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x01})
	h.Write([]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x02})
	writeBE(h, uint16(0x0800))

	ip := &bytes.Buffer{}
	ip.WriteByte(0x45)
	ip.WriteByte(0)
	writeBE(ip, uint16(f.frameLen-14))
	writeBE(ip, uint16(rand.Intn(65535)))
	writeBE(ip, uint16(0x4000))
	ip.WriteByte(64)
	ip.WriteByte(f.proto)
	writeBE(ip, uint16(0))
	ip.Write(f.src.To4())
	ip.Write(f.dst.To4())
	h.Write(ip.Bytes())

	writeBE(h, f.srcPort)
	writeBE(h, f.dstPort)
	h.Write(make([]byte, 16))
	return h.Bytes()
}

func buildMockDatagram(agent net.IP, seq, uptime, samplingRate uint32, flows []mockFlow) []byte {
	d := &bytes.Buffer{}
	writeBE(d, uint32(5)) // sFlow version
	writeBE(d, uint32(1)) // agent address type: IPv4
	d.Write(agent.To4())
	writeBE(d, uint32(0)) // sub agent id
	writeBE(d, seq)
	writeBE(d, uptime)
	writeBE(d, uint32(len(flows)))

	for i, f := range flows {
		hdr := mockSampledHeader(f)
		rec := &bytes.Buffer{}
		writeBE(rec, uint32(1)) // header protocol: Ethernet
		writeBE(rec, f.frameLen)
		writeBE(rec, uint32(0))
		writeBE(rec, uint32(len(hdr)))
		rec.Write(hdr)
		for rec.Len()%4 != 0 {
			rec.WriteByte(0)
		}

		sample := &bytes.Buffer{}
		writeBE(sample, seq*100+uint32(i))
		writeBE(sample, uint32(0x00000001)) // source id
		writeBE(sample, samplingRate)
		writeBE(sample, (seq+1)*samplingRate) // sample pool
		writeBE(sample, uint32(0))            // drops
		writeBE(sample, uint32(7))            // input if
		writeBE(sample, uint32(9))            // output if
		writeBE(sample, uint32(1))            // record count
		writeBE(sample, uint32(1))            // record format: sampled header
		writeBE(sample, uint32(rec.Len()))
		sample.Write(rec.Bytes())

		writeBE(d, uint32(1)) // sample type: flow sample
		writeBE(d, uint32(sample.Len()))
		d.Write(sample.Bytes())
	}
	return d.Bytes()
}

// runMockSender implements the "mock" subcommand.
func runMockSender(args []string) error {
	fs := flag.NewFlagSet("mock", flag.ExitOnError)
	target := fs.String("target", "127.0.0.1:6343", "collector UDP address")
	agent := fs.String("agent", "10.0.0.11", "sFlow agent (switch) IP")
	podList := fs.String("pods", "", "comma separated pod IPs (at least two)")
	rate := fs.Uint("rate", 1000, "sampling rate")
	interval := fs.Duration("interval", 500*time.Millisecond, "send interval")
	count := fs.Int("count", 0, "number of datagrams, 0 = forever")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var pods []net.IP
	for _, s := range strings.Split(*podList, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		ip := net.ParseIP(s).To4()
		if ip == nil {
			return fmt.Errorf("bad pod ip %q", s)
		}
		pods = append(pods, ip)
	}
	if len(pods) < 2 {
		return fmt.Errorf("-pods requires at least two pod IPs")
	}
	agentIP := net.ParseIP(*agent).To4()
	if agentIP == nil {
		return fmt.Errorf("bad agent ip %q", *agent)
	}

	conn, err := net.Dial("udp", *target)
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Printf("sending mock sFlow datagrams to %s (pods=%s, count=%d)\n", *target, *podList, *count)
	seq := uint32(1)
	totalFlows := 0
	start := time.Now()
	for n := 0; *count == 0 || n < *count; n++ {
		var flows []mockFlow
		for i := 0; i < 3; i++ {
			a, b := pods[rand.Intn(len(pods))], pods[rand.Intn(len(pods))]
			if a.Equal(b) {
				continue
			}
			flows = append(flows, mockFlow{
				src: a, dst: b,
				srcPort: uint16(30000 + rand.Intn(20000)), dstPort: 5201,
				proto: 6, frameLen: uint32(200 + rand.Intn(1300)),
			})
		}
		if len(flows) == 0 {
			flows = append(flows, mockFlow{
				src: pods[0], dst: pods[1],
				srcPort: uint16(30000 + rand.Intn(20000)), dstPort: 5201,
				proto: 6, frameLen: 512,
			})
		}

		pkt := buildMockDatagram(agentIP, seq, uint32(time.Since(start).Milliseconds()), uint32(*rate), flows)
		if _, err := conn.Write(pkt); err != nil {
			fmt.Fprintf(os.Stderr, "send: %v\n", err)
		} else {
			totalFlows += len(flows)
		}
		seq++
		if seq%50 == 0 {
			fmt.Printf("sent %d datagrams, %d flow samples\n", seq-1, totalFlows)
		}
		time.Sleep(*interval)
	}
	fmt.Printf("done, sent %d datagrams, %d flow samples\n", seq-1, totalFlows)
	return nil
}
