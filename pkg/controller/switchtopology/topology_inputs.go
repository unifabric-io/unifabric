// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchtopology

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	topology "github.com/unifabric-io/unifabric/pkg/topology"
)

type topologyInputs struct {
	hosts             []topology.Host
	switches          []topology.Switch
	nodeOnly          bool
	syntheticSwitches map[string]bool
}

type topologySwitchBuilder struct {
	data topology.Switch
	seen map[string]bool
}

func buildTopologyInputsForRole(role v1beta1.SwitchRole, fabricNodes []v1beta1.FabricNode, switches []v1beta1.Switch) (topologyInputs, error) {
	managedHostSet := map[string]bool{}
	normalizedSwitchNames := map[string]string{}
	participatingHostSet := map[string]bool{}
	roleSwitches := make([]v1beta1.Switch, 0, len(switches))

	for _, node := range fabricNodes {
		managedHostSet[node.Name] = true
	}
	for _, sw := range switches {
		if effectiveSwitchRole(sw) != role {
			continue
		}
		roleSwitches = append(roleSwitches, sw)
		normalizedSwitchNames[normalizeResourceName(sw.Name)] = sw.Name
		if sw.Status.Hostname != "" {
			normalizedSwitchNames[normalizeResourceName(sw.Status.Hostname)] = sw.Name
		}
	}
	nodeOnly := len(roleSwitches) == 0
	semiAutomatic := !nodeOnly
	for _, sw := range roleSwitches {
		if !hasManualSwitchNeighbors(sw) {
			semiAutomatic = false
			break
		}
	}
	syntheticSwitches := map[string]bool{}
	builderBySwitch := map[string]*topologySwitchBuilder{}
	ensureSwitchBuilder := func(switchName string) *topologySwitchBuilder {
		builder, ok := builderBySwitch[switchName]
		if ok {
			return builder
		}

		builder = &topologySwitchBuilder{
			data: topology.Switch{Name: switchName},
			seen: map[string]bool{},
		}
		builderBySwitch[switchName] = builder
		return builder
	}
	for _, sw := range roleSwitches {
		ensureSwitchBuilder(sw.Name)
	}

	for _, node := range fabricNodes {
		nics := nodeNicsForRole(node, role)
		if len(nics) == 0 {
			continue
		}

		matchedHost := false
		for _, nic := range nics {
			if nic.State != "up" || nic.LLDPNeighbor.Hostname == "" || nic.LLDPNeighbor.Port == "" {
				continue
			}

			var switchName string
			var matchedRealSwitch bool
			switchName, matchedRealSwitch = resolveSwitchName(nic.LLDPNeighbor.Hostname, normalizedSwitchNames, builderBySwitch)
			if !matchedRealSwitch && (nodeOnly || semiAutomatic) {
				switchName = normalizeResourceName(nic.LLDPNeighbor.Hostname)
				if switchName == "" {
					continue
				}
				syntheticSwitches[switchName] = true
			} else if !matchedRealSwitch {
				continue
			}

			ensureSwitchBuilder(switchName).addNeighbor(topology.Neighbor{
				LocalDeviceName:  switchName,
				RemoteSystemName: node.Name,
			})
			matchedHost = true
		}

		if matchedHost {
			participatingHostSet[node.Name] = true
		}
	}

	for _, sw := range roleSwitches {
		builder := ensureSwitchBuilder(sw.Name)
		if semiAutomatic {
			manualNeighbors, err := parseManualSwitchNeighbors(sw)
			if err != nil {
				return topologyInputs{}, err
			}
			for _, neighborName := range manualNeighbors {
				neighborSwitchName, exists := resolveSwitchName(neighborName, normalizedSwitchNames, builderBySwitch)
				if !exists {
					neighborSwitchName = normalizeResourceName(neighborName)
					exists = syntheticSwitches[neighborSwitchName]
				}
				if !exists {
					return topologyInputs{}, fmt.Errorf("Switch %q annotation %q references unknown %s Switch or node-discovered leaf %q", sw.Name, v1beta1.SwitchNeighborsAnnotation, role, neighborName)
				}
				if neighborSwitchName == sw.Name {
					return topologyInputs{}, fmt.Errorf("Switch %q annotation %q must not reference itself", sw.Name, v1beta1.SwitchNeighborsAnnotation)
				}
				builder.addNeighbor(topology.Neighbor{
					LocalDeviceName:  sw.Name,
					RemoteSystemName: neighborSwitchName,
				})
			}
			continue
		}

		for _, neighbor := range sw.Status.LLDPNeighbors {
			remoteSystemName := canonicalRemoteSystemName(
				neighbor.RemoteSystemName,
				managedHostSet,
				normalizedSwitchNames,
			)
			isHostPeer := neighbor.RemoteSystemType == v1beta1.SwitchLLDPRemoteSystemTypeKubernetesNode || managedHostSet[remoteSystemName]
			if isHostPeer {
				continue
			}
			builder.addNeighbor(topology.Neighbor{
				LocalDeviceName:  sw.Name,
				RemoteSystemName: remoteSystemName,
			})
		}
	}

	hostNames := sortedMapKeys(participatingHostSet)
	hosts := make([]topology.Host, 0, len(hostNames))
	for _, hostName := range hostNames {
		hosts = append(hosts, topology.Host{Name: hostName})
	}

	switchNames := sortedMapKeys(buildersToBoolMap(builderBySwitch))
	topoSwitches := make([]topology.Switch, 0, len(switchNames))
	for _, switchName := range switchNames {
		builder := builderBySwitch[switchName]
		sort.Slice(builder.data.Neighbors, func(i, j int) bool {
			return builder.data.Neighbors[i].RemoteSystemName < builder.data.Neighbors[j].RemoteSystemName
		})
		topoSwitches = append(topoSwitches, builder.data)
	}

	return topologyInputs{
		hosts:             hosts,
		switches:          topoSwitches,
		nodeOnly:          nodeOnly,
		syntheticSwitches: syntheticSwitches,
	}, nil
}

