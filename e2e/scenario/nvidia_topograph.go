// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package scenario

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/unifabric-io/unifabric/e2e/check"
	"github.com/unifabric-io/unifabric/e2e/kube"
	"github.com/unifabric-io/unifabric/e2e/report"
	"github.com/unifabric-io/unifabric/e2e/wait"
)

// topographModelNodeLabelKey marks the kubelet-less Node objects that mirror
// the hosts in NVIDIA Topograph's embedded "medium.yaml" test model. The
// Topograph k8s engine and node-observer trigger are scoped to this label.
const topographModelNodeLabelKey = "e2e.unifabric.io/topograph-model"

// topographModelNodeManifest is relative to the --topology-dir directory.
const topographModelNodeManifest = "script/topograph-model-nodes.yaml"

type topographModelExpectation struct {
	scaleUp string
	tier1   string
	tier2   string
	tier3   string
}

// topographModelNodes mirrors the embedded "medium.yaml" model shipped in
// NVIDIA Topograph: hosts 1101..1402 under leaves sw11..sw14, spines
// sw21/sw22, core sw3, with accelerator domains nvl1..nvl4.
var topographModelNodes = map[string]topographModelExpectation{
	"1101": {scaleUp: "nvl1", tier1: "sw11", tier2: "sw21", tier3: "sw3"},
	"1102": {scaleUp: "nvl1", tier1: "sw11", tier2: "sw21", tier3: "sw3"},
	"1201": {scaleUp: "nvl2", tier1: "sw12", tier2: "sw21", tier3: "sw3"},
	"1202": {scaleUp: "nvl2", tier1: "sw12", tier2: "sw21", tier3: "sw3"},
	"1301": {scaleUp: "nvl3", tier1: "sw13", tier2: "sw22", tier3: "sw3"},
	"1302": {scaleUp: "nvl3", tier1: "sw13", tier2: "sw22", tier3: "sw3"},
	"1401": {scaleUp: "nvl4", tier1: "sw14", tier2: "sw22", tier3: "sw3"},
	"1402": {scaleUp: "nvl4", tier1: "sw14", tier2: "sw22", tier3: "sw3"},
}

func runNvidiaTopograph(r *runner) error {
	if err := r.topographAbsent(); err != nil {
		return err
	}
	return runNvidiaTopographScenario(r.cfg, r.deadline, r.rows)
}

// runNvidiaTopographScenario validates the closed loop between a real
// upstream NVIDIA Topograph deployment and Unifabric: Topograph computes the
// embedded test model, writes real Node labels through its k8s engine, and
// Unifabric aggregates those labels into Topology/scaleup and
// Topology/scaleout.
func runNvidiaTopographScenario(cfg Config, deadline time.Time, rows *[]report.Row) error {
	report.StageLog("check_nvidia_topograph", report.Colorize("start", report.Bold))
	interval := time.Duration(cfg.SleepSeconds) * time.Second
	manifest := filepath.Join(cfg.TopologyDir, topographModelNodeManifest)

	if err := deleteTopographModelNodes(); err != nil {
		return fmt.Errorf("clear stale Topograph model nodes: %w", err)
	}
	defer func() {
		if err := deleteTopographModelNodes(); err != nil {
			report.StageLog("cleanup_nvidia_topograph", report.Colorize(err.Error(), report.Red))
		}
	}()

	if _, err := kube.Run(true, "kubectl", "apply", "-f", manifest); err != nil {
		return fmt.Errorf("create Topograph model nodes: %w", err)
	}

	labelDetail, err := wait.ForCheck(
		"check_topograph_node_labels",
		validateTopographModelNodeLabels,
		deadline,
		interval,
	)
	if err != nil {
		return err
	}
	*rows = append(*rows, report.Row{
		"Topograph label write-back",
		"real Topograph labels the model nodes",
		labelDetail,
		"PASS",
	})

	scaleUpDetail, err := wait.ForCheck(
		"check_topograph_scaleup",
		validateTopographScaleUpTopologyStatus,
		deadline,
		interval,
	)
	if err != nil {
		return err
	}
	*rows = append(*rows, report.Row{
		"Topology/scaleup from Topograph",
		"four nvl domains with two nodes each",
		scaleUpDetail,
		"PASS",
	})

	scaleOutDetail, err := wait.ForCheck(
		"check_topograph_scaleout",
		validateTopographScaleOutTopologyStatus,
		deadline,
		interval,
	)
	if err != nil {
		return err
	}
	*rows = append(*rows, report.Row{
		"Topology/scaleout from Topograph",
		"three-tier switch tree and four node groups",
		scaleOutDetail,
		"PASS",
	})

	if err := deleteTopographModelNodes(); err != nil {
		return fmt.Errorf("delete Topograph model nodes: %w", err)
	}
	cleanupDetail, err := wait.ForCheck(
		"check_topograph_cleanup",
		validateTopographTopologyStatusesEmpty,
		deadline,
		interval,
	)
	if err != nil {
		return err
	}
	*rows = append(*rows, report.Row{
		"model node removal",
		"Topology statuses drain after node deletion",
		cleanupDetail,
		"PASS",
	})
	return nil
}

