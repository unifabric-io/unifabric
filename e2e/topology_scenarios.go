// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"
)

const (
	defaultScaleUpTier1LabelKey = "scale-up.unifabric.io/tier-1"
	autoTopologyFinalizer       = "unifabric.io/auto-discovered-topology-labels"
	switchNeighborsAnnotation   = "unifabric.io/switch-neighbors"
)

var scaleUpScenarioLabels = map[string]map[string]string{
	"node-gpu-1": {
		defaultScaleUpTier1LabelKey: "scaleup-clique-a",
	},
	"node-gpu-2": {
		defaultScaleUpTier1LabelKey: "scaleup-clique-a",
	},
	"node-gpu-3": {
		defaultScaleUpTier1LabelKey: "scaleup-clique-b",
	},
	"node-gpu-4": {
		defaultScaleUpTier1LabelKey: "scaleup-clique-b",
	},
}

func runManualScaleUpScenario(_ config, deadline time.Time, rows *[]row) error {
	stageLog("check_manual_scaleup", colorize("start", colorBold))
	if err := clearScaleUpScenarioLabels(); err != nil {
		return fmt.Errorf("clear stale scale-up scenario labels: %w", err)
	}
	defer func() {
		if err := clearScaleUpScenarioLabels(); err != nil {
			stageLog("cleanup_manual_scaleup", colorize(err.Error(), colorRed))
		}
	}()

	for nodeName, labels := range scaleUpScenarioLabels {
		for key, value := range labels {
			if _, err := runCommand(true, "kubectl", "label", "node", nodeName, key+"="+value, "--overwrite"); err != nil {
				return fmt.Errorf("label %s for manual scale-up scenario: %w", nodeName, err)
			}
		}
	}

	detail, err := waitForCheckPass(
		"check_manual_scaleup",
		validateManualScaleUpTopologyStatus,
		deadline,
		2*time.Second,
	)
	if err != nil {
		return err
	}
	*rows = append(*rows, row{
		"nv-topograph label aggregation",
		"one accelerator tier and two clique groups",
		detail,
		"PASS",
	})

	if err := clearScaleUpScenarioLabels(); err != nil {
		return fmt.Errorf("clear scale-up scenario labels: %w", err)
	}
	emptyDetail, err := waitForCheckPass(
		"check_empty_scaleup",
		validateEmptyScaleUpTopologyStatus,
		deadline,
		2*time.Second,
	)
	if err != nil {
		return err
	}
	*rows = append(*rows, row{
		"empty label snapshot",
		"retain Topology CR and clear status",
		emptyDetail,
		"PASS",
	})
	return nil
}

func validateRoCELLDPNodeOnlySwitchInputs() (bool, string) {
	expectedNeighbors := map[string][]string{
		"switch-gpu-spine1": {
			"switch-gpu-leaf1",
			"switch-gpu-leaf2",
			"switch-gpu-leaf3",
			"switch-gpu-leaf4",
		},
	}
	syntheticLeaves := map[string]bool{
		"switch-gpu-leaf1": true,
		"switch-gpu-leaf2": true,
		"switch-gpu-leaf3": true,
		"switch-gpu-leaf4": true,
	}
	var switches switchList
	if err := fetchResourceJSON("switches.unifabric.io", &switches); err != nil {
		return false, fmt.Sprintf("failed to read Switch resources: %v", err)
	}
	actualByName := make(map[string]switchResource, len(switches.Items))
	for _, sw := range switches.Items {
		actualByName[sw.Metadata.Name] = sw
	}

	errs := []string{}
	for name, expected := range expectedNeighbors {
		sw, ok := actualByName[name]
		if !ok {
			errs = append(errs, fmt.Sprintf("%s: Switch missing", name))
			continue
		}
		if sw.Spec.MgmtIP != "" {
			errs = append(errs, fmt.Sprintf("%s: spec.mgmtIP must be empty in roce-lldp-node-only mode", name))
		}
		if sw.Spec.Role != "ScaleOut" {
			errs = append(errs, fmt.Sprintf("%s: spec.role expected ScaleOut, got %q", name, sw.Spec.Role))
		}
		raw, exists := sw.Metadata.Annotations[switchNeighborsAnnotation]
		if !exists {
			errs = append(errs, fmt.Sprintf("%s: annotation %s missing", name, switchNeighborsAnnotation))
			continue
		}
		var neighbors []string
		if err := json.Unmarshal([]byte(raw), &neighbors); err != nil {
			errs = append(errs, fmt.Sprintf("%s: invalid neighbor annotation: %v", name, err))
			continue
		}
		sort.Strings(neighbors)
		sortedExpected := append([]string(nil), expected...)
		sort.Strings(sortedExpected)
		if !slices.Equal(neighbors, sortedExpected) {
			errs = append(errs, fmt.Sprintf("%s: expected neighbors %v, got %v", name, sortedExpected, neighbors))
		}
		for _, neighbor := range neighbors {
			if !syntheticLeaves[neighbor] {
				errs = append(errs, fmt.Sprintf("%s: annotation contains an unexpected synthetic leaf %q", name, neighbor))
			}
		}
	}
	if len(errs) != 0 {
		sort.Strings(errs)
		return false, strings.Join(errs, "; ")
	}
	return true, "validated one spine Switch referencing four node-discovered synthetic leaves"
}

