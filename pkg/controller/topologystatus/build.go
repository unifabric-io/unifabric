// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package topologystatus

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/topologylabel"
	"k8s.io/apimachinery/pkg/util/validation"
)

type LabeledResource struct {
	Name   string
	Labels map[string]string
}

type LabelSnapshot struct {
	Nodes    []LabeledResource
	Switches []LabeledResource
}

type BuildResult struct {
	Status  v1beta1.TopologyStatus
	Pending []string
}

type domainState struct {
	domain  v1beta1.TopologyDomain
	parents map[string]struct{}
	members map[string]struct{}
}

// BuildTopologyStatus builds one complete status snapshot from Node and Switch
// labels. Conflicting Node inputs return an error and no partial status. An
// orphan Switch member is reported as pending and omitted from members.
func BuildTopologyStatus(labelTemplate *topologylabel.Template, snapshot LabelSnapshot) (BuildResult, error) {
	if labelTemplate == nil {
		return BuildResult{}, fmt.Errorf("topology label template must not be nil")
	}

	domains := map[string]*domainState{}
	nodeGroups := map[string]*v1beta1.TopologyNodeGroup{}
	nodes := append([]LabeledResource(nil), snapshot.Nodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })

	for _, node := range nodes {
		valuesByTier, err := labelsByTier(labelTemplate, node.Labels)
		if err != nil {
			return BuildResult{}, fmt.Errorf("Node %q: %w", node.Name, err)
		}
		if len(valuesByTier) == 0 {
			continue
		}

		tiers := sortedTiers(valuesByTier)
		for index, tier := range tiers {
			if tier != index+1 {
				return BuildResult{}, fmt.Errorf("Node %q has a discontinuous topology path: expected tier %d, found tier %d", node.Name, index+1, tier)
			}
			value := valuesByTier[tier]
			if err := validateDomainName(value); err != nil {
				return BuildResult{}, fmt.Errorf("Node %q tier %d: %w", node.Name, tier, err)
			}
			if err := ensureDomain(domains, value, tier); err != nil {
				return BuildResult{}, fmt.Errorf("Node %q: %w", node.Name, err)
			}
		}

		for tier := 1; tier < len(tiers); tier++ {
			childName := valuesByTier[tier]
			parentName := valuesByTier[tier+1]
			domains[childName].parents[parentName] = struct{}{}
			if len(domains[childName].parents) > 1 {
				return BuildResult{}, fmt.Errorf("domain %q has multiple parents: %s", childName, strings.Join(sortedSet(domains[childName].parents), ", "))
			}
		}

		path := make([]string, 0, len(tiers))
		for tier := len(tiers); tier >= 1; tier-- {
			path = append(path, valuesByTier[tier])
		}
		groupKey := strings.Join(path, "\x00")
		group := nodeGroups[groupKey]
		if group == nil {
			group = &v1beta1.TopologyNodeGroup{DomainPath: path}
			nodeGroups[groupKey] = group
		}
		group.Nodes = append(group.Nodes, node.Name)
	}

	pending := []string{}
	switches := append([]LabeledResource(nil), snapshot.Switches...)
	sort.Slice(switches, func(i, j int) bool { return switches[i].Name < switches[j].Name })
	for _, sw := range switches {
		value := sw.Labels[v1beta1.TopologyDomainLabel]
		if value == "" {
			continue
		}
		if err := validateDomainName(value); err != nil {
			return BuildResult{}, fmt.Errorf("Switch %q: %w", sw.Name, err)
		}
		domain := domains[value]
		if domain == nil {
			pending = append(pending, fmt.Sprintf("Switch %q domain %q has no matching Node domain", sw.Name, value))
			continue
		}
		domain.members[sw.Name] = struct{}{}
	}

	status := v1beta1.TopologyStatus{
		Domains: make([]v1beta1.TopologyDomain, 0, len(domains)),
		Nodes:   make([]v1beta1.TopologyNodeGroup, 0, len(nodeGroups)),
	}
	for _, state := range domains {
		if len(state.parents) == 1 {
			state.domain.Parent = sortedSet(state.parents)[0]
		}
		state.domain.Members = sortedSet(state.members)
		status.Domains = append(status.Domains, state.domain)
	}
	if len(status.Domains) == 0 {
		status.Domains = nil
	}
	sort.Slice(status.Domains, func(i, j int) bool {
		if status.Domains[i].Tier != status.Domains[j].Tier {
			return status.Domains[i].Tier > status.Domains[j].Tier
		}
		return status.Domains[i].Name < status.Domains[j].Name
	})

	for _, group := range nodeGroups {
		sort.Strings(group.Nodes)
		status.Nodes = append(status.Nodes, *group)
	}
	if len(status.Nodes) == 0 {
		status.Nodes = nil
	}
	sort.Slice(status.Nodes, func(i, j int) bool {
		left := strings.Join(status.Nodes[i].DomainPath, "\x00")
		right := strings.Join(status.Nodes[j].DomainPath, "\x00")
		if left != right {
			return left < right
		}
		return strings.Join(status.Nodes[i].Nodes, "\x00") < strings.Join(status.Nodes[j].Nodes, "\x00")
	})
	sort.Strings(pending)

	return BuildResult{Status: status, Pending: pending}, nil
}

func labelsByTier(labelTemplate *topologylabel.Template, labels map[string]string) (map[int]string, error) {
	result := map[int]string{}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		tier, ok := labelTemplate.MatchTier(key)
		if !ok {
			continue
		}
		value := labels[key]
		if value == "" {
			continue
		}
		if existing, exists := result[tier]; exists && existing != value {
			return nil, fmt.Errorf("multiple label keys resolve to tier %d with values %q and %q", tier, existing, value)
		}
		result[tier] = value
	}
	return result, nil
}

func ensureDomain(domains map[string]*domainState, name string, tier int) error {
	if tier > math.MaxInt32 {
		return fmt.Errorf("tier %d exceeds the Topology API range", tier)
	}
	if existing := domains[name]; existing != nil {
		if int(existing.domain.Tier) != tier {
			return fmt.Errorf("domain %q appears in both tier %d and tier %d", name, existing.domain.Tier, tier)
		}
		return nil
	}
	domains[name] = &domainState{
		domain:  v1beta1.TopologyDomain{Name: name, Tier: int32(tier)},
		parents: map[string]struct{}{},
		members: map[string]struct{}{},
	}
	return nil
}

func validateDomainName(name string) error {
	if name == "" {
		return fmt.Errorf("domain name must not be empty")
	}
	if problems := validation.IsValidLabelValue(name); len(problems) != 0 {
		return fmt.Errorf("domain name %q is invalid: %s", name, strings.Join(problems, ", "))
	}
	return nil
}

func sortedTiers(values map[int]string) []int {
	tiers := make([]int, 0, len(values))
	for tier := range values {
		tiers = append(tiers, tier)
	}
	sort.Ints(tiers)
	return tiers
}

func sortedSet(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
