// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

// Package scenario holds the E2E scenarios selectable with --scenario. Each
// scenario lives in its own file and is registered in scenarios below.
package scenario

import (
	"fmt"
	"strings"
	"time"

	"github.com/unifabric-io/unifabric/e2e/report"
)

// Config holds the command-line settings shared by all scenarios.
type Config struct {
	TimeoutMinutes      int
	SleepSeconds        int
	ExpectedFabricNodes int
	TopologyDir         string
	Scenario            string
}

var scenarios = []struct {
	name string
	run  func(r *runner) error
	// skipFabricNodes is set for scenarios that run without the RoCE FabricNode agents.
	skipFabricNodes bool
}{
	{"roce-lldp-full-auto", runRoCELLDPFullAuto, false},
	{"roce-lldp-node-only", runRoCELLDPNodeOnly, false},
	{"external", runExternal, false},
	{"nvidia-topograph", runNvidiaTopograph, false},
	{"ib-switch-discovery", runIBSwitchDiscovery, true},
}

// Names returns the scenario names accepted by --scenario.
func Names() []string {
	names := make([]string, 0, len(scenarios))
	for _, s := range scenarios {
		names = append(names, s.name)
	}
	return names
}

// Run executes the scenario named by cfg.Scenario and appends one row per
// passed check to rows.
func Run(cfg Config, deadline time.Time, rows *[]report.Row) error {
	for _, s := range scenarios {
		if s.name != cfg.Scenario {
			continue
		}
		r := &runner{cfg: cfg, deadline: deadline, rows: rows}
		if !s.skipFabricNodes {
			if err := r.waitFabricNodes(); err != nil {
				return err
			}
		}
		return s.run(r)
	}
	return fmt.Errorf("unsupported E2E scenario %q: expected one of %s", cfg.Scenario, strings.Join(Names(), ", "))
}
