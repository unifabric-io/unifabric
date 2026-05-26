// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchtopology

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	topology "github.com/unifabric-io/unifabric/pkg/topology"
)

type desiredSwitchGroup struct {
	InternalName string
	Name         string
	LabelValue   string
	Role         v1beta1.SwitchRole
	Tier         v1beta1.SwitchGroupTier
	Healthy      bool
	Switches     []string
	Nodes        []string
}

type desiredNodeLabels struct {
	Leaf  string
	Spine string
	Core  string
}

type topologyInputs struct {
	hosts    []topology.Host
	switches []topology.Switch
}

type topologySwitchBuilder struct {
	data topology.Switch
	seen map[string]bool
}

func buildDesiredState(cfg *config.ControllerConfig, fabricNodes []v1beta1.FabricNode, switches []v1beta1.Switch) ([]desiredSwitchGroup, map[string]desiredNodeLabels, []string) {
	managedNodeNames := managedNodeNames(fabricNodes)
	desiredNodeLabelsByName := make(map[string]desiredNodeLabels)
	desiredGroups := []desiredSwitchGroup{}
	selectorCfg := config.ControllerNodeTopologyConfig{}
	if cfg != nil {
		selectorCfg = cfg.NodeTopologyDiscovery
	}

	for _, role := range []v1beta1.SwitchRole{
		v1beta1.SwitchRoleScaleOut,
		v1beta1.SwitchRoleScaleUp,
		v1beta1.SwitchRoleStorage,
	} {
		inputs := buildTopologyInputsForRole(role, selectorCfg, fabricNodes, switches)
		if len(inputs.hosts) == 0 || len(inputs.switches) == 0 {
			continue
		}

		roleGroups, desiredByInternalName, topologyData := buildDesiredGroupsForRole(cfg, role, inputs)
		desiredGroups = append(desiredGroups, roleGroups...)

		if role != v1beta1.SwitchRoleScaleOut {
			continue
		}

		for _, host := range inputs.hosts {
			desiredNodeLabelsByName[host.Name] = labelsForHost(topologyData, desiredByInternalName, host.Name)
		}
	}

	sort.Slice(desiredGroups, func(i, j int) bool {
		if desiredGroups[i].Role != desiredGroups[j].Role {
			return desiredGroups[i].Role < desiredGroups[j].Role
		}
		return desiredGroups[i].Name < desiredGroups[j].Name
	})

	return desiredGroups, desiredNodeLabelsByName, managedNodeNames
}

func buildTopologyInputs(fabricNodes []v1beta1.FabricNode, switches []v1beta1.Switch) (topologyInputs, []string) {
	return buildTopologyInputsForRole(v1beta1.SwitchRoleScaleOut, config.ControllerNodeTopologyConfig{}, fabricNodes, switches), managedNodeNames(fabricNodes)
}

func buildDesiredGroupsForRole(cfg *config.ControllerConfig, role v1beta1.SwitchRole, inputs topologyInputs) ([]desiredSwitchGroup, map[string]desiredSwitchGroup, topology.Topology) {
	topologyData := topology.DiscoverTopology(
		inputs.hosts,
		inputs.switches,
		topology.WithHashGroupName(),
		topology.WithHashLength(cfg.ScaleOutDiscovery.Switches.GroupNaming.HashLength),
	)

	hostSet := make(map[string]bool, len(inputs.hosts))
	for _, host := range inputs.hosts {
		hostSet[host.Name] = true
	}

	desiredGroups := []desiredSwitchGroup{}
	desiredByInternalName := map[string]desiredSwitchGroup{}
	for tier, groups := range topologyData.GroupsByTier {
		if tier < int(v1beta1.SwitchGroupTierLeaf) || tier > int(v1beta1.SwitchGroupTierCore) {
			continue
		}

		for _, group := range groups {
			switchNames := append([]string(nil), group.Members...)
			sort.Strings(switchNames)
			if len(switchNames) == 0 {
				continue
			}

			nodeNames := []string{}
			if tier == int(v1beta1.SwitchGroupTierLeaf) {
				for _, nodeName := range group.LowerTierNodes {
					if hostSet[nodeName] {
						nodeNames = append(nodeNames, nodeName)
					}
				}
				sort.Strings(nodeNames)
				nodeNames = dedupeStrings(nodeNames)
			}

			desired := desiredSwitchGroup{
				InternalName: group.Name,
				Name:         stableSwitchGroupName(tier, switchNames, cfg.ScaleOutDiscovery.Switches.GroupNaming.HashLength),
				LabelValue:   switchGroupLabelValue(cfg.ScaleOutDiscovery.Switches.GroupNaming, tier, switchNames),
				Role:         role,
				Tier:         v1beta1.SwitchGroupTier(tier),
				Healthy:      tier != int(v1beta1.SwitchGroupTierLeaf) || len(nodeNames) > 0,
				Switches:     switchNames,
				Nodes:        nodeNames,
			}

			desiredGroups = append(desiredGroups, desired)
			desiredByInternalName[group.Name] = desired
		}
	}

	return desiredGroups, desiredByInternalName, topologyData
}

