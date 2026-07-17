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
	})
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
			{Name: "tier1-group1", Tier: 1, Parent: "tier2-group1", Members: []string{"leaf-a", "leaf-b"}},
		},
		Nodes: []v1beta1.TopologyNodeGroup{
			{Nodes: []string{"node-c"}, DomainPath: []string{"row-c", "rack-c"}},
			{Nodes: []string{"node-a", "node-b"}, DomainPath: []string{"tier4-group1", "tier3-group1", "tier2-group1", "tier1-group1"}},
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
	}}})
	if err == nil {
		t.Fatal("BuildTopologyStatus() accepted a discontinuous path")
	}
}

func TestBuildTopologyStatusPreservesPreviousOnMultipleParents(t *testing.T) {
	_, err := BuildTopologyStatus(mustLabelTemplate(t), LabelSnapshot{Nodes: []LabeledResource{
		{Name: "node-a", Labels: topologyLabels("rack-a", "row-a")},
		{Name: "node-b", Labels: topologyLabels("rack-a", "row-b")},
	}})
	if err == nil {
		t.Fatal("BuildTopologyStatus() accepted multiple parents")
	}
}

func TestBuildTopologyStatusAllowsEmptySnapshot(t *testing.T) {
	result, err := BuildTopologyStatus(mustLabelTemplate(t), LabelSnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status.Domains != nil || result.Status.Nodes != nil {
		t.Fatalf("empty status = %#v", result.Status)
	}
}
