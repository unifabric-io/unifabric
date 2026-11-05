// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package scenario

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/unifabric-io/unifabric/e2e/check"
	"github.com/unifabric-io/unifabric/e2e/kube"
	"github.com/unifabric-io/unifabric/e2e/report"
)

const (
	ibModelNodesManifest = "script/ib-model-nodes.yaml"
	ibManualSwitchFile   = "script/ib-manual-switch.yaml"
	ibManualSwitchName   = "e2e-manual-switch"
	ibDiscoveredByLabel  = "unifabric.io/discovered-by"
	ibDiscoveredByValue  = "unifabric-agent"
	ibModelNodeSelector  = "e2e.unifabric.io/topograph-model=true"
)

// ibLeafHosts maps each leaf in e2e/topology/ib-sim/small.ibnetdiscover to
// the hosts it serves. Dual leaves (sw1101/sw1102 and sw1103/sw1104) serve the
// same hosts, so each pair must share the hosts' tier-1 domain.
var ibLeafHosts = map[string][]string{
	"sw1101": {"1101", "1102"},
	"sw1102": {"1101", "1102"},
	"sw1103": {"1103", "1104"},
	"sw1104": {"1103", "1104"},
}

var ibSpines = []string{"sw2101", "sw2102"}

// runIBSwitchDiscovery validates unifabric-ib-with-topograph mode: the agent
// creates Switch resources from an ibnetdiscover replay, Topograph labels the
// model Nodes from the same file, and the controller copies those Node labels
// onto the Switches so Topology/scaleout contains the IB switches.
//
// It is meant for local kind runs prepared with
// "setup.sh step-install-ib-switch-discovery".
func runIBSwitchDiscovery(r *runner) error {
	nodes := filepath.Join(r.cfg.TopologyDir, ibModelNodesManifest)
	manual := filepath.Join(r.cfg.TopologyDir, ibManualSwitchFile)

	cleanup := func() error {
		if _, err := kube.Run(true, "kubectl", "delete", "node", "-l", ibModelNodeSelector, "--ignore-not-found"); err != nil {
			return err
		}
		_, err := kube.Run(true, "kubectl", "delete", "switches.unifabric.io", ibManualSwitchName, "--ignore-not-found")
		return err
	}
	if err := cleanup(); err != nil {
		return fmt.Errorf("clear stale IB scenario resources: %w", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			report.StageLog("cleanup_ib_switch_discovery", report.Colorize(err.Error(), report.Red))
		}
	}()

	if err := r.waitCheck("check_ib_switches_created", validateIBSwitchesCreated,
		"agent-created Switches", "4 leaves and 2 spines created from ibnetdiscover"); err != nil {
		return err
	}

	for _, manifest := range []string{nodes, manual} {
		if _, err := kube.Run(true, "kubectl", "apply", "-f", manifest); err != nil {
			return fmt.Errorf("apply %s: %w", manifest, err)
		}
	}

	if err := r.waitCheck("check_ib_node_labels", validateIBNodeLabels,
		"Topograph label write-back", "dual-leaf hosts share one tier-1 domain under one tier-2 domain"); err != nil {
		return err
	}
	if err := r.waitCheck("check_ib_scaleup_topology", validateIBScaleUpTopology,
		"Topology/scaleup", "nv-gpu-operator mode: two gpu.clique domains of two nodes"); err != nil {
		return err
	}
	if err := r.waitCheck("check_ib_scaleout_topology", validateIBScaleOutTopology,
		"Topology/scaleout", "two tier-1 domains, one tier-2 domain, two node groups"); err != nil {
		return err
	}
	if err := r.waitCheck("check_ib_manual_switch", validateIBManualSwitchUntouched,
		"manual Switch", "left untouched by discovery and domain labeling"); err != nil {
		return err
	}

	if err := cleanup(); err != nil {
		return fmt.Errorf("delete IB model nodes: %w", err)
	}
	return r.waitCheck("check_ib_cleanup", validateIBTopologyDrained,
		"model node removal", "Topology/scaleout drops the removed nodes")
}

