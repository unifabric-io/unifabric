// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package scenario

import (
	"github.com/unifabric-io/unifabric/e2e/check"
	"github.com/unifabric-io/unifabric/e2e/wait"
)

// roceLLDP describes the flow shared by the RoCE LLDP scenarios: wait for the
// declared Switch CRs, run scenario-specific switch checks, then validate the
// FabricNodes, Node labels and Topology/scaleout derived from them.
type roceLLDP struct {
	switchCRs     int
	switchChecks  func(r *runner) error
	topologyCheck wait.Check
	after         func(r *runner) error
}

func runRoCELLDP(r *runner, s roceLLDP) error {
	if err := r.waitCount("wait_declared_switch_crs", "switches.unifabric.io", s.switchCRs, "declared Switch CRs"); err != nil {
		return err
	}
	if err := s.switchChecks(r); err != nil {
		return err
	}
	if err := r.waitCheck(
		"check_fabricnode_interfaces",
		func() (bool, string) { return check.FabricNodeInterfaces(r.cfg.TopologyDir) },
		"fabricnode interfaces",
		"match topology yaml",
	); err != nil {
		return err
	}
	if err := r.waitCheck(
		"check_node_topology_labels",
		check.NodeTopologyLabels,
		"node topology labels",
		"tier labels match expected switch topology",
	); err != nil {
		return err
	}
	if err := r.waitCheck(
		"check_topology_status",
		s.topologyCheck,
		"scaleout Topology status",
		"domains, parents, members, nodes, and paths match",
	); err != nil {
		return err
	}
	if s.after != nil {
		return s.after(r)
	}
	return nil
}
