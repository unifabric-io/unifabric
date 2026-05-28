// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"context"
	"encoding/json"
	"net"
	"sync"

	corev1 "k8s.io/api/core/v1"
)

const (
	multusNetworkStatusAnnotation = "k8s.v1.cni.cncf.io/network-status"
	podIPsAnnotation              = "k8s.v1.cni.cncf.io/podIPs"
	podIPAnnotation               = "k8s.v1.cni.cncf.io/podIP"
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

func (c *PodIPCache) ReplacePods(ctx context.Context, pods []corev1.Pod, ownerResolver OwnerResolver) error {
	infos := make([]PodInfo, 0, len(pods))
	for i := range pods {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		pod := &pods[i]
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		info := PodInfo{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			NodeName:  pod.Spec.NodeName,
			IPs:       podIPs(pod),
		}
		if ownerResolver != nil {
			info.TopOwner = ownerResolver.OwnerForPod(ctx, pod)
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		infos = append(infos, info)
	}
	c.Replace(infos)
	return nil
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

func podIPs(pod *corev1.Pod) []net.IP {
	if pod == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var ips []net.IP

	addIP := func(value string) {
		ip := net.ParseIP(value)
		key := normalizeIPKey(ip)
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		ips = append(ips, ip)
	}

	for _, podIP := range pod.Status.PodIPs {
		addIP(podIP.IP)
	}
	addIP(pod.Status.PodIP)
	addAnnotatedPodIPs(pod.Annotations, addIP)

	return ips
}

func addAnnotatedPodIPs(annotations map[string]string, addIP func(string)) {
	if len(annotations) == 0 || addIP == nil {
		return
	}

	if value := annotations[multusNetworkStatusAnnotation]; value != "" {
		var statuses []struct {
			IP  string   `json:"ip"`
			IPs []string `json:"ips"`
		}
		if err := json.Unmarshal([]byte(value), &statuses); err == nil {
			for _, status := range statuses {
				addIP(status.IP)
				for _, ip := range status.IPs {
					addIP(ip)
				}
			}
		}
	}

	if value := annotations[podIPAnnotation]; value != "" {
		addIP(value)
	}
	if value := annotations[podIPsAnnotation]; value != "" {
		var values []string
		if err := json.Unmarshal([]byte(value), &values); err == nil {
			for _, ip := range values {
				addIP(ip)
			}
			return
		}
		addIP(value)
	}
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
