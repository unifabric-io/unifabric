// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import "testing"

func TestQPInfoIndexExactAndUniqueQPNFallback(t *testing.T) {
	index := newQPInfoIndex()
	key := qpKey{Tgid: 100, QPN: 42}
	info := qpInfo{DGID: [16]byte{15: 1}}
	index.add(key, info)

	if got, ok := index.lookup(key); !ok || got != info {
		t.Fatalf("exact lookup = %+v, %t; want %+v, true", got, ok, info)
	}
	if got, ok := index.lookup(qpKey{Tgid: 200, QPN: 42}); !ok || got != info {
		t.Fatalf("QPN fallback = %+v, %t; want %+v, true", got, ok, info)
	}
}

func TestQPInfoIndexRejectsAmbiguousQPNFallback(t *testing.T) {
	index := newQPInfoIndex()
	firstKey := qpKey{Tgid: 100, QPN: 42}
	secondKey := qpKey{Tgid: 200, QPN: 42}
	firstInfo := qpInfo{DGID: [16]byte{15: 1}}
	secondInfo := qpInfo{DGID: [16]byte{15: 2}}
	index.add(firstKey, firstInfo)
	index.add(secondKey, secondInfo)

	if got, ok := index.lookup(firstKey); !ok || got != firstInfo {
		t.Fatalf("exact lookup after collision = %+v, %t; want %+v, true", got, ok, firstInfo)
	}
	if _, ok := index.lookup(qpKey{Tgid: 300, QPN: 42}); ok {
		t.Fatal("ambiguous QPN fallback unexpectedly succeeded")
	}
}