func fetchIBSwitches() (map[string]kube.Switch, error) {
	var list kube.SwitchList
	if err := kube.FetchJSON("switches.unifabric.io", &list); err != nil {
		return nil, fmt.Errorf("failed to read Switches: %w", err)
	}
	byName := make(map[string]kube.Switch, len(list.Items))
	for _, sw := range list.Items {
		byName[sw.Metadata.Name] = sw
	}
	return byName, nil
}

func validateIBSwitchesCreated() (bool, string) {
	switches, err := fetchIBSwitches()
	if err != nil {
		return false, err.Error()
	}
	var missing []string
	for name := range ibLeafHosts {
		missing = appendIfNotDiscovered(missing, switches, name)
	}
	for _, name := range ibSpines {
		missing = appendIfNotDiscovered(missing, switches, name)
	}
	if len(missing) != 0 {
		sort.Strings(missing)
		return false, "waiting for agent-created Switches: " + strings.Join(missing, ",")
	}
	return true, fmt.Sprintf("%d Switches created by the agent", len(ibLeafHosts)+len(ibSpines))
}

func appendIfNotDiscovered(missing []string, switches map[string]kube.Switch, name string) []string {
	sw, ok := switches[name]
	if !ok || sw.Metadata.Labels[ibDiscoveredByLabel] != ibDiscoveredByValue {
		return append(missing, name)
	}
	return missing
}

// ibNodeDomains returns the tier-1 domain of each leaf host group and the
// single tier-2 domain, or an error describing what is not yet consistent.
func ibNodeDomains() (map[string]string, string, error) {
	var nodes kube.NodeList
	if err := kube.FetchJSON("nodes", &nodes); err != nil {
		return nil, "", fmt.Errorf("failed to read nodes: %w", err)
	}
	labels := make(map[string]map[string]string, len(nodes.Items))
	for _, n := range nodes.Items {
		labels[n.Metadata.Name] = n.Metadata.Labels
	}

	tier1ByHost := map[string]string{}
	tier2 := map[string]struct{}{}
	for _, hosts := range ibLeafHosts {
		for _, host := range hosts {
			l, ok := labels[host]
			if !ok {
				return nil, "", fmt.Errorf("node %s not found", host)
			}
			t1, t2 := l[kube.ScaleOutTier1LabelKey], l[kube.ScaleOutTier2LabelKey]
			if t1 == "" || t2 == "" {
				return nil, "", fmt.Errorf("node %s waiting for tier-1/tier-2 labels (got %q, %q)", host, t1, t2)
			}
			tier1ByHost[host] = t1
			tier2[t2] = struct{}{}
		}
	}
	if len(tier2) != 1 {
		return nil, "", fmt.Errorf("expected one tier-2 domain, got %d", len(tier2))
	}
	var spine string
	for t2 := range tier2 {
		spine = t2
	}
	if tier1ByHost["1101"] != tier1ByHost["1102"] || tier1ByHost["1103"] != tier1ByHost["1104"] {
		return nil, "", fmt.Errorf("dual-leaf hosts must share a tier-1 domain: %v", tier1ByHost)
	}
	if tier1ByHost["1101"] == tier1ByHost["1103"] {
		return nil, "", fmt.Errorf("leaf pairs must be different tier-1 domains: %v", tier1ByHost)
	}
	return tier1ByHost, spine, nil
}

func validateIBNodeLabels() (bool, string) {
	tier1, spine, err := ibNodeDomains()
	if err != nil {
		return false, err.Error()
	}
	return true, fmt.Sprintf("tier-1 %s and %s under tier-2 %s", tier1["1101"], tier1["1103"], spine)
}