func validateNvidiaTopographNotDeployed() (string, error) {
	resources := []string{
		"deployment/unifabric-nvidia-node-observer",
		"deployment/unifabric-nvidia-topograph",
		"daemonset/unifabric-nvidia-node-data-broker",
	}
	for _, resource := range resources {
		stdout, err := runCommand(
			true,
			"kubectl",
			"-n",
			"unifabric-system",
			"get",
			resource,
			"--ignore-not-found",
			"-o",
			"name",
		)
		if err != nil {
			return "", fmt.Errorf("check NVIDIA Topograph resource %s: %w", resource, err)
		}
		if strings.TrimSpace(stdout) != "" {
			return "", fmt.Errorf("NVIDIA Topograph resource unexpectedly exists: %s", strings.TrimSpace(stdout))
		}
	}
	return "Topograph, node-observer, and node-data-broker are absent", nil
}

func runControllerRestartScenario(cfg config, deadline time.Time, rows *[]row) error {
	stageLog("check_controller_restart", colorize("start", colorBold))
	before, err := captureScaleOutState()
	if err != nil {
		return err
	}
	if _, err := runCommand(
		true,
		"kubectl",
		"-n",
		"unifabric-system",
		"rollout",
		"restart",
		"deployment/unifabric-controller",
	); err != nil {
		return fmt.Errorf("restart controller: %w", err)
	}
	if _, err := runCommand(
		true,
		"kubectl",
		"-n",
		"unifabric-system",
		"rollout",
		"status",
		"deployment/unifabric-controller",
		"--timeout=5m",
	); err != nil {
		return fmt.Errorf("wait for controller restart: %w", err)
	}

	detail, err := waitForCheckPass(
		"check_controller_restart",
		func() (bool, string) {
			ok, statusDetail := validateScaleOutTopologyStatus()
			if !ok {
				return false, statusDetail
			}
			after, captureErr := captureScaleOutState()
			if captureErr != nil {
				return false, captureErr.Error()
			}
			if before != after {
				return false, "managed Node/Switch labels or Topology status changed after restart"
			}
			return true, "managed labels and Topology status remained byte-for-byte stable"
		},
		deadline,
		time.Duration(cfg.sleepSeconds)*time.Second,
	)
	if err != nil {
		return err
	}
	*rows = append(*rows, row{
		"controller restart",
		"preserve assigned domains and aggregate status",
		detail,
		"PASS",
	})
	return nil
}

