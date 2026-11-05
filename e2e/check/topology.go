// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package check

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/unifabric-io/unifabric/e2e/kube"
)

var (
	AutoTopologyGroupName = regexp.MustCompile(`^tier([1-9][0-9]*)-group([1-9][0-9]*)$`)
)

func NodeTopologyLabels() (bool, string) {
	var nodes kube.NodeList
	if err := kube.FetchJSON("nodes", &nodes); err != nil {
		return false, fmt.Sprintf("failed to read kubernetes nodes: %v", err)
	}

	actualByName := make(map[string]kube.Node, len(nodes.Items))
	for _, node := range nodes.Items {
		actualByName[node.Metadata.Name] = node
	}

	groups, errs := ObserveScaleOutGroups(actualByName)
	if len(errs) > 0 {
		return false, strings.Join(errs, "; ")
	}

	return true, fmt.Sprintf(
		"validated topology labels for 4 GPU nodes: nodes 1/2=%s, nodes 3/4=%s, parent=%s",
		groups.Nodes12Tier1,
		groups.Nodes34Tier1,
		groups.Tier2,
	)
}

type ScaleOutGroups struct {
	Nodes12Tier1 string
	Nodes34Tier1 string
	Tier2        string
}

func ObserveScaleOutGroups(actualByName map[string]kube.Node) (ScaleOutGroups, []string) {
	groups := ScaleOutGroups{}
	errs := []string{}
	nodeNames := []string{"node-gpu-1", "node-gpu-2", "node-gpu-3", "node-gpu-4"}
	tier1ByNode := map[string]string{}
	tier2ByNode := map[string]string{}

	for _, nodeName := range nodeNames {
		node, ok := actualByName[nodeName]
		if !ok {
			errs = append(errs, fmt.Sprintf("%s: kubernetes node missing", nodeName))
			continue
		}
		labels := node.Metadata.Labels
		tier1ByNode[nodeName] = labels[kube.ScaleOutTier1LabelKey]
		tier2ByNode[nodeName] = labels[kube.ScaleOutTier2LabelKey]
		if !isAutoTopologyGroupForTier(tier1ByNode[nodeName], 1) {
			errs = append(errs, fmt.Sprintf("%s: tier 1 label has invalid value %q", nodeName, tier1ByNode[nodeName]))
		}
		if !isAutoTopologyGroupForTier(tier2ByNode[nodeName], 2) {
			errs = append(errs, fmt.Sprintf("%s: tier 2 label has invalid value %q", nodeName, tier2ByNode[nodeName]))
		}
		if actualTier3 := labels[kube.ScaleOutTier3LabelKey]; actualTier3 != "" {
			errs = append(errs, fmt.Sprintf("%s: expected empty tier 3 label, got %s", nodeName, actualTier3))
		}
	}

	groups.Nodes12Tier1 = tier1ByNode["node-gpu-1"]
	groups.Nodes34Tier1 = tier1ByNode["node-gpu-3"]
	groups.Tier2 = tier2ByNode["node-gpu-1"]
	if groups.Nodes12Tier1 != tier1ByNode["node-gpu-2"] {
		errs = append(errs, fmt.Sprintf("nodes 1/2 have different tier 1 groups: %q and %q", groups.Nodes12Tier1, tier1ByNode["node-gpu-2"]))
	}
	if groups.Nodes34Tier1 != tier1ByNode["node-gpu-4"] {
		errs = append(errs, fmt.Sprintf("nodes 3/4 have different tier 1 groups: %q and %q", groups.Nodes34Tier1, tier1ByNode["node-gpu-4"]))
	}
	if groups.Nodes12Tier1 != "" && groups.Nodes12Tier1 == groups.Nodes34Tier1 {
		errs = append(errs, fmt.Sprintf("both leaf domains unexpectedly use tier 1 group %q", groups.Nodes12Tier1))
	}
	for _, nodeName := range nodeNames[1:] {
		if groups.Tier2 != tier2ByNode[nodeName] {
			errs = append(errs, fmt.Sprintf("%s: tier 2 group expected %q, got %q", nodeName, groups.Tier2, tier2ByNode[nodeName]))
		}
	}

	sort.Strings(errs)
	return groups, errs
}

func isAutoTopologyGroupForTier(value string, tier int) bool {
	match := AutoTopologyGroupName.FindStringSubmatch(value)
	return len(match) == 3 && match[1] == strconv.Itoa(tier)
}

func ScaleOutTopologyStatus() (bool, string) {
	return validateScaleOutTopologyStatusWithMembers(true)
}

func ScaleOutNodeOnlyTopologyStatus() (bool, string) {
	return validateScaleOutTopologyStatusWithMembers(false)
}

