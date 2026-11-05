// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package scenario

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/unifabric-io/unifabric/e2e/check"
	"github.com/unifabric-io/unifabric/e2e/kube"
	"github.com/unifabric-io/unifabric/e2e/report"
	"github.com/unifabric-io/unifabric/e2e/wait"
)

var scaleUpScenarioLabels = map[string]map[string]string{
	"node-gpu-1": {
		kube.ScaleUpTier1LabelKey: "scaleup-clique-a",
	},
	"node-gpu-2": {
		kube.ScaleUpTier1LabelKey: "scaleup-clique-a",
	},
	"node-gpu-3": {
		kube.ScaleUpTier1LabelKey: "scaleup-clique-b",
	},
	"node-gpu-4": {
		kube.ScaleUpTier1LabelKey: "scaleup-clique-b",
	},
}

func runExternal(r *runner) error {
	if err := r.topographAbsent(); err != nil {
		return err
	}
	if err := r.waitCheck(
		"check_fabricnode_interfaces",
		func() (bool, string) { return check.FabricNodeInterfaces(r.cfg.TopologyDir) },
		"fabricnode interfaces",
		"agents report the simulated nodes",
	); err != nil {
		return err
	}
	return runExternalScaleUpScenario(r.cfg, r.deadline, r.rows)
}

func runExternalScaleUpScenario(_ Config, deadline time.Time, rows *[]report.Row) error {
	report.StageLog("check_external_scaleup", report.Colorize("start", report.Bold))
	if err := clearScaleUpScenarioLabels(); err != nil {
		return fmt.Errorf("clear stale scale-up scenario labels: %w", err)
	}
	defer func() {
		if err := clearScaleUpScenarioLabels(); err != nil {
			report.StageLog("cleanup_external_scaleup", report.Colorize(err.Error(), report.Red))
		}
	}()

	for nodeName, labels := range scaleUpScenarioLabels {
		for key, value := range labels {
			if _, err := kube.Run(true, "kubectl", "label", "node", nodeName, key+"="+value, "--overwrite"); err != nil {
				return fmt.Errorf("label %s for external scale-up scenario: %w", nodeName, err)
			}
		}
	}

	detail, err := wait.ForCheck(
		"check_external_scaleup",
		validateExternalScaleUpTopologyStatus,
		deadline,
		2*time.Second,
	)
	if err != nil {
		return err
	}
	*rows = append(*rows, report.Row{
		"external label aggregation",
		"one accelerator tier and two clique groups",
		detail,
		"PASS",
	})

	if err := clearScaleUpScenarioLabels(); err != nil {
		return fmt.Errorf("clear scale-up scenario labels: %w", err)
	}
	emptyDetail, err := wait.ForCheck(
		"check_empty_scaleup",
		validateEmptyScaleUpTopologyStatus,
		deadline,
		2*time.Second,
	)
	if err != nil {
		return err
	}
	*rows = append(*rows, report.Row{
		"empty label snapshot",
		"retain Topology CR and clear status",
		emptyDetail,
		"PASS",
	})
	return nil
}

func validateExternalScaleUpTopologyStatus() (bool, string) {
	var topology kube.Topology
	if err := kube.FetchJSON("topologies.unifabric.io/scaleup", &topology); err != nil {
		return false, fmt.Sprintf("failed to read Topology/scaleup: %v", err)
	}
	expectedDomains := map[string]kube.TopologyDomain{
		"scaleup-clique-a": {Name: "scaleup-clique-a", Tier: 1},
		"scaleup-clique-b": {Name: "scaleup-clique-b", Tier: 1},
	}
	expectedPaths := map[string]string{
		"scaleup-clique-a": "node-gpu-1,node-gpu-2",
		"scaleup-clique-b": "node-gpu-3,node-gpu-4",
	}
	errs := check.TopologyMatches(topology.Status, expectedDomains, expectedPaths)
	if len(errs) != 0 {
		return false, strings.Join(errs, "; ")
	}
	return true, "validated 2 accelerator domains and 2 Node groups"
}

func validateEmptyScaleUpTopologyStatus() (bool, string) {
	var topology kube.Topology
	if err := kube.FetchJSON("topologies.unifabric.io/scaleup", &topology); err != nil {
		return false, fmt.Sprintf("Topology/scaleup should remain after labels are cleared: %v", err)
	}
	if len(topology.Status.Domains) != 0 || len(topology.Status.Nodes) != 0 {
		return false, fmt.Sprintf("expected empty status, got domains=%d nodeGroups=%d", len(topology.Status.Domains), len(topology.Status.Nodes))
	}
	return true, "Topology/scaleup retained with empty domains and nodes"
}

func clearScaleUpScenarioLabels() error {
	errs := []string{}
	for nodeName, labels := range scaleUpScenarioLabels {
		for key := range labels {
			if _, err := kube.Run(false, "kubectl", "label", "node", nodeName, key+"-"); err != nil {
				errs = append(errs, fmt.Sprintf("%s:%s: %v", nodeName, key, err))
			}
		}
	}
	if len(errs) != 0 {
		sort.Strings(errs)
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}