func runScaleOutResetScenario(cfg config, deadline time.Time, rows *[]row) error {
	stageLog("check_scaleout_reset", colorize("start", colorBold))
	var before topologyResource
	if err := fetchResourceJSON("topologies.unifabric.io/scaleout", &before); err != nil {
		return fmt.Errorf("capture Topology/scaleout before reset: %w", err)
	}
	if !contains(before.Metadata.Finalizers, autoTopologyFinalizer) {
		return fmt.Errorf("Topology/scaleout is missing reset finalizer %q", autoTopologyFinalizer)
	}
	if _, err := runCommand(
		true,
		"kubectl",
		"delete",
		"topology",
		"scaleout",
		"--wait=false",
	); err != nil {
		return fmt.Errorf("delete Topology/scaleout for explicit reset: %w", err)
	}

	detail, err := waitForCheckPass(
		"check_scaleout_reset",
		func() (bool, string) {
			var after topologyResource
			if fetchErr := fetchResourceJSON("topologies.unifabric.io/scaleout", &after); fetchErr != nil {
				return false, fmt.Sprintf("waiting for recreated Topology/scaleout: %v", fetchErr)
			}
			if after.Metadata.UID == "" || after.Metadata.UID == before.Metadata.UID {
				return false, fmt.Sprintf("waiting for a new Topology UID; current=%q", after.Metadata.UID)
			}
			if !contains(after.Metadata.Finalizers, autoTopologyFinalizer) {
				return false, "recreated Topology/scaleout is missing reset finalizer"
			}
			ok, statusDetail := validateScaleOutTopologyStatus()
			if !ok {
				return false, statusDetail
			}
			groups, groupErr := currentScaleOutGroups()
			if groupErr != nil {
				return false, groupErr.Error()
			}
			tier1 := []string{groups.nodes12Tier1, groups.nodes34Tier1}
			sort.Strings(tier1)
			if !reflect.DeepEqual(tier1, []string{"tier1-group1", "tier1-group2"}) ||
				groups.tier2 != "tier2-group1" {
				return false, fmt.Sprintf("reset did not restart allocation at group1: tier1=%v tier2=%s", tier1, groups.tier2)
			}
			return true, fmt.Sprintf("recreated with UID %s and allocation restarted at group1", after.Metadata.UID)
		},
		deadline,
		time.Duration(cfg.sleepSeconds)*time.Second,
	)
	if err != nil {
		return err
	}
	*rows = append(*rows, row{
		"Topology deletion reset",
		"clear owned labels, recreate CR, and reallocate from group1",
		detail,
		"PASS",
	})
	return nil
}

func validateManualScaleUpTopologyStatus() (bool, string) {
	var topology topologyResource
	if err := fetchResourceJSON("topologies.unifabric.io/scaleup", &topology); err != nil {
		return false, fmt.Sprintf("failed to read Topology/scaleup: %v", err)
	}
	expectedDomains := map[string]topologyDomain{
		"scaleup-clique-a": {Name: "scaleup-clique-a", Tier: 1},
		"scaleup-clique-b": {Name: "scaleup-clique-b", Tier: 1},
	}
	expectedPaths := map[string]string{
		"scaleup-clique-a": "node-gpu-1,node-gpu-2",
		"scaleup-clique-b": "node-gpu-3,node-gpu-4",
	}
	errs := validateTopologyStatus(topology.Status, expectedDomains, expectedPaths)
	if len(errs) != 0 {
		return false, strings.Join(errs, "; ")
	}
	return true, "validated 2 accelerator domains and 2 Node groups"
}

func validateEmptyScaleUpTopologyStatus() (bool, string) {
	var topology topologyResource
	if err := fetchResourceJSON("topologies.unifabric.io/scaleup", &topology); err != nil {
		return false, fmt.Sprintf("Topology/scaleup should remain after labels are cleared: %v", err)
	}
	if len(topology.Status.Domains) != 0 || len(topology.Status.Nodes) != 0 {
		return false, fmt.Sprintf("expected empty status, got domains=%d nodeGroups=%d", len(topology.Status.Domains), len(topology.Status.Nodes))
	}
	return true, "Topology/scaleup retained with empty domains and nodes"
}

