// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ibswitchdomain

import (
	"fmt"
	"sort"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
)

type resultState int

const (
	stateAssigned resultState = iota
	statePending
	stateConflict
)

type domainResult struct {
	state   resultState
	domain  string
	message string
}

// computeDomains decides the unifabric.io/domain value of every managed Switch.
//
// A Switch with a KubernetesNode neighbor is a leaf (tier 1). Every other
// Switch gets tier = distance to the nearest leaf + 1. The domain is the Node
// topology label of that tier, shared by all Nodes below the Switch, so it is
// always the value Topograph already wrote.
func computeDomains(switches []v1beta1.Switch, nodeLabels map[string]map[string]string, labelKey func(tier int) (string, error)) map[string]domainResult {
	managed := make(map[string]*v1beta1.Switch, len(switches))
	for i := range switches {
		managed[switches[i].Name] = &switches[i]
	}

	nodeNeighbors := make(map[string][]string, len(managed))
	adjacent := make(map[string]map[string]struct{}, len(managed))
	for name := range managed {
		adjacent[name] = map[string]struct{}{}
	}
	for name, sw := range managed {
		for _, neighbor := range sw.Status.LLDPNeighbors {
			switch neighbor.RemoteSystemType {
			case v1beta1.SwitchLLDPRemoteSystemTypeKubernetesNode:
				nodeNeighbors[name] = append(nodeNeighbors[name], neighbor.RemoteSystemName)
			case v1beta1.SwitchLLDPRemoteSystemTypeSwitch:
				if _, ok := managed[neighbor.RemoteSystemName]; ok && neighbor.RemoteSystemName != name {
					adjacent[name][neighbor.RemoteSystemName] = struct{}{}
					adjacent[neighbor.RemoteSystemName][name] = struct{}{}
				}
			}
		}
	}

	tier := make(map[string]int, len(managed))
	var frontier []string
	for name := range managed {
		if len(nodeNeighbors[name]) > 0 {
			tier[name] = 1
			frontier = append(frontier, name)
		}
	}
	for level := 2; len(frontier) > 0; level++ {
		var next []string
		for _, name := range frontier {
			for neighbor := range adjacent[name] {
				if _, seen := tier[neighbor]; !seen {
					tier[neighbor] = level
					next = append(next, neighbor)
				}
			}
		}
		frontier = next
	}

	descendants := make(map[string]map[string]struct{}, len(managed))
	var collect func(name string) map[string]struct{}
	collect = func(name string) map[string]struct{} {
		if nodes, ok := descendants[name]; ok {
			return nodes
		}
		nodes := map[string]struct{}{}
		descendants[name] = nodes
		for _, node := range nodeNeighbors[name] {
			nodes[node] = struct{}{}
		}
		for neighbor := range adjacent[name] {
			if tier[neighbor] < tier[name] {
				for node := range collect(neighbor) {
					nodes[node] = struct{}{}
				}
			}
		}
		return nodes
	}

	results := make(map[string]domainResult, len(managed))
	for name := range managed {
		level, ok := tier[name]
		if !ok {
			results[name] = domainResult{state: statePending, message: "no path to a Switch that connects Kubernetes Nodes"}
			continue
		}
		key, err := labelKey(level)
		if err != nil {
			results[name] = domainResult{state: statePending, message: err.Error()}
			continue
		}
		results[name] = resolveDomain(key, collect(name), nodeLabels)
	}
	return results
}

func resolveDomain(key string, nodes map[string]struct{}, nodeLabels map[string]map[string]string) domainResult {
	names := make([]string, 0, len(nodes))
	for node := range nodes {
		names = append(names, node)
	}
	sort.Strings(names)

	var domain, first string
	for _, node := range names {
		labels, ok := nodeLabels[node]
		if !ok {
			return domainResult{state: statePending, message: fmt.Sprintf("Node %q not found", node)}
		}
		value := labels[key]
		if value == "" {
			return domainResult{state: statePending, message: fmt.Sprintf("Node %q has no label %q yet", node, key)}
		}
		if first == "" {
			domain, first = value, node
			continue
		}
		if value != domain {
			return domainResult{state: stateConflict, message: fmt.Sprintf("label %q differs: Node %q=%q, Node %q=%q", key, first, domain, node, value)}
		}
	}
	if domain == "" {
		return domainResult{state: statePending, message: "no Kubernetes Node below this Switch"}
	}
	return domainResult{state: stateAssigned, domain: domain}
}