// validateIBScaleUpTopology checks the nv-gpu-operator scale-up mode: the
// nvidia.com/gpu.clique Node label (default key) forms the tier-1 domain.
func validateIBScaleUpTopology() (bool, string) {
	var topology kube.Topology
	if err := kube.FetchJSON("topologies.unifabric.io/scaleup", &topology); err != nil {
		return false, fmt.Sprintf("failed to read Topology/scaleup: %v", err)
	}
	expectedDomains := map[string]kube.TopologyDomain{
		"e2e-clique.1": {Name: "e2e-clique.1", Tier: 1},
		"e2e-clique.2": {Name: "e2e-clique.2", Tier: 1},
	}
	expectedPaths := map[string]string{
		"e2e-clique.1": "1101,1102",
		"e2e-clique.2": "1103,1104",
	}
	if errs := check.TopologyMatches(topology.Status, expectedDomains, expectedPaths); len(errs) != 0 {
		return false, strings.Join(errs, "; ")
	}
	return true, "2 gpu.clique domains and 2 Node groups"
}

func validateIBScaleOutTopology() (bool, string) {
	tier1, spine, err := ibNodeDomains()
	if err != nil {
		return false, err.Error()
	}
	var topology kube.Topology
	if err := kube.FetchJSON("topologies.unifabric.io/scaleout", &topology); err != nil {
		return false, fmt.Sprintf("failed to read Topology/scaleout: %v", err)
	}

	wantMembers := map[string][]string{
		tier1["1101"]: {"sw1101", "sw1102"},
		tier1["1103"]: {"sw1103", "sw1104"},
		spine:         ibSpines,
	}
	if len(topology.Status.Domains) != len(wantMembers) {
		return false, fmt.Sprintf("domain count expected %d, got %d", len(wantMembers), len(topology.Status.Domains))
	}
	for _, d := range topology.Status.Domains {
		want, ok := wantMembers[d.Name]
		if !ok {
			return false, "unexpected domain " + d.Name
		}
		wantTier, wantParent := 1, spine
		if d.Name == spine {
			wantTier, wantParent = 2, ""
		}
		got := append([]string(nil), d.SwitchMember...)
		sort.Strings(got)
		if d.Tier != wantTier || d.Parent != wantParent || strings.Join(got, ",") != strings.Join(want, ",") {
			return false, fmt.Sprintf("domain %s expected tier=%d parent=%q members=%v, got tier=%d parent=%q members=%v",
				d.Name, wantTier, wantParent, want, d.Tier, d.Parent, got)
		}
	}

	wantPaths := map[string]string{
		spine + "/" + tier1["1101"]: "1101,1102",
		spine + "/" + tier1["1103"]: "1103,1104",
	}
	if len(topology.Status.Nodes) != len(wantPaths) {
		return false, fmt.Sprintf("node group count expected %d, got %d", len(wantPaths), len(topology.Status.Nodes))
	}
	for _, g := range topology.Status.Nodes {
		got := append([]string(nil), g.Nodes...)
		sort.Strings(got)
		if want := wantPaths[strings.Join(g.SwitchDomainPath, "/")]; strings.Join(got, ",") != want {
			return false, fmt.Sprintf("path %s has unexpected nodes %v", strings.Join(g.SwitchDomainPath, "/"), got)
		}
	}
	return true, fmt.Sprintf("%d domains and %d node groups", len(wantMembers), len(wantPaths))
}

func validateIBManualSwitchUntouched() (bool, string) {
	switches, err := fetchIBSwitches()
	if err != nil {
		return false, err.Error()
	}
	sw, ok := switches[ibManualSwitchName]
	if !ok {
		return false, "manual Switch was deleted"
	}
	for _, key := range []string{ibDiscoveredByLabel, kube.SwitchDomainLabelKey} {
		if v, ok := sw.Metadata.Labels[key]; ok {
			return false, fmt.Sprintf("manual Switch gained label %s=%s", key, v)
		}
	}
	return true, "still present with no discovery or domain labels"
}

func validateIBTopologyDrained() (bool, string) {
	var topology kube.Topology
	if err := kube.FetchJSON("topologies.unifabric.io/scaleout", &topology); err != nil {
		return false, fmt.Sprintf("failed to read Topology/scaleout: %v", err)
	}
	if n := len(topology.Status.Nodes); n != 0 {
		return false, fmt.Sprintf("%d node groups still present", n)
	}
	return true, "no node groups remain"
}