func buildTopologyInputsForRole(role v1beta1.SwitchRole, selectorCfg config.ControllerNodeTopologyConfig, fabricNodes []v1beta1.FabricNode, switches []v1beta1.Switch) topologyInputs {
	managedHostSet := map[string]bool{}
	normalizedSwitchNames := map[string]string{}
	participatingHostSet := map[string]bool{}

	for _, node := range fabricNodes {
		managedHostSet[node.Name] = true
	}
	for _, sw := range switches {
		if effectiveSwitchRole(sw) != role {
			continue
		}
		normalizedSwitchNames[normalizeResourceName(sw.Name)] = sw.Name
		if sw.Status.Hostname != "" {
			normalizedSwitchNames[normalizeResourceName(sw.Status.Hostname)] = sw.Name
		}
	}
	builderBySwitch := map[string]*topologySwitchBuilder{}
	for _, sw := range switches {
		if effectiveSwitchRole(sw) != role || !sw.Status.Healthy {
			continue
		}

		builder := &topologySwitchBuilder{
			data: topology.Switch{Name: sw.Name},
			seen: map[string]bool{},
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
		builderBySwitch[sw.Name] = builder
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

			switchName, ok := resolveSwitchName(nic.LLDPNeighbor.Hostname, normalizedSwitchNames, builderBySwitch)
			if !ok {
				continue
			}

			builderBySwitch[switchName].addNeighbor(topology.Neighbor{
				LocalDeviceName:  switchName,
				RemoteSystemName: node.Name,
			})
			matchedHost = true
		}

		if matchedHost {
			participatingHostSet[node.Name] = true
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

	return topologyInputs{hosts: hosts, switches: topoSwitches}
}

func managedNodeNames(fabricNodes []v1beta1.FabricNode) []string {
	managedNodeSet := map[string]bool{}
	for _, node := range fabricNodes {
		managedNodeSet[node.Name] = true
	}
	return sortedMapKeys(managedNodeSet)
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

func (r *Reconciler) reconcileSwitchGroups(ctx context.Context, desiredGroups []desiredSwitchGroup) error {
	var currentGroups v1beta1.SwitchGroupList
	if err := r.client.List(ctx, &currentGroups); err != nil {
		return err
	}

	desiredByName := make(map[string]desiredSwitchGroup, len(desiredGroups))
	for _, group := range desiredGroups {
		desiredByName[group.Name] = group
	}

	for _, currentGroup := range currentGroups.Items {
		if _, ok := desiredByName[currentGroup.Name]; ok {
			continue
		}
		if err := r.client.Delete(ctx, currentGroup.DeepCopy()); err != nil && client.IgnoreNotFound(err) != nil {
			return err
		}
	}

	for _, desiredGroup := range desiredGroups {
		if err := r.upsertSwitchGroup(ctx, desiredGroup); err != nil {
			return err
		}
	}

	return nil
}

func (r *Reconciler) upsertSwitchGroup(ctx context.Context, desiredGroup desiredSwitchGroup) error {
	group := &v1beta1.SwitchGroup{}
	groupKey := types.NamespacedName{Name: desiredGroup.Name}
	if err := r.client.Get(ctx, groupKey, group); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return err
		}

		group = &v1beta1.SwitchGroup{
			ObjectMeta: metav1.ObjectMeta{Name: desiredGroup.Name},
		}
		if err := r.client.Create(ctx, group); err != nil {
			return err
		}
	}

	desiredStatus := v1beta1.SwitchGroupStatus{
		Role:     desiredGroup.Role,
		Tier:     desiredGroup.Tier,
		Healthy:  desiredGroup.Healthy,
		Switches: make([]v1beta1.SwitchGroupSwitchStatus, 0, len(desiredGroup.Switches)),
	}
	for _, switchName := range desiredGroup.Switches {
		desiredStatus.Switches = append(desiredStatus.Switches, v1beta1.SwitchGroupSwitchStatus{
			SwitchRef: v1beta1.SwitchGroupSwitchReference{Name: switchName},
		})
	}
	if len(desiredGroup.Nodes) > 0 {
		desiredStatus.Nodes = make([]v1beta1.SwitchGroupNodeStatus, 0, len(desiredGroup.Nodes))
		for _, nodeName := range desiredGroup.Nodes {
			desiredStatus.Nodes = append(desiredStatus.Nodes, v1beta1.SwitchGroupNodeStatus{
				FabricNodeRef: v1beta1.SwitchGroupNodeReference{Name: nodeName},
			})
		}
	}

	if equality.Semantic.DeepEqual(group.Status, desiredStatus) {
		return nil
	}

	group.Status = desiredStatus
	return r.client.Status().Update(ctx, group)
}

func (r *Reconciler) reconcileNodeLabels(ctx context.Context, managedNodeNames []string, desiredNodeLabelsByName map[string]desiredNodeLabels) error {
	for _, nodeName := range managedNodeNames {
		node := &corev1.Node{}
		if err := r.client.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			if client.IgnoreNotFound(err) != nil {
				return err
			}
			continue
		}

		desiredLabels := desiredNodeLabelsByName[nodeName]
		changed := false
		if node.Labels == nil {
			node.Labels = map[string]string{}
		}

		changed = setNodeTopologyLabel(node.Labels, r.cfg.TopologyLabels.ScaleOutLeaf, config.DefaultLabelScaleOutLeaf, desiredLabels.Leaf) || changed
		changed = setNodeTopologyLabel(node.Labels, r.cfg.TopologyLabels.ScaleOutSpine, config.DefaultLabelScaleOutSpine, desiredLabels.Spine) || changed
		changed = setNodeTopologyLabel(node.Labels, r.cfg.TopologyLabels.ScaleOutCore, config.DefaultLabelScaleOutCore, desiredLabels.Core) || changed
		if !changed {
			continue
		}

		if err := r.client.Update(ctx, node); err != nil {
			return err
		}
	}

	return nil
}