func validateScaleOutTopologyStatusWithMembers(includePhysicalLeaves bool) (bool, string) {
	var nodes kube.NodeList
	if err := kube.FetchJSON("nodes", &nodes); err != nil {
		return false, fmt.Sprintf("failed to read kubernetes nodes: %v", err)
	}
	actualNodesByName := make(map[string]kube.Node, len(nodes.Items))
	for _, node := range nodes.Items {
		actualNodesByName[node.Metadata.Name] = node
	}
	groups, nodeErrs := ObserveScaleOutGroups(actualNodesByName)
	if len(nodeErrs) != 0 {
		return false, "node topology labels: " + strings.Join(nodeErrs, "; ")
	}

	var topology kube.Topology
	if err := kube.FetchJSON("topologies.unifabric.io/scaleout", &topology); err != nil {
		return false, fmt.Sprintf("failed to read Topology/scaleout: %v", err)
	}
	type expectedDomain struct {
		tier    int
		parent  string
		members []string
	}
	leaf12Members := []string{}
	leaf34Members := []string{}
	if includePhysicalLeaves {
		leaf12Members = []string{"switch-gpu-leaf1", "switch-gpu-leaf2"}
		leaf34Members = []string{"switch-gpu-leaf3", "switch-gpu-leaf4"}
	}
	expectedDomains := map[string]expectedDomain{
		groups.Tier2:        {tier: 2, members: []string{"switch-gpu-spine1"}},
		groups.Nodes12Tier1: {tier: 1, parent: groups.Tier2, members: leaf12Members},
		groups.Nodes34Tier1: {tier: 1, parent: groups.Tier2, members: leaf34Members},
	}
	errs := []string{}
	if len(topology.Status.Domains) != len(expectedDomains) {
		errs = append(errs, fmt.Sprintf("domain count expected %d, got %d", len(expectedDomains), len(topology.Status.Domains)))
	}
	for _, domain := range topology.Status.Domains {
		expected, ok := expectedDomains[domain.Name]
		if !ok {
			errs = append(errs, fmt.Sprintf("unexpected domain %s", domain.Name))
			continue
		}
		if domain.Tier != expected.tier || domain.Parent != expected.parent || strings.Join(SortedUnique(domain.SwitchMember), ",") != strings.Join(expected.members, ",") {
			errs = append(errs, fmt.Sprintf("domain %s expected tier=%d parent=%s members=%v, got tier=%d parent=%s members=%v", domain.Name, expected.tier, expected.parent, expected.members, domain.Tier, domain.Parent, domain.SwitchMember))
		}
	}
	expectedPaths := map[string]string{
		groups.Tier2 + "/" + groups.Nodes12Tier1: "node-gpu-1,node-gpu-2",
		groups.Tier2 + "/" + groups.Nodes34Tier1: "node-gpu-3,node-gpu-4",
	}
	if len(topology.Status.Nodes) != len(expectedPaths) {
		errs = append(errs, fmt.Sprintf("node group count expected %d, got %d", len(expectedPaths), len(topology.Status.Nodes)))
	}
	for _, nodeGroup := range topology.Status.Nodes {
		path := strings.Join(nodeGroup.SwitchDomainPath, "/")
		if strings.Join(SortedUnique(nodeGroup.Nodes), ",") != expectedPaths[path] {
			errs = append(errs, fmt.Sprintf("path %s has unexpected nodes %v", path, nodeGroup.Nodes))
		}
	}

	var switches kube.SwitchList
	if err := kube.FetchJSON("switches.unifabric.io", &switches); err != nil {
		return false, fmt.Sprintf("failed to read Switch labels: %v", err)
	}
	expectedSwitchDomains := map[string]string{
		"switch-gpu-spine1": groups.Tier2,
	}
	if includePhysicalLeaves {
		expectedSwitchDomains["switch-gpu-leaf1"] = groups.Nodes12Tier1
		expectedSwitchDomains["switch-gpu-leaf2"] = groups.Nodes12Tier1
		expectedSwitchDomains["switch-gpu-leaf3"] = groups.Nodes34Tier1
		expectedSwitchDomains["switch-gpu-leaf4"] = groups.Nodes34Tier1
	}
	errs = append(errs, validateSwitchDomainLabels(switches.Items, expectedSwitchDomains)...)
	if len(errs) != 0 {
		sort.Strings(errs)
		return false, strings.Join(errs, "; ")
	}
	return true, fmt.Sprintf("validated %d domains and %d Node paths", len(expectedDomains), len(expectedPaths))
}

func SortedUnique(values []string) []string {
	seen := map[string]struct{}{}
	for _, value := range values {
		if value != "" {
			seen[value] = struct{}{}
		}
	}

	unique := make([]string, 0, len(seen))
	for value := range seen {
		unique = append(unique, value)
	}
	sort.Strings(unique)
	return unique
}

func TopologyMatches(
	status kube.TopologyStatus,
	expectedDomains map[string]kube.TopologyDomain,
	expectedPaths map[string]string,
) []string {
	errs := []string{}
	if len(status.Domains) != len(expectedDomains) {
		errs = append(errs, fmt.Sprintf("domain count expected %d, got %d", len(expectedDomains), len(status.Domains)))
	}
	for _, domain := range status.Domains {
		expected, ok := expectedDomains[domain.Name]
		if !ok {
			errs = append(errs, fmt.Sprintf("unexpected domain %s", domain.Name))
			continue
		}
		if domain.Tier != expected.Tier ||
			domain.Parent != expected.Parent ||
			!reflect.DeepEqual(SortedUnique(domain.SwitchMember), SortedUnique(expected.SwitchMember)) {
			errs = append(errs, fmt.Sprintf(
				"domain %s expected tier=%d parent=%s members=%v, got tier=%d parent=%s members=%v",
				domain.Name,
				expected.Tier,
				expected.Parent,
				expected.SwitchMember,
				domain.Tier,
				domain.Parent,
				domain.SwitchMember,
			))
		}
	}
	if len(status.Nodes) != len(expectedPaths) {
		errs = append(errs, fmt.Sprintf("node group count expected %d, got %d", len(expectedPaths), len(status.Nodes)))
	}
	for _, nodeGroup := range status.Nodes {
		path := strings.Join(nodeGroup.SwitchDomainPath, "/")
		expectedNodes, ok := expectedPaths[path]
		if !ok {
			errs = append(errs, fmt.Sprintf("unexpected Node path %s", path))
			continue
		}
		if actualNodes := strings.Join(SortedUnique(nodeGroup.Nodes), ","); actualNodes != expectedNodes {
			errs = append(errs, fmt.Sprintf("path %s expected nodes %s, got %s", path, expectedNodes, actualNodes))
		}
	}
	sort.Strings(errs)
	return errs
}
