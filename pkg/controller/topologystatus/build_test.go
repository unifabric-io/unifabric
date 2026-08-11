// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package topologystatus

import (
	"reflect"
	"testing"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/topologylabel"
)

func mustLabelTemplate(t *testing.T) *topologylabel.Template {
	t.Helper()
	compiled, err := topologylabel.Compile("test", "scale-out.unifabric.io/tier-{{ .Tier }}")
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func topologyLabels(values ...string) map[string]string {
	labels := map[string]string{}
	for index, value := range values {
		labels["scale-out.unifabric.io/tier-"+string(rune('1'+index))] = value
	}
	return labels
}

func topologyDomain(value string) map[string]string {
	return map[string]string{v1beta1.TopologyDomainLabel: value}
}

func TestBuildTopologyStatusBuildsArbitraryDepthAndStableGroups(t *testing.T) {
	result, err := BuildTopologyStatus(mustLabelTemplate(t), LabelSnapshot{
		Nodes: []LabeledResource{
			{Name: "node-b", Labels: topologyLabels("tier1-group1", "tier2-group1", "tier3-group1", "tier4-group1")},
			{Name: "node-a", Labels: topologyLabels("tier1-group1", "tier2-group1", "tier3-group1", "tier4-group1")},
			{Name: "node-c", Labels: topologyLabels("rack-c", "row-c")},
		},
		Switches: []LabeledResource{
			{Name: "leaf-b", Labels: topologyDomain("tier1-group1")},
			{Name: "leaf-a", Labels: topologyDomain("tier1-group1")},
			{Name: "orphan", Labels: topologyDomain("missing")},
		},
	}, nil)
	if err != nil {
		t.Fatalf("BuildTopologyStatus() error = %v", err)
	}
	want := v1beta1.TopologyStatus{
		Domains: []v1beta1.TopologyDomain{
			{Name: "tier4-group1", Tier: 4},
			{Name: "tier3-group1", Tier: 3, Parent: "tier4-group1"},
			{Name: "row-c", Tier: 2},
			{Name: "tier2-group1", Tier: 2, Parent: "tier3-group1"},
			{Name: "rack-c", Tier: 1, Parent: "row-c"},
			{Name: "tier1-group1", Tier: 1, Parent: "tier2-group1", SwitchMember: []string{"leaf-a", "leaf-b"}},
		},
		Nodes: []v1beta1.TopologyNodeGroup{
			{Name: "node-group1", Nodes: []string{"node-c"}, SwitchDomainPath: []string{"row-c", "rack-c"}},
			{Name: "node-group2", Nodes: []string{"node-a", "node-b"}, SwitchDomainPath: []string{"tier4-group1", "tier3-group1", "tier2-group1", "tier1-group1"}},
		},
	}
	if !reflect.DeepEqual(result.Status, want) {
		t.Fatalf("status = %#v, want %#v", result.Status, want)
	}
	if len(result.Pending) != 1 {
		t.Fatalf("pending = %#v", result.Pending)
	}
}

func TestBuildTopologyStatusRejectsDiscontinuousPath(t *testing.T) {
	_, err := BuildTopologyStatus(mustLabelTemplate(t), LabelSnapshot{Nodes: []LabeledResource{{
		Name: "node-a",
		Labels: map[string]string{
			"scale-out.unifabric.io/tier-1": "rack-a",
			"scale-out.unifabric.io/tier-3": "region-a",
		},
	}}}, nil)
	if err == nil {
		t.Fatal("BuildTopologyStatus() accepted a discontinuous path")
	}
}

func TestBuildTopologyStatusPreservesPreviousOnMultipleParents(t *testing.T) {
	_, err := BuildTopologyStatus(mustLabelTemplate(t), LabelSnapshot{Nodes: []LabeledResource{
		{Name: "node-a", Labels: topologyLabels("rack-a", "row-a")},
		{Name: "node-b", Labels: topologyLabels("rack-a", "row-b")},
	}}, nil)
	if err == nil {
		t.Fatal("BuildTopologyStatus() accepted multiple parents")
	}
}

func TestBuildTopologyStatusAllowsEmptySnapshot(t *testing.T) {
	result, err := BuildTopologyStatus(mustLabelTemplate(t), LabelSnapshot{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status.Domains != nil || result.Status.Nodes != nil {
		t.Fatalf("empty status = %#v", result.Status)
	}
}

func TestBuildTopologyStatusKeepsNodeGroupNamesStable(t *testing.T) {
	snapshotWithThreeGroups := LabelSnapshot{Nodes: []LabeledResource{
		{Name: "node-a", Labels: topologyLabels("rack-a")},
		{Name: "node-b", Labels: topologyLabels("rack-b")},
		{Name: "node-c", Labels: topologyLabels("rack-c")},
	}}

	first, err := BuildTopologyStatus(mustLabelTemplate(t), snapshotWithThreeGroups, nil)
	if err != nil {
		t.Fatal(err)
	}
	firstNames := nodeGroupNamesByPath(first.Status.Nodes)
	if firstNames["rack-a"] != "node-group1" || firstNames["rack-b"] != "node-group2" || firstNames["rack-c"] != "node-group3" {
		t.Fatalf("first names = %#v", firstNames)
	}

	// Remove the middle group (rack-b) and re-build using the previous
	// status.nodes: rack-a and rack-c must keep their existing names, and
	// rack-b's freed number 2 must remain unused since it no longer exists.
	snapshotWithoutMiddleGroup := LabelSnapshot{Nodes: []LabeledResource{
		{Name: "node-a", Labels: topologyLabels("rack-a")},
		{Name: "node-c", Labels: topologyLabels("rack-c")},
	}}
	second, err := BuildTopologyStatus(mustLabelTemplate(t), snapshotWithoutMiddleGroup, first.Status.Nodes)
	if err != nil {
		t.Fatal(err)
	}
	secondNames := nodeGroupNamesByPath(second.Status.Nodes)
	if secondNames["rack-a"] != "node-group1" || secondNames["rack-c"] != "node-group3" {
		t.Fatalf("second names = %#v", secondNames)
	}

	// Re-introduce rack-b and add a brand-new group rack-d: rack-b reclaims
	// its freed number, and the new rack-d group gets the next free number.
	snapshotWithNewGroup := LabelSnapshot{Nodes: []LabeledResource{
		{Name: "node-a", Labels: topologyLabels("rack-a")},
		{Name: "node-b", Labels: topologyLabels("rack-b")},
		{Name: "node-c", Labels: topologyLabels("rack-c")},
		{Name: "node-d", Labels: topologyLabels("rack-d")},
	}}
	third, err := BuildTopologyStatus(mustLabelTemplate(t), snapshotWithNewGroup, second.Status.Nodes)
	if err != nil {
		t.Fatal(err)
	}
	thirdNames := nodeGroupNamesByPath(third.Status.Nodes)
	if thirdNames["rack-a"] != "node-group1" || thirdNames["rack-b"] != "node-group2" || thirdNames["rack-c"] != "node-group3" || thirdNames["rack-d"] != "node-group4" {
		t.Fatalf("third names = %#v", thirdNames)
	}
}

func nodeGroupNamesByPath(groups []v1beta1.TopologyNodeGroup) map[string]string {
	names := map[string]string{}
	for _, group := range groups {
		names[nodeGroupKey(group.SwitchDomainPath)] = group.Name
	}
	return names
}
