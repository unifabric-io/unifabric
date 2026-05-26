// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package topology

import (
	"crypto/sha1"
	"encoding/hex"
	"sort"
	"strings"
)

type Host struct {
	Name string `json:"name"`
}

type Switch struct {
	Name      string         `json:"name"`
	Neighbors []LLDPNeighbor `json:"neighbors"`
}

type LLDPNeighbor struct {
	LocalDeviceName string `json:"localDeviceName"`
	LocalPort       string `json:"localPort"`

	RemoteSystemName  string `json:"remoteSystemName"`
	RemoteChassisID   string `json:"remoteChassisID"`
	RemotePortID      string `json:"remotePortID"`
	RemotePortDesc    string `json:"remotePortDesc"`
	RemoteMgmtAddress string `json:"remoteMgmtAddress"`
}

type DeviceType string

const (
	DeviceHost   DeviceType = "host"
	DeviceSwitch DeviceType = "switch"
)

type TopologyNodeType string

const (
	TopologyNodeDevice TopologyNodeType = "device"
	TopologyNodeGroup  TopologyNodeType = "group"
)

type GroupNameMode string

const (
	GroupNameModeConcat GroupNameMode = "concat"
	GroupNameModeHash   GroupNameMode = "hash"
)

type Device struct {
	Name string     `json:"name"`
	Type DeviceType `json:"type"`
	Tier int        `json:"tier"`
}

type Link struct {
	SrcDevice string `json:"srcDevice"`
	SrcPort   string `json:"srcPort"`

	DstDevice string `json:"dstDevice"`
	DstPort   string `json:"dstPort"`

	LLDP LLDPNeighbor `json:"lldp"`
}

type TopologyGraph struct {
	Devices map[string]Device          `json:"devices"`
	Adj     map[string]map[string]bool `json:"adj"`
	Links   []Link                     `json:"links"`
}

type Topology struct {
	Nodes map[string]TopologyNode `json:"nodes"`
	Edges []TopologyEdge          `json:"edges"`

	NodesByTier map[int][]string `json:"nodesByTier"`

	GroupsByTier map[int][]TopologyGroup `json:"groupsByTier"`
	ParentIndex  map[string][]string     `json:"parentIndex"`
}

type TopologyNode struct {
	Name string           `json:"name"`
	Type TopologyNodeType `json:"type"`

	Tier       int        `json:"tier"`
	DeviceType DeviceType `json:"deviceType"`
	Members    []string   `json:"members"`
}

type TopologyEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
}

type TopologyGroup struct {
	Name string `json:"name"`
	Tier int    `json:"tier"`

	Members []string `json:"members"`

	LowerTierNodes []string `json:"lowerTierNodes"`
	UpperTierNodes []string `json:"upperTierNodes"`
}

type DiscoverOptions struct {
	GroupNameMode GroupNameMode
	HashLength    int
}

type DiscoverOption func(*DiscoverOptions)

func defaultDiscoverOptions() DiscoverOptions {
	return DiscoverOptions{
		GroupNameMode: GroupNameModeConcat,
		HashLength:    7,
	}
}

func WithConcatGroupName() DiscoverOption {
	return func(o *DiscoverOptions) {
		o.GroupNameMode = GroupNameModeConcat
	}
}

func WithHashGroupName() DiscoverOption {
	return func(o *DiscoverOptions) {
		o.GroupNameMode = GroupNameModeHash
	}
}

func WithHashLength(n int) DiscoverOption {
	return func(o *DiscoverOptions) {
		if n > 0 {
			o.HashLength = n
		}
	}
}

func DiscoverTopology(
	hosts []Host,
	switches []Switch,
	options ...DiscoverOption,
) Topology {
	opts := defaultDiscoverOptions()

	for _, option := range options {
		option(&opts)
	}

	graph := BuildTopologyGraph(hosts, switches)

	InferTiers(graph)

	NormalizeTierEdges(graph)

	return BuildTopology(graph, opts)
}

