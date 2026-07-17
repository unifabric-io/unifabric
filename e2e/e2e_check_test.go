// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestObserveScaleOutGroupsAcceptsEitherTier1AllocationOrder(t *testing.T) {
	tests := []struct {
		name         string
		nodes12Tier1 string
		nodes34Tier1 string
	}{
		{
			name:         "original allocation order",
			nodes12Tier1: "tier1-group1",
			nodes34Tier1: "tier1-group2",
		},
		{
			name:         "reversed allocation order",
			nodes12Tier1: "tier1-group2",
			nodes34Tier1: "tier1-group1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups, errs := observeScaleOutGroups(map[string]kubernetesNode{
				"node-gpu-1": newKubernetesNode("node-gpu-1", tt.nodes12Tier1, "tier2-group1"),
				"node-gpu-2": newKubernetesNode("node-gpu-2", tt.nodes12Tier1, "tier2-group1"),
				"node-gpu-3": newKubernetesNode("node-gpu-3", tt.nodes34Tier1, "tier2-group1"),
				"node-gpu-4": newKubernetesNode("node-gpu-4", tt.nodes34Tier1, "tier2-group1"),
			})
			if len(errs) != 0 {
				t.Fatalf("observeScaleOutGroups() returned errors: %v", errs)
			}
			if groups.nodes12Tier1 != tt.nodes12Tier1 || groups.nodes34Tier1 != tt.nodes34Tier1 {
				t.Fatalf("observeScaleOutGroups() = %#v", groups)
			}
		})
	}
}

func TestObserveScaleOutGroupsRejectsMergedLeafDomains(t *testing.T) {
	_, errs := observeScaleOutGroups(map[string]kubernetesNode{
		"node-gpu-1": newKubernetesNode("node-gpu-1", "tier1-group1", "tier2-group1"),
		"node-gpu-2": newKubernetesNode("node-gpu-2", "tier1-group1", "tier2-group1"),
		"node-gpu-3": newKubernetesNode("node-gpu-3", "tier1-group1", "tier2-group1"),
		"node-gpu-4": newKubernetesNode("node-gpu-4", "tier1-group1", "tier2-group1"),
	})
	if len(errs) == 0 {
		t.Fatal("observeScaleOutGroups() accepted merged leaf domains")
	}
}

func newKubernetesNode(name, tier1, tier2 string) kubernetesNode {
	node := kubernetesNode{}
	node.Metadata.Name = name
	node.Metadata.Labels = map[string]string{
		defaultScaleOutTier1LabelKey: tier1,
		defaultScaleOutTier2LabelKey: tier2,
	}
	return node
}

func TestValidateTopologyStatusAcceptsArbitraryDepthAndStableGroups(t *testing.T) {
	status := topologyStatus{
		Domains: []topologyDomain{
			{Name: "tier4-root", Tier: 4},
			{Name: "tier3-spine", Tier: 3, Parent: "tier4-root"},
			{Name: "tier2-leaf", Tier: 2, Parent: "tier3-spine"},
			{Name: "tier1-clique", Tier: 1, Parent: "tier2-leaf"},
		},
		Nodes: []topologyNodeGroup{{
			Nodes:      []string{"node-gpu-2", "node-gpu-1"},
			DomainPath: []string{"tier4-root", "tier3-spine", "tier2-leaf", "tier1-clique"},
		}},
	}
	expectedDomains := map[string]topologyDomain{
		"tier4-root":   {Name: "tier4-root", Tier: 4},
		"tier3-spine":  {Name: "tier3-spine", Tier: 3, Parent: "tier4-root"},
		"tier2-leaf":   {Name: "tier2-leaf", Tier: 2, Parent: "tier3-spine"},
		"tier1-clique": {Name: "tier1-clique", Tier: 1, Parent: "tier2-leaf"},
	}
	expectedPaths := map[string]string{
		"tier4-root/tier3-spine/tier2-leaf/tier1-clique": "node-gpu-1,node-gpu-2",
	}

	if errs := validateTopologyStatus(status, expectedDomains, expectedPaths); len(errs) != 0 {
		t.Fatalf("validateTopologyStatus() returned errors: %v", errs)
	}
}

func TestValidateTopologyStatusRejectsWrongParentAndNodePath(t *testing.T) {
	status := topologyStatus{
		Domains: []topologyDomain{
			{Name: "tier2-root", Tier: 2},
			{Name: "tier1-leaf", Tier: 1, Parent: "wrong-root"},
		},
		Nodes: []topologyNodeGroup{{
			Nodes:      []string{"node-gpu-1"},
			DomainPath: []string{"tier2-root", "unexpected-leaf"},
		}},
	}
	expectedDomains := map[string]topologyDomain{
		"tier2-root": {Name: "tier2-root", Tier: 2},
		"tier1-leaf": {Name: "tier1-leaf", Tier: 1, Parent: "tier2-root"},
	}
	expectedPaths := map[string]string{
		"tier2-root/tier1-leaf": "node-gpu-1",
	}

	if errs := validateTopologyStatus(status, expectedDomains, expectedPaths); len(errs) < 2 {
		t.Fatalf("validateTopologyStatus() errors = %v, want parent and path errors", errs)
	}
}
