// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package check

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/unifabric-io/unifabric/e2e/kube"
)

func loadExpectedScaleOutSwitches(topologyDir string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(topologyDir, "switch-gpu-*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	result := make([]string, 0, len(paths))
	for _, path := range paths {
		result = append(result, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	}
	return result, nil
}

func SwitchStatuses(topologyDir string) (bool, string) {
	expectedNames, err := loadExpectedScaleOutSwitches(topologyDir)
	if err != nil {
		return false, fmt.Sprintf("failed to load expected switches: %v", err)
	}
	if len(expectedNames) == 0 {
		return false, fmt.Sprintf("no switch-gpu-*.yaml found under %s", topologyDir)
	}

	var switches kube.SwitchList
	if err := kube.FetchJSON("switches.unifabric.io", &switches); err != nil {
		return false, fmt.Sprintf("failed to read switches: %v", err)
	}

	actualByName := make(map[string]kube.Switch, len(switches.Items))
	for _, item := range switches.Items {
		actualByName[item.Metadata.Name] = item
	}

	errs := []string{}
	for _, name := range expectedNames {
		sw, ok := actualByName[name]
		if !ok {
			errs = append(errs, fmt.Sprintf("%s: Switch missing", name))
			continue
		}
		if !sw.Status.Healthy {
			errs = append(errs, fmt.Sprintf("%s: switch not healthy", name))
		}
		if strings.TrimSpace(sw.Status.Hostname) == "" {
			errs = append(errs, fmt.Sprintf("%s: status.hostname empty", name))
		}
		if sw.Status.LLDPNeighborCount <= 0 {
			errs = append(errs, fmt.Sprintf("%s: lldpNeighborCount=%d", name, sw.Status.LLDPNeighborCount))
		}
	}

	if len(errs) > 0 {
		sort.Strings(errs)
		return false, strings.Join(errs, "; ")
	}

	return true, fmt.Sprintf("validated %d switches with hostname and LLDP status", len(expectedNames))
}

func validateSwitchDomainLabels(switches []kube.Switch, expectedDomains map[string]string) []string {
	actualByName := make(map[string]kube.Switch, len(switches))
	for _, sw := range switches {
		actualByName[sw.Metadata.Name] = sw
	}

	errs := []string{}
	for name, expectedDomain := range expectedDomains {
		sw, ok := actualByName[name]
		if !ok {
			errs = append(errs, fmt.Sprintf("Switch %s missing", name))
			continue
		}
		if actualDomain := sw.Metadata.Labels[kube.SwitchDomainLabelKey]; actualDomain != expectedDomain {
			errs = append(errs, fmt.Sprintf("Switch %s label %s expected %s, got %s", name, kube.SwitchDomainLabelKey, expectedDomain, actualDomain))
		}
	}
	return errs
}