func BuildTopologyGraph(
	hosts []Host,
	switches []Switch,
) *TopologyGraph {
	g := &TopologyGraph{
		Devices: map[string]Device{},
		Adj:     map[string]map[string]bool{},
		Links:   []Link{},
	}

	for _, h := range hosts {
		g.Devices[h.Name] = Device{
			Name: h.Name,
			Type: DeviceHost,
			Tier: 0,
		}
		ensureVertex(g.Adj, h.Name)
	}

	for _, sw := range switches {
		g.Devices[sw.Name] = Device{
			Name: sw.Name,
			Type: DeviceSwitch,
			Tier: -1,
		}
		ensureVertex(g.Adj, sw.Name)
	}

	resolver := NewDeviceResolver(g.Devices)

	for _, sw := range switches {
		for _, n := range sw.Neighbors {
			if n.LocalDeviceName != "" && n.LocalDeviceName != sw.Name {
				continue
			}

			dstDevice, ok := resolver.Resolve(n)
			if !ok {
				continue
			}

			g.Links = append(g.Links, Link{
				SrcDevice: sw.Name,
				SrcPort:   n.LocalPort,
				DstDevice: dstDevice,
				DstPort:   n.RemotePortID,
				LLDP:      n,
			})

			g.addUndirectedEdge(sw.Name, dstDevice)
		}
	}

	return g
}

func ensureVertex(adj map[string]map[string]bool, name string) {
	if _, ok := adj[name]; !ok {
		adj[name] = map[string]bool{}
	}
}

func (g *TopologyGraph) addUndirectedEdge(a, b string) {
	ensureVertex(g.Adj, a)
	ensureVertex(g.Adj, b)

	g.Adj[a][b] = true
	g.Adj[b][a] = true
}

type DeviceResolver struct {
	byName map[string]string
}

func NewDeviceResolver(devices map[string]Device) *DeviceResolver {
	r := &DeviceResolver{
		byName: map[string]string{},
	}

	for name := range devices {
		r.byName[name] = name
	}

	return r
}

func (r *DeviceResolver) Resolve(n LLDPNeighbor) (string, bool) {
	if n.RemoteSystemName == "" {
		return "", false
	}

	name, ok := r.byName[n.RemoteSystemName]
	return name, ok
}

func InferTiers(g *TopologyGraph) {
	queue := []string{}

	for name, device := range g.Devices {
		if device.Type != DeviceSwitch {
			continue
		}

		if hasHostNeighbor(g, name) {
			device.Tier = 1
			g.Devices[name] = device
			queue = append(queue, name)
		}
	}

	sort.Strings(queue)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		currentDevice := g.Devices[current]
		currentTier := currentDevice.Tier

		neighbors := sortedBoolKeys(g.Adj[current])

		for _, neighbor := range neighbors {
			neighborDevice := g.Devices[neighbor]

			if neighborDevice.Type != DeviceSwitch {
				continue
			}

			if neighborDevice.Tier != -1 {
				continue
			}

			neighborDevice.Tier = currentTier + 1
			g.Devices[neighbor] = neighborDevice

			queue = append(queue, neighbor)
		}
	}
}

func hasHostNeighbor(g *TopologyGraph, switchName string) bool {
	for neighbor := range g.Adj[switchName] {
		if g.Devices[neighbor].Type == DeviceHost {
			return true
		}
	}

	return false
}

func NormalizeTierEdges(g *TopologyGraph) {
	newAdj := map[string]map[string]bool{}

	for name := range g.Devices {
		newAdj[name] = map[string]bool{}
	}

	for a, neighbors := range g.Adj {
		for b := range neighbors {
			da := g.Devices[a]
			db := g.Devices[b]

			if da.Tier < 0 || db.Tier < 0 {
				continue
			}

			if abs(da.Tier-db.Tier) != 1 {
				continue
			}

			newAdj[a][b] = true
			newAdj[b][a] = true
		}
	}

	g.Adj = newAdj
}

func abs(x int) int {
	if x < 0 {
		return -x
	}

	return x
}

