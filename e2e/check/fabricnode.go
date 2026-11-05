// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package check

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/unifabric-io/unifabric/e2e/kube"
)

func loadExpectedInterfaces(topologyDir string) (map[string]expectedInterfaces, error) {
	paths, err := filepath.Glob(filepath.Join(topologyDir, "node-gpu-*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	expectedGPUIfaces := map[string]struct{}{}
	for i := 1; i <= 8; i++ {
		expectedGPUIfaces[fmt.Sprintf("eth%d", i)] = struct{}{}
	}
	expectedStorageIfaces := map[string]struct{}{"eth9": {}}

	expected := make(map[string]expectedInterfaces, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		var data netplanFile
		if err := yaml.Unmarshal(content, &data); err != nil {
			return nil, err
		}

		roles := expectedInterfaces{
			gpu:     map[string]string{},
			storage: map[string]string{},
		}
		for iface, cfg := range data.Network.Ethernets {
			if len(cfg.Addresses) == 0 {
				continue
			}
			if _, ok := expectedGPUIfaces[iface]; ok {
				roles.gpu[iface] = cfg.Addresses[0]
				continue
			}
			if _, ok := expectedStorageIfaces[iface]; ok {
				roles.storage[iface] = cfg.Addresses[0]
			}
		}

		expected[strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))] = roles
	}
	return expected, nil
}

func FabricNodeInterfaces(topologyDir string) (bool, string) {
	expectedByNode, err := loadExpectedInterfaces(topologyDir)
	if err != nil {
		return false, fmt.Sprintf("failed to load topology interfaces: %v", err)
	}
	if len(expectedByNode) == 0 {
		return false, fmt.Sprintf("no node-gpu-*.yaml found under %s", topologyDir)
	}

	var nodes fabricNodeList
	if err := kube.FetchJSON("fabricnodes.unifabric.io", &nodes); err != nil {
		return false, fmt.Sprintf("failed to read fabricnodes: %v", err)
	}

	actualGPUByNode := map[string]map[string]string{}
	actualStorageByNode := map[string]map[string]string{}
	for _, item := range nodes.Items {
		name := item.Metadata.Name
		if !strings.HasPrefix(name, "node-gpu-") {
			continue
		}

		gpuIfaces := map[string]string{}
		storageIfaces := map[string]string{}
		for _, nic := range item.Status.ScaleOutNics {
			if nic.Name != "" {
				gpuIfaces[nic.Name] = nic.IPv4
			}
		}
		for _, nic := range item.Status.StorageNics {
			if nic.Name != "" {
				storageIfaces[nic.Name] = nic.IPv4
			}
		}
		actualGPUByNode[name] = gpuIfaces
		actualStorageByNode[name] = storageIfaces
	}

	errs := []string{}
	for nodeName, expectedRoles := range expectedByNode {
		actualGPUIfaces, okGPU := actualGPUByNode[nodeName]
		actualStorageIfaces, okStorage := actualStorageByNode[nodeName]
		if !okGPU || !okStorage {
			errs = append(errs, fmt.Sprintf("%s: FabricNode missing", nodeName))
			continue
		}

		for iface, expectedIP := range expectedRoles.gpu {
			actualIP, ok := actualGPUIfaces[iface]
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: missing gpu iface %s", nodeName, iface))
				continue
			}
			if actualIP != "" && actualIP != expectedIP {
				errs = append(errs, fmt.Sprintf("%s:gpu:%s expected %s, got %s", nodeName, iface, expectedIP, actualIP))
			}
		}

		for iface, expectedIP := range expectedRoles.storage {
			actualIP, ok := actualStorageIfaces[iface]
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: missing storage iface %s", nodeName, iface))
				continue
			}
			if actualIP != "" && actualIP != expectedIP {
				errs = append(errs, fmt.Sprintf("%s:storage:%s expected %s, got %s", nodeName, iface, expectedIP, actualIP))
			}
		}
	}

	if len(errs) > 0 {
		sort.Strings(errs)
		return false, strings.Join(errs, "; ")
	}
	return true, fmt.Sprintf("validated gpu eth1-eth8 and storage eth9 for %d GPU nodes", len(expectedByNode))
}

type netplanFile struct {
	Network struct {
		Ethernets map[string]struct {
			Addresses []string `yaml:"addresses"`
		} `yaml:"ethernets"`
	} `yaml:"network"`
}

type expectedInterfaces struct {
	gpu     map[string]string
	storage map[string]string
}

type fabricNodeList struct {
	Items []fabricNode `json:"items"`
}

type fabricNode struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Status struct {
		ScaleOutNics []nicInfo `json:"scaleOutNics"`
		StorageNics  []nicInfo `json:"storageNics"`
	} `json:"status"`
}

type nicInfo struct {
	Name string `json:"name"`
	IPv4 string `json:"ipv4"`
}