func deleteTopographModelNodes() error {
	if _, err := kube.Run(
		true,
		"kubectl",
		"delete",
		"node",
		"-l",
		topographModelNodeLabelKey+"=true",
		"--ignore-not-found",
	); err != nil {
		return fmt.Errorf("delete nodes labeled %s: %w", topographModelNodeLabelKey, err)
	}
	return nil
}

func validateTopographModelNodeLabels() (bool, string) {
	var nodes kube.NodeList
	if err := kube.FetchJSON("nodes", &nodes); err != nil {
		return false, fmt.Sprintf("failed to read kubernetes nodes: %v", err)
	}
	actualByName := make(map[string]kube.Node, len(nodes.Items))
	for _, node := range nodes.Items {
		actualByName[node.Metadata.Name] = node
	}

	errs := []string{}
	for nodeName, expected := range topographModelNodes {
		node, ok := actualByName[nodeName]
		if !ok {
			errs = append(errs, fmt.Sprintf("%s: model node missing", nodeName))
			continue
		}
		expectedLabels := map[string]string{
			kube.ScaleUpTier1LabelKey:  expected.scaleUp,
			kube.ScaleOutTier1LabelKey: expected.tier1,
			kube.ScaleOutTier2LabelKey: expected.tier2,
			kube.ScaleOutTier3LabelKey: expected.tier3,
		}
		for key, expectedValue := range expectedLabels {
			if actual := node.Metadata.Labels[key]; actual != expectedValue {
				errs = append(errs, fmt.Sprintf("%s: label %s expected %q, got %q", nodeName, key, expectedValue, actual))
			}
		}
	}
	if len(errs) != 0 {
		sort.Strings(errs)
		return false, strings.Join(errs, "; ")
	}
	return true, fmt.Sprintf("Topograph labeled %d model nodes with scale-up and scale-out tiers", len(topographModelNodes))
}

func validateTopographScaleUpTopologyStatus() (bool, string) {
	var topology kube.Topology
	if err := kube.FetchJSON("topologies.unifabric.io/scaleup", &topology); err != nil {
		return false, fmt.Sprintf("failed to read Topology/scaleup: %v", err)
	}
	expectedDomains := map[string]kube.TopologyDomain{
		"nvl1": {Name: "nvl1", Tier: 1},
		"nvl2": {Name: "nvl2", Tier: 1},
		"nvl3": {Name: "nvl3", Tier: 1},
		"nvl4": {Name: "nvl4", Tier: 1},
	}
	expectedPaths := map[string]string{
		"nvl1": "1101,1102",
		"nvl2": "1201,1202",
		"nvl3": "1301,1302",
		"nvl4": "1401,1402",
	}
	errs := check.TopologyMatches(topology.Status, expectedDomains, expectedPaths)
	if len(errs) != 0 {
		return false, strings.Join(errs, "; ")
	}
	return true, "validated 4 accelerator domains and 4 Node groups"
}

func validateTopographScaleOutTopologyStatus() (bool, string) {
	var topology kube.Topology
	if err := kube.FetchJSON("topologies.unifabric.io/scaleout", &topology); err != nil {
		return false, fmt.Sprintf("failed to read Topology/scaleout: %v", err)
	}
	expectedDomains := map[string]kube.TopologyDomain{
		"sw11": {Name: "sw11", Tier: 1, Parent: "sw21"},
		"sw12": {Name: "sw12", Tier: 1, Parent: "sw21"},
		"sw13": {Name: "sw13", Tier: 1, Parent: "sw22"},
		"sw14": {Name: "sw14", Tier: 1, Parent: "sw22"},
		"sw21": {Name: "sw21", Tier: 2, Parent: "sw3"},
		"sw22": {Name: "sw22", Tier: 2, Parent: "sw3"},
		"sw3":  {Name: "sw3", Tier: 3},
	}
	expectedPaths := map[string]string{
		"sw3/sw21/sw11": "1101,1102",
		"sw3/sw21/sw12": "1201,1202",
		"sw3/sw22/sw13": "1301,1302",
		"sw3/sw22/sw14": "1401,1402",
	}
	errs := check.TopologyMatches(topology.Status, expectedDomains, expectedPaths)
	if len(errs) != 0 {
		return false, strings.Join(errs, "; ")
	}
	return true, "validated 7 switch domains across 3 tiers and 4 Node groups"
}

func validateTopographTopologyStatusesEmpty() (bool, string) {
	for _, name := range []string{"scaleup", "scaleout"} {
		var topology kube.Topology
		if err := kube.FetchJSON("topologies.unifabric.io/"+name, &topology); err != nil {
			return false, fmt.Sprintf("Topology/%s should remain after model nodes are deleted: %v", name, err)
		}
		if len(topology.Status.Domains) != 0 || len(topology.Status.Nodes) != 0 {
			return false, fmt.Sprintf(
				"Topology/%s expected empty status, got domains=%d nodeGroups=%d",
				name,
				len(topology.Status.Domains),
				len(topology.Status.Nodes),
			)
		}
	}
	return true, "Topology/scaleup and Topology/scaleout retained with empty statuses"
}