func BuildTopology(
	g *TopologyGraph,
	opts DiscoverOptions,
) Topology {
	topology := Topology{
		Nodes:        map[string]TopologyNode{},
		Edges:        []TopologyEdge{},
		NodesByTier:  map[int][]string{},
		GroupsByTier: map[int][]TopologyGroup{},
		ParentIndex:  map[string][]string{},
	}

	for name, d := range g.Devices {
		if d.Tier < 0 {
			continue
		}

		topology.Nodes[name] = TopologyNode{
			Name:       name,
			Type:       TopologyNodeDevice,
			Tier:       d.Tier,
			DeviceType: d.Type,
			Members:    []string{},
		}

		topology.NodesByTier[d.Tier] = append(topology.NodesByTier[d.Tier], name)
	}

	sortNodesByTier(topology.NodesByTier)

	for a, neighbors := range g.Adj {
		for b := range neighbors {
			if a > b {
				continue
			}

			topology.Edges = append(topology.Edges, TopologyEdge{
				Source: a,
				Target: b,
				Kind:   "physical",
			})
		}
	}

	zones := CollectConnectedZones(g)

	for _, zoneDeviceNames := range zones {
		zone := BuildZoneDeviceView(g, zoneDeviceNames)
		deviceToGroup := map[string]string{}

		for name, device := range zone.Devices {
			if device.Tier == 0 {
				deviceToGroup[name] = name
			}
		}

		tiers := sortedTierKeys(zone.DevicesByTier)

		for _, tier := range tiers {
			if tier == 0 {
				continue
			}

			groups := BuildTierGroups(g, zone, tier, opts, deviceToGroup)

			for _, group := range groups {
				topology.GroupsByTier[tier] = append(topology.GroupsByTier[tier], group)

				for _, member := range group.Members {
					deviceToGroup[member] = group.Name
				}

				if len(group.Members) == 1 && group.Name == group.Members[0] {
					continue
				}

				topology.Nodes[group.Name] = TopologyNode{
					Name:       group.Name,
					Type:       TopologyNodeGroup,
					Tier:       tier,
					DeviceType: "",
					Members:    group.Members,
				}

				topology.NodesByTier[tier] = append(topology.NodesByTier[tier], group.Name)

				for _, member := range group.Members {
					topology.Edges = append(topology.Edges, TopologyEdge{
						Source: group.Name,
						Target: member,
						Kind:   "group-member",
					})
				}
			}
		}

		BuildParentIndexForZone(g, zone, &topology)
	}

	sortNodesByTier(topology.NodesByTier)
	sortGroupsByTier(topology.GroupsByTier)

	return topology
}

func CollectConnectedZones(g *TopologyGraph) [][]string {
	visited := map[string]bool{}
	var zones [][]string

	starts := collectHostStarts(g)

	for _, start := range starts {
		if visited[start] {
			continue
		}

		deviceNames := collectZoneDeviceNames(g, start, visited)
		zones = append(zones, deviceNames)
	}

	return zones
}

func collectHostStarts(g *TopologyGraph) []string {
	starts := []string{}

	for name, d := range g.Devices {
		if d.Type == DeviceHost {
			starts = append(starts, name)
		}
	}

	sort.Strings(starts)
	return starts
}

func collectZoneDeviceNames(
	g *TopologyGraph,
	start string,
	visited map[string]bool,
) []string {
	queue := []string{start}
	result := []string{}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current] {
			continue
		}

		visited[current] = true
		result = append(result, current)

		neighbors := sortedBoolKeys(g.Adj[current])

		for _, neighbor := range neighbors {
			if !visited[neighbor] {
				queue = append(queue, neighbor)
			}
		}
	}

	sort.Strings(result)
	return result
}

type ZoneDeviceView struct {
	Devices       map[string]Device
	DevicesByTier map[int]map[string]Device
}

func BuildZoneDeviceView(
	g *TopologyGraph,
	deviceNames []string,
) ZoneDeviceView {
	zone := ZoneDeviceView{
		Devices:       map[string]Device{},
		DevicesByTier: map[int]map[string]Device{},
	}

	for _, name := range deviceNames {
		device := g.Devices[name]

		zone.Devices[name] = device

		if _, ok := zone.DevicesByTier[device.Tier]; !ok {
			zone.DevicesByTier[device.Tier] = map[string]Device{}
		}

		zone.DevicesByTier[device.Tier][name] = device
	}

	return zone
}

