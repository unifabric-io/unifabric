// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package scenario

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/unifabric-io/unifabric/e2e/kube"
	"github.com/unifabric-io/unifabric/e2e/report"
	"github.com/unifabric-io/unifabric/e2e/wait"
)

// runner carries what every check of one scenario run needs and records the
// passed checks as result rows.
type runner struct {
	cfg      Config
	deadline time.Time
	rows     *[]report.Row
}

func (r *runner) interval() time.Duration {
	return time.Duration(r.cfg.SleepSeconds) * time.Second
}

func (r *runner) record(name, expected, actual string) {
	*r.rows = append(*r.rows, report.Row{name, expected, actual, "PASS"})
}

// waitCount waits until the number of resources reaches expected.
func (r *runner) waitCount(stage, resource string, expected int, name string) error {
	report.StageLog(stage, report.Colorize("start", report.Bold))
	actual, err := wait.ForCount(stage, resource, expected, r.deadline, r.interval())
	if err != nil {
		return err
	}
	r.record(name, strconv.Itoa(expected), strconv.Itoa(actual))
	return nil
}

// waitCheck waits until checker passes, recording its detail as the actual value.
func (r *runner) waitCheck(stage string, checker wait.Check, name, expected string) error {
	report.StageLog(stage, report.Colorize("start", report.Bold))
	detail, err := wait.ForCheck(stage, checker, r.deadline, r.interval())
	if err != nil {
		return err
	}
	r.record(name, expected, detail)
	return nil
}

func (r *runner) waitFabricNodes() error {
	return r.waitCount("wait_fabricnodes", "fabricnodes.unifabric.io", r.cfg.ExpectedFabricNodes, "fabricnodes count")
}

// topographAbsent checks that the chart did not deploy NVIDIA Topograph.
func (r *runner) topographAbsent() error {
	report.StageLog("check_topograph_absent", report.Colorize("start", report.Bold))
	detail, err := validateNvidiaTopographNotDeployed()
	if err != nil {
		return err
	}
	r.record("NVIDIA Topograph", "not deployed by the chart", detail)
	return nil
}

func validateNvidiaTopographNotDeployed() (string, error) {
	resources := []string{
		"deployment/unifabric-nvidia-node-observer",
		"deployment/unifabric-nvidia-topograph",
		"daemonset/unifabric-nvidia-node-data-broker",
	}
	for _, resource := range resources {
		stdout, err := kube.Run(
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
