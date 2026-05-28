// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"net"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestPodIPCacheAttribution(t *testing.T) {
	cache := NewPodIPCache()
	cache.Replace([]PodInfo{
		{
			Name:      "trainer-0",
			Namespace: "training",
			NodeName:  "node-a",
			IPs:       []net.IP{net.ParseIP("10.1.1.10")},
			TopOwner: &OwnerRef{
				Kind:      "Deployment",
				Namespace: "training",
				Name:      "trainer",
			},
		},
	})
	record := EnrichRecord(FlowRecord{
		SrcAddr: net.ParseIP("10.1.1.10"),
		DstAddr: net.ParseIP("10.1.1.20"),
	}, cache)
	if record.SrcAttribution.PodName != "trainer-0" || record.SrcAttribution.TopOwnerName != "trainer" {
		t.Fatalf("source attribution = %#v", record.SrcAttribution)
	}
	if record.DstAttribution.PodName != "" {
		t.Fatalf("destination attribution = %#v, want empty", record.DstAttribution)
	}
}

func TestPodIPCacheReplacePodsSkipsNonRunningAndSupportsPodIPs(t *testing.T) {
	cache := NewPodIPCache()
	cache.ReplacePods([]corev1.Pod{
		{
			Status: corev1.PodStatus{Phase: corev1.PodPending, PodIP: "10.0.0.1"},
		},
		{
			Spec:       corev1.PodSpec{NodeName: "node-b"},
			ObjectMeta: metav1Object("workloads", "worker-0"),
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				PodIPs: []corev1.PodIP{
					{IP: "10.0.0.2"},
					{IP: "2001:db8::2"},
				},
			},
		},
	}, nil)
	if _, ok := cache.Lookup(net.ParseIP("10.0.0.1")); ok {
		t.Fatalf("pending pod was cached")
	}
	if attr, ok := cache.Lookup(net.ParseIP("2001:db8::2")); !ok || attr.PodName != "worker-0" {
		t.Fatalf("ipv6 lookup = %#v/%v", attr, ok)
	}
}