func BuildTierGroups(
	g *TopologyGraph,
	zone ZoneDeviceView,
	tier int,
	opts DiscoverOptions,
	lowerDeviceToGroup map[string]string,
) []TopologyGroup {
	devicesInTier, ok := zone.DevicesByTier[tier]
	if !ok {
		return nil
	}

	lowerGroupToTierDevices := buildLowerGroupToTierDevices(
		g,
		zone,
		tier,
		lowerDeviceToGroup,
	)

	processed := map[string]bool{}
	groups := []TopologyGroup{}
	names := sortedDeviceNames(devicesInTier)

	for _, name := range names {
		if processed[name] {
			continue
		}

		members := map[string]Device{}
		lower := map[string]Device{}
		upper := map[string]Device{}

		queue := []string{name}

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]

			if processed[current] {
				continue
			}

			processed[current] = true
			members[current] = zone.Devices[current]

			neighbors := sortedBoolKeys(g.Adj[current])

			for _, neighbor := range neighbors {
				neighborDevice, ok := zone.Devices[neighbor]
				if !ok {
					continue
				}

				if neighborDevice.Tier == tier-1 {
					lower[neighbor] = neighborDevice

					lowerGroupName := groupNameOfDevice(neighbor, lowerDeviceToGroup)

					for _, related := range lowerGroupToTierDevices[lowerGroupName] {
						if !processed[related] {
							queue = append(queue, related)
						}
					}
				}

				if neighborDevice.Tier == tier+1 {
					upper[neighbor] = neighborDevice
				}
			}
		}

		memberNames := sortedNamesFromMap(members)

		groups = append(groups, TopologyGroup{
			Name:           GenerateGroupName(memberNames, opts),
			Tier:           tier,
			Members:        memberNames,
			LowerTierNodes: sortedNamesFromMap(lower),
			UpperTierNodes: sortedNamesFromMap(upper),
		})
	}

	return groups
}

func buildLowerGroupToTierDevices(
	g *TopologyGraph,
	zone ZoneDeviceView,
	tier int,
	lowerDeviceToGroup map[string]string,
) map[string][]string {
	result := map[string][]string{}

	devicesInTier, ok := zone.DevicesByTier[tier]
	if !ok {
		return result
	}

	deviceNames := sortedDeviceNames(devicesInTier)

	for _, deviceName := range deviceNames {
		neighbors := sortedBoolKeys(g.Adj[deviceName])

		for _, neighbor := range neighbors {
			neighborDevice, ok := zone.Devices[neighbor]
			if !ok {
				continue
			}

			if neighborDevice.Tier != tier-1 {
				continue
			}

			lowerGroupName := groupNameOfDevice(neighbor, lowerDeviceToGroup)

			result[lowerGroupName] = append(result[lowerGroupName], deviceName)
		}
	}

	for lowerGroupName := range result {
		sort.Strings(result[lowerGroupName])
		result[lowerGroupName] = dedupeSortedStrings(result[lowerGroupName])
	}

	return result
}

func groupNameOfDevice(deviceName string, deviceToGroup map[string]string) string {
	if groupName, ok := deviceToGroup[deviceName]; ok {
		return groupName
	}

	return deviceName
}

func GenerateGroupName(
	memberNames []string,
	opts DiscoverOptions,
) string {
	names := append([]string{}, memberNames...)
	sort.Strings(names)

	if len(names) == 1 {
		return names[0]
	}

	base := strings.Join(names, "-")

	switch opts.GroupNameMode {
	case GroupNameModeHash:
		return shortSHA(base, opts.HashLength)

	case GroupNameModeConcat:
		fallthrough

	default:
		return base + "-group"
	}
}

func shortSHA(s string, length int) string {
	sum := sha1.Sum([]byte(s))
	full := hex.EncodeToString(sum[:])

	if length <= 0 || length > len(full) {
		length = 7
	}

	return full[:length]
}

func BuildParentIndexForZone(
	g *TopologyGraph,
	zone ZoneDeviceView,
	topology *Topology,
) {
	deviceToGroup := buildDeviceToGroupIndex(zone, topology)

	buildDeviceParentIndex(g, zone, topology, deviceToGroup)

	buildGroupParentIndex(g, zone, topology, deviceToGroup)
}

