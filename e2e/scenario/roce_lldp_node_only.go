// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package scenario

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/unifabric-io/unifabric/e2e/check"
	"github.com/unifabric-io/unifabric/e2e/kube"
)

const (
	switchNeighborsAnnotation = "unifabric.io/switch-neighbors"
)

func runRoCELLDPNodeOnly(r *runner) error {
	return runRoCELLDP(r, roceLLDP{
		switchCRs: 1,
		switchChecks: func(r *runner) error {
			return r.waitCheck(
				"check_switch_annotations",
				validateRoCELLDPNodeOnlySwitchInputs,
				"switch annotation inputs",
				"switch-only neighbors without management endpoints",
			)
		},
		topologyCheck: check.ScaleOutNodeOnlyTopologyStatus,
	})
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
	var switches kube.SwitchList
	if err := kube.FetchJSON("switches.unifabric.io", &switches); err != nil {
		return false, fmt.Sprintf("failed to read Switch resources: %v", err)
	}
	actualByName := make(map[string]kube.Switch, len(switches.Items))
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