func labelsForHost(topologyData topology.Topology, desiredByInternalName map[string]desiredSwitchGroup, hostName string) desiredNodeLabels {
	labels := desiredNodeLabels{}
	for _, internalGroupName := range topologyData.QueryParentChain(hostName) {
		desiredGroup, ok := desiredByInternalName[internalGroupName]
		if !ok {
			continue
		}

		switch desiredGroup.Tier {
		case v1beta1.SwitchGroupTierLeaf:
			labels.Leaf = desiredGroup.LabelValue
		case v1beta1.SwitchGroupTierSpine:
			labels.Spine = desiredGroup.LabelValue
		case v1beta1.SwitchGroupTierCore:
			labels.Core = desiredGroup.LabelValue
		}
	}
	return labels
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

func switchGroupLabelValue(naming config.SwitchGroupNamingConfig, tier int, switchNames []string) string {
	names := append([]string(nil), switchNames...)
	sort.Strings(names)

	if naming.LabelValueFormat == "name" {
		if len(names) == 1 {
			return names[0]
		}
		return strings.Join(names, "-") + "-group"
	}

	return switchGroupNamePrefix(tier) + shortHash(strings.Join(names, ","), naming.HashLength)
}

func stableSwitchGroupName(tier int, switchNames []string, hashLength int) string {
	names := append([]string(nil), switchNames...)
	sort.Strings(names)
	return switchGroupNamePrefix(tier) + shortHash(fmt.Sprintf("tier=%d|switches=%s", tier, strings.Join(names, ",")), hashLength)
}

func switchGroupNamePrefix(tier int) string {
	switch v1beta1.SwitchGroupTier(tier) {
	case v1beta1.SwitchGroupTierLeaf:
		return "leaf-"
	case v1beta1.SwitchGroupTierSpine:
		return "spine-"
	case v1beta1.SwitchGroupTierCore:
		return "core-"
	default:
		return ""
	}
}

func shortHash(value string, length int) string {
	sum := sha1.Sum([]byte(value))
	encoded := hex.EncodeToString(sum[:])
	if length <= 0 || length > len(encoded) {
		length = 7
	}
	return encoded[:length]
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

func setNodeTopologyLabel(labels map[string]string, configuredKey, defaultKey, desiredValue string) bool {
	labelKey := configuredKey
	if labelKey == "" {
		labelKey = defaultKey
	}

	currentValue, exists := labels[labelKey]
	switch {
	case desiredValue == "" && exists:
		delete(labels, labelKey)
		return true
	case desiredValue == "":
		return false
	case !exists || currentValue != desiredValue:
		labels[labelKey] = desiredValue
		return true
	default:
		return false
	}
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