func hasManualSwitchNeighbors(sw v1beta1.Switch) bool {
	_, exists := sw.Annotations[v1beta1.SwitchNeighborsAnnotation]
	return exists
}

func parseManualSwitchNeighbors(sw v1beta1.Switch) ([]string, error) {
	raw := strings.TrimSpace(sw.Annotations[v1beta1.SwitchNeighborsAnnotation])
	if raw == "" {
		return nil, nil
	}

	var neighbors []string
	if err := json.Unmarshal([]byte(raw), &neighbors); err != nil {
		return nil, fmt.Errorf("Switch %q annotation %q must be a JSON string array: %w", sw.Name, v1beta1.SwitchNeighborsAnnotation, err)
	}
	for index := range neighbors {
		neighbors[index] = strings.TrimSpace(neighbors[index])
		if neighbors[index] == "" {
			return nil, fmt.Errorf("Switch %q annotation %q contains an empty neighbor name", sw.Name, v1beta1.SwitchNeighborsAnnotation)
		}
	}
	sort.Strings(neighbors)
	return dedupeStrings(neighbors), nil
}

func effectiveSwitchRole(sw v1beta1.Switch) v1beta1.SwitchRole {
	if sw.Spec.Role == "" {
		return v1beta1.SwitchRoleScaleOut
	}
	return sw.Spec.Role
}

func nodeNicsForRole(node v1beta1.FabricNode, role v1beta1.SwitchRole) []v1beta1.NicInfo {
	switch role {
	case v1beta1.SwitchRoleScaleOut:
		if node.Status.NodeRole == v1beta1.NodeRoleStorage {
			return nil
		}
		return node.Status.ScaleOutNics
	case v1beta1.SwitchRoleScaleUp:
		return nil
	case v1beta1.SwitchRoleStorage:
		return node.Status.StorageNics
	default:
		return nil
	}
}

func (b *topologySwitchBuilder) addNeighbor(neighbor topology.Neighbor) {
	if neighbor.RemoteSystemName == "" {
		return
	}

	key := neighbor.RemoteSystemName
	if b.seen[key] {
		return
	}
	b.seen[key] = true
	b.data.Neighbors = append(b.data.Neighbors, neighbor)
}

func canonicalRemoteSystemName(raw string, hostNames map[string]bool, normalizedSwitchNames map[string]string) string {
	if hostNames[raw] {
		return raw
	}
	if switchName, ok := normalizedSwitchNames[normalizeResourceName(raw)]; ok {
		return switchName
	}
	return raw
}

func resolveSwitchName(raw string, normalizedSwitchNames map[string]string, existing map[string]*topologySwitchBuilder) (string, bool) {
	if _, ok := existing[raw]; ok {
		return raw, true
	}
	switchName, ok := normalizedSwitchNames[normalizeResourceName(raw)]
	if !ok {
		return "", false
	}
	_, ok = existing[switchName]
	return switchName, ok
}

func normalizeResourceName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var builder strings.Builder
	previousDash := false
	for _, r := range name {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			builder.WriteRune(r)
			previousDash = false
		case !previousDash:
			builder.WriteRune('-')
			previousDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func buildersToBoolMap(values map[string]*topologySwitchBuilder) map[string]bool {
	result := make(map[string]bool, len(values))
	for key := range values {
		result[key] = true
	}
	return result
}

func sortedMapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}

	result := []string{values[0]}
	for i := 1; i < len(values); i++ {
		if values[i] != values[i-1] {
			result = append(result, values[i])
		}
	}
	return result
}
