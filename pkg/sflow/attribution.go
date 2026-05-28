// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"net"
	"sync"

	corev1 "k8s.io/api/core/v1"
)

type PodLookup interface {
	Lookup(ip net.IP) (EndpointAttribution, bool)
}

type PodIPCache struct {
	mu   sync.RWMutex
	pods map[string]EndpointAttribution
}

func NewPodIPCache() *PodIPCache {
	return &PodIPCache{pods: make(map[string]EndpointAttribution)}
}

func (c *PodIPCache) Replace(pods []PodInfo) {
	next := make(map[string]EndpointAttribution)
	for _, pod := range pods {
		attr := EndpointAttribution{
			PodName:      pod.Name,
			PodNamespace: pod.Namespace,
			NodeName:     pod.NodeName,
		}
		if pod.TopOwner != nil {
			attr.TopOwnerKind = pod.TopOwner.Kind
			attr.TopOwnerName = pod.TopOwner.Name
			attr.TopOwnerNamespace = pod.TopOwner.Namespace
		}
		for _, ip := range pod.IPs {
			key := normalizeIPKey(ip)
			if key != "" {
				next[key] = attr
			}
		}
	}
	c.mu.Lock()
	c.pods = next
	c.mu.Unlock()
}

func (c *PodIPCache) ReplacePods(pods []corev1.Pod, ownerResolver OwnerResolver) {
	infos := make([]PodInfo, 0, len(pods))
	for i := range pods {
		pod := &pods[i]
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		info := PodInfo{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			NodeName:  pod.Spec.NodeName,
		}
		for _, podIP := range pod.Status.PodIPs {
			if ip := net.ParseIP(podIP.IP); ip != nil {
				info.IPs = append(info.IPs, ip)
			}
		}
		if len(info.IPs) == 0 {
			if ip := net.ParseIP(pod.Status.PodIP); ip != nil {
				info.IPs = append(info.IPs, ip)
			}
		}
		if ownerResolver != nil {
			info.TopOwner = ownerResolver.OwnerForPod(pod)
		}
		infos = append(infos, info)
	}
	c.Replace(infos)
}

func (c *PodIPCache) Lookup(ip net.IP) (EndpointAttribution, bool) {
	key := normalizeIPKey(ip)
	if key == "" {
		return EndpointAttribution{}, false
	}
	c.mu.RLock()
	attr, ok := c.pods[key]
	c.mu.RUnlock()
	return attr, ok
}

func EnrichRecord(record FlowRecord, lookup PodLookup) FlowRecord {
	if lookup == nil {
		return record
	}
	if attr, ok := lookup.Lookup(record.SrcAddr); ok {
		record.SrcAttribution = attr
	}
	if attr, ok := lookup.Lookup(record.DstAddr); ok {
		record.DstAttribution = attr
	}
	return record
}

func normalizeIPKey(ip net.IP) string {
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return net.IP(v4).String()
	}
	if v16 := ip.To16(); v16 != nil {
		return net.IP(v16).String()
	}
	return ""
}
