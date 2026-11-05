// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/unifabric-io/unifabric/e2e/report"
	"github.com/unifabric-io/unifabric/e2e/scenario"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg := parseFlags()
	deadline := time.Now().Add(time.Duration(cfg.TimeoutMinutes) * time.Minute)
	rows := []report.Row{}

	if err := scenario.Run(cfg, deadline, &rows); err != nil {
		rows = append(rows, report.Row{"overall", "all stages pass", err.Error(), "FAIL"})
		fmt.Println(report.RenderTable(report.Row{"Check", "Expected", "Actual", "Result"}, rows))
		fmt.Println(report.Colorize("E2E checks failed.", report.Red))
		return 1
	}

	fmt.Println(report.RenderTable(report.Row{"Check", "Expected", "Actual", "Result"}, rows))
	fmt.Println(report.Colorize("E2E checks passed.", report.Green))
	return 0
}

func parseFlags() scenario.Config {
	cfg := scenario.Config{}
	flag.IntVar(&cfg.TimeoutMinutes, "timeout-minutes", 30, "timeout minutes for convergence checks")
	flag.IntVar(&cfg.SleepSeconds, "sleep-seconds", 20, "sleep seconds between check attempts")
	flag.IntVar(&cfg.ExpectedFabricNodes, "expected-fabricnodes", 4, "expected FabricNode resource count")
	flag.StringVar(&cfg.TopologyDir, "topology-dir", "e2e/topology", "topology directory containing node-gpu-*.yaml")
	flag.StringVar(&cfg.Scenario, "scenario", "roce-lldp-full-auto", "E2E scenario: "+strings.Join(scenario.Names(), ", "))
	flag.Parse()
	return cfg
}