func buildDeviceToGroupIndex(
	zone ZoneDeviceView,
	topology *Topology,
) map[string]string {
	deviceToGroup := map[string]string{}

	tiers := sortedTierKeys(topology.GroupsByTier)

	for _, tier := range tiers {
		if tier == 0 {
			continue
		}

		for _, group := range topology.GroupsByTier[tier] {
			for _, member := range group.Members {
				if _, ok := zone.Devices[member]; ok {
					deviceToGroup[member] = group.Name
				}
			}
		}
	}

	return deviceToGroup
}

func buildDeviceParentIndex(
	g *TopologyGraph,
	zone ZoneDeviceView,
	topology *Topology,
	deviceToGroup map[string]string,
) {
	deviceNames := sortedDeviceNames(zone.Devices)

	for _, childName := range deviceNames {
		child := zone.Devices[childName]
		parentSet := map[string]bool{}

		neighbors := sortedBoolKeys(g.Adj[childName])

		for _, neighbor := range neighbors {
			parent, ok := zone.Devices[neighbor]
			if !ok {
				continue
			}

			if parent.Tier != child.Tier+1 {
				continue
			}

			parentName := parent.Name

			if groupName, ok := deviceToGroup[parent.Name]; ok {
				parentName = groupName
			}

			parentSet[parentName] = true
		}

		if len(parentSet) > 0 {
			topology.ParentIndex[childName] = sortedBoolKeys(parentSet)
		}
	}
}

func buildGroupParentIndex(
	g *TopologyGraph,
	zone ZoneDeviceView,
	topology *Topology,
	deviceToGroup map[string]string,
) {
	tiers := sortedTierKeys(topology.GroupsByTier)

	for _, tier := range tiers {
		if tier == 0 {
			continue
		}

		groups := topology.GroupsByTier[tier]

		for _, group := range groups {
			parentSet := map[string]bool{}

			for _, member := range group.Members {
				memberDevice, ok := zone.Devices[member]
				if !ok {
					continue
				}

				neighbors := sortedBoolKeys(g.Adj[member])

				for _, neighbor := range neighbors {
					parent, ok := zone.Devices[neighbor]
					if !ok {
						continue
					}

					if parent.Tier != memberDevice.Tier+1 {
						continue
					}

					parentName := parent.Name

					if groupName, ok := deviceToGroup[parent.Name]; ok {
						parentName = groupName
					}

					if parentName == group.Name {
						continue
					}

					parentSet[parentName] = true
				}
			}

			if len(parentSet) > 0 {
				topology.ParentIndex[group.Name] = sortedBoolKeys(parentSet)
			}
		}
	}
}

func (t Topology) QueryParents(name string) []string {
	return t.ParentIndex[name]
}

func (t Topology) QueryParentChain(name string) []string {
	result := []string{}
	seen := map[string]bool{}

	current := name

	for {
		parents := t.ParentIndex[current]
		if len(parents) == 0 {
			break
		}

		parent := parents[0]

		if seen[parent] {
			break
		}

		seen[parent] = true
		result = append(result, parent)

		current = parent
	}

	return result
}

func sortedDeviceNames[T any](m map[string]T) []string {
	names := []string{}

	for name := range m {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

func sortedNamesFromMap[T any](m map[string]T) []string {
	names := []string{}

	for name := range m {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

func sortedBoolKeys(m map[string]bool) []string {
	names := []string{}

	for name := range m {
		if m[name] {
			names = append(names, name)
		}
	}

	sort.Strings(names)
	return names
}

func sortedTierKeys[T any](m map[int]T) []int {
	tiers := []int{}

	for tier := range m {
		tiers = append(tiers, tier)
	}

	sort.Ints(tiers)
	return tiers
}

func sortNodesByTier(nodesByTier map[int][]string) {
	for tier := range nodesByTier {
		sort.Strings(nodesByTier[tier])
		nodesByTier[tier] = dedupeSortedStrings(nodesByTier[tier])
	}
}

func sortGroupsByTier(groupsByTier map[int][]TopologyGroup) {
	for tier := range groupsByTier {
		sort.Slice(groupsByTier[tier], func(i, j int) bool {
			return groupsByTier[tier][i].Name < groupsByTier[tier][j].Name
		})
	}
}

func dedupeSortedStrings(values []string) []string {
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
