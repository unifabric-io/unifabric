// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package scenario

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/unifabric-io/unifabric/e2e/check"
	"github.com/unifabric-io/unifabric/e2e/kube"
	"github.com/unifabric-io/unifabric/e2e/report"
	"github.com/unifabric-io/unifabric/e2e/wait"
)

const (
	autoTopologyFinalizer = "unifabric.io/auto-discovered-topology-labels"
)

func runRoCELLDPFullAuto(r *runner) error {
	return runRoCELLDP(r, roceLLDP{
		switchCRs: 5,
		switchChecks: func(r *runner) error {
			if err := r.waitCheck(
				"check_switch_status",
				func() (bool, string) { return check.SwitchStatuses(r.cfg.TopologyDir) },
				"switch subscription status",
				"healthy switches with LLDP snapshots",
			); err != nil {
				return err
			}
			// The sFlow mock sender Job replays datagrams whose sampled packets
			// carry the macvlan test pod IPs, so Elasticsearch must hold flow
			// documents attributed to the pd-prefill/pd-decoder workloads.
			return r.waitCheck(
				"check_sflow_pipeline",
				check.SFlowPipeline,
				"sflow pipeline",
				"flows attributed to macvlan test pods in Elasticsearch",
			)
		},
		topologyCheck: check.ScaleOutTopologyStatus,
		after: func(r *runner) error {
			if err := runControllerRestartScenario(r.cfg, r.deadline, r.rows); err != nil {
				return err
			}
			return runScaleOutResetScenario(r.cfg, r.deadline, r.rows)
		},
	})
}

func runControllerRestartScenario(cfg Config, deadline time.Time, rows *[]report.Row) error {
	report.StageLog("check_controller_restart", report.Colorize("start", report.Bold))
	before, err := captureScaleOutState()
	if err != nil {
		return err
	}
	if _, err := kube.Run(
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
	if _, err := kube.Run(
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

	detail, err := wait.ForCheck(
		"check_controller_restart",
		func() (bool, string) {
			ok, statusDetail := check.ScaleOutTopologyStatus()
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
		time.Duration(cfg.SleepSeconds)*time.Second,
	)
	if err != nil {
		return err
	}
	*rows = append(*rows, report.Row{
		"controller restart",
		"preserve assigned domains and aggregate status",
		detail,
		"PASS",
	})
	return nil
}

func runScaleOutResetScenario(cfg Config, deadline time.Time, rows *[]report.Row) error {
	report.StageLog("check_scaleout_reset", report.Colorize("start", report.Bold))
	var before kube.Topology
	if err := kube.FetchJSON("topologies.unifabric.io/scaleout", &before); err != nil {
		return fmt.Errorf("capture Topology/scaleout before reset: %w", err)
	}
	if !contains(before.Metadata.Finalizers, autoTopologyFinalizer) {
		return fmt.Errorf("Topology/scaleout is missing reset finalizer %q", autoTopologyFinalizer)
	}
	if _, err := kube.Run(
		true,
		"kubectl",
		"delete",
		"topology",
		"scaleout",
		"--wait=false",
	); err != nil {
		return fmt.Errorf("delete Topology/scaleout for explicit reset: %w", err)
	}

	detail, err := wait.ForCheck(
		"check_scaleout_reset",
		func() (bool, string) {
			var after kube.Topology
			if fetchErr := kube.FetchJSON("topologies.unifabric.io/scaleout", &after); fetchErr != nil {
				return false, fmt.Sprintf("waiting for recreated Topology/scaleout: %v", fetchErr)
			}
			if after.Metadata.UID == "" || after.Metadata.UID == before.Metadata.UID {
				return false, fmt.Sprintf("waiting for a new Topology UID; current=%q", after.Metadata.UID)
			}
			if !contains(after.Metadata.Finalizers, autoTopologyFinalizer) {
				return false, "recreated Topology/scaleout is missing reset finalizer"
			}
			ok, statusDetail := check.ScaleOutTopologyStatus()
			if !ok {
				return false, statusDetail
			}
			groups, groupErr := currentScaleOutGroups()
			if groupErr != nil {
				return false, groupErr.Error()
			}
			tier1 := []string{groups.Nodes12Tier1, groups.Nodes34Tier1}
			sort.Strings(tier1)
			if !reflect.DeepEqual(tier1, []string{"tier1-group1", "tier1-group2"}) ||
				groups.Tier2 != "tier2-group1" {
				return false, fmt.Sprintf("reset did not restart allocation at group1: tier1=%v tier2=%s", tier1, groups.Tier2)
			}
			return true, fmt.Sprintf("recreated with UID %s and allocation restarted at group1", after.Metadata.UID)
		},
		deadline,
		time.Duration(cfg.SleepSeconds)*time.Second,
	)
	if err != nil {
		return err
	}
	*rows = append(*rows, report.Row{
		"Topology deletion reset",
		"clear owned labels, recreate CR, and reallocate from group1",
		detail,
		"PASS",
	})
	return nil
}

func captureScaleOutState() (string, error) {
	var nodes kube.NodeList
	if err := kube.FetchJSON("nodes", &nodes); err != nil {
		return "", fmt.Errorf("capture Kubernetes Nodes: %w", err)
	}
	var switches kube.SwitchList
	if err := kube.FetchJSON("switches.unifabric.io", &switches); err != nil {
		return "", fmt.Errorf("capture Switches: %w", err)
	}
	var topology kube.Topology
	if err := kube.FetchJSON("topologies.unifabric.io/scaleout", &topology); err != nil {
		return "", fmt.Errorf("capture Topology/scaleout: %w", err)
	}

	state := struct {
		NodeLabels    map[string]map[string]string `json:"nodeLabels"`
		SwitchDomains map[string]string            `json:"switchDomains"`
		Status        kube.TopologyStatus          `json:"status"`
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
			kube.ScaleOutTier1LabelKey: node.Metadata.Labels[kube.ScaleOutTier1LabelKey],
			kube.ScaleOutTier2LabelKey: node.Metadata.Labels[kube.ScaleOutTier2LabelKey],
			kube.ScaleOutTier3LabelKey: node.Metadata.Labels[kube.ScaleOutTier3LabelKey],
		}
	}
	for _, sw := range switches.Items {
		if strings.HasPrefix(sw.Metadata.Name, "switch-gpu-") {
			state.SwitchDomains[sw.Metadata.Name] = sw.Metadata.Labels[kube.SwitchDomainLabelKey]
		}
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return "", fmt.Errorf("encode scale-out state: %w", err)
	}
	return string(encoded), nil
}

func currentScaleOutGroups() (check.ScaleOutGroups, error) {
	var nodes kube.NodeList
	if err := kube.FetchJSON("nodes", &nodes); err != nil {
		return check.ScaleOutGroups{}, err
	}
	actualByName := make(map[string]kube.Node, len(nodes.Items))
	for _, node := range nodes.Items {
		actualByName[node.Metadata.Name] = node
	}
	groups, errs := check.ObserveScaleOutGroups(actualByName)
	if len(errs) != 0 {
		return check.ScaleOutGroups{}, fmt.Errorf("%s", strings.Join(errs, "; "))
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