func validateTopologyStatus(
	status topologyStatus,
	expectedDomains map[string]topologyDomain,
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
			!reflect.DeepEqual(sortedUnique(domain.Members), sortedUnique(expected.Members)) {
			errs = append(errs, fmt.Sprintf(
				"domain %s expected tier=%d parent=%s members=%v, got tier=%d parent=%s members=%v",
				domain.Name,
				expected.Tier,
				expected.Parent,
				expected.Members,
				domain.Tier,
				domain.Parent,
				domain.Members,
			))
		}
	}
	if len(status.Nodes) != len(expectedPaths) {
		errs = append(errs, fmt.Sprintf("node group count expected %d, got %d", len(expectedPaths), len(status.Nodes)))
	}
	for _, nodeGroup := range status.Nodes {
		path := strings.Join(nodeGroup.DomainPath, "/")
		expectedNodes, ok := expectedPaths[path]
		if !ok {
			errs = append(errs, fmt.Sprintf("unexpected Node path %s", path))
			continue
		}
		if actualNodes := strings.Join(sortedUnique(nodeGroup.Nodes), ","); actualNodes != expectedNodes {
			errs = append(errs, fmt.Sprintf("path %s expected nodes %s, got %s", path, expectedNodes, actualNodes))
		}
	}
	sort.Strings(errs)
	return errs
}

func clearScaleUpScenarioLabels() error {
	errs := []string{}
	for nodeName, labels := range scaleUpScenarioLabels {
		for key := range labels {
			if _, err := runCommand(false, "kubectl", "label", "node", nodeName, key+"-"); err != nil {
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

func captureScaleOutState() (string, error) {
	var nodes kubernetesNodeList
	if err := fetchResourceJSON("nodes", &nodes); err != nil {
		return "", fmt.Errorf("capture Kubernetes Nodes: %w", err)
	}
	var switches switchList
	if err := fetchResourceJSON("switches.unifabric.io", &switches); err != nil {
		return "", fmt.Errorf("capture Switches: %w", err)
	}
	var topology topologyResource
	if err := fetchResourceJSON("topologies.unifabric.io/scaleout", &topology); err != nil {
		return "", fmt.Errorf("capture Topology/scaleout: %w", err)
	}

	state := struct {
		NodeLabels    map[string]map[string]string `json:"nodeLabels"`
		SwitchDomains map[string]string            `json:"switchDomains"`
		Status        topologyStatus               `json:"status"`
	}{
		NodeLabels:    map[string]map[string]string{},
		SwitchDomains: map[string]string{},
		Status:        topology.Status,
	}
	for _, node := range nodes.Items {
		if !strings.HasPrefix(node.Metadata.Name, "node-gpu-") {
			continue
		}
		state.NodeLabels[node.Metadata.Name] = map[string]string{
			defaultScaleOutTier1LabelKey: node.Metadata.Labels[defaultScaleOutTier1LabelKey],
			defaultScaleOutTier2LabelKey: node.Metadata.Labels[defaultScaleOutTier2LabelKey],
			defaultScaleOutTier3LabelKey: node.Metadata.Labels[defaultScaleOutTier3LabelKey],
		}
	}
	for _, sw := range switches.Items {
		if strings.HasPrefix(sw.Metadata.Name, "switch-gpu-") {
			state.SwitchDomains[sw.Metadata.Name] = sw.Metadata.Labels[switchDomainLabelKey]
		}
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("encode scale-out state: %w", err)
	}
	return string(encoded), nil
}

func currentScaleOutGroups() (observedScaleOutGroups, error) {
	var nodes kubernetesNodeList
	if err := fetchResourceJSON("nodes", &nodes); err != nil {
		return observedScaleOutGroups{}, err
	}
	actualByName := make(map[string]kubernetesNode, len(nodes.Items))
	for _, node := range nodes.Items {
		actualByName[node.Metadata.Name] = node
	}
	groups, errs := observeScaleOutGroups(actualByName)
	if len(errs) != 0 {
		return observedScaleOutGroups{}, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return groups, nil
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
