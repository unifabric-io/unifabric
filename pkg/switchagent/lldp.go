// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchagent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/unifabric-io/unifabric/pkg/agent/fabricnode"
	"github.com/unifabric-io/unifabric/pkg/config"
	"google.golang.org/protobuf/proto"
)

const (
	lldpCLI104Path = "/usr/local/sbin/lldpcli-1.0.4"
	lldpCLI116Path = "/usr/local/sbin/lldpcli-1.0.16"
)

func collectSnapshot(cfg *config.SwitchAgentConfig, generation uint64) (*LLDPNeighborSnapshot, error) {
	return collectSnapshotWithOptions(cfg.SwitchName, generation, lldpCollectionOptions(cfg))
}

func collectSnapshotWithOptions(switchName string, generation uint64, options fabricnode.LldpCliOptions) (*LLDPNeighborSnapshot, error) {
	output, err := fabricnode.LldpCliShowNeighborsWithOptions(options)
	if err != nil {
		return nil, err
	}
	return parseSnapshot(switchName, generation, output)
}

func lldpCollectionOptions(cfg *config.SwitchAgentConfig) fabricnode.LldpCliOptions {
	if cfg.LLDP.CollectionMode == "hostProc" {
		return fabricnode.LldpCliOptions{UseHostNamespace: true}
	}

	switch cfg.LLDP.CLIVersion {
	case "1.0.4":
		return fabricnode.LldpCliOptions{BinaryPath: lldpCLI104Path, SocketPath: cfg.LLDP.SocketPath}
	default:
		return fabricnode.LldpCliOptions{BinaryPath: lldpCLI116Path, SocketPath: cfg.LLDP.SocketPath}
	}
}

func parseSnapshot(switchName string, generation uint64, output []byte) (*LLDPNeighborSnapshot, error) {
	var response fabricnode.LLDPJSON0Response
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("unmarshal lldp json0 output: %w", err)
	}

	normalizedSwitchName := normalizeSwitchName(switchName)
	neighbors := make([]*LLDPNeighbor, 0)
	for _, lldpData := range response.LLDP {
		for _, iface := range lldpData.Interface {
			if len(iface.Chassis) == 0 || len(iface.Port) == 0 {
				continue
			}

			chassis := iface.Chassis[0]
			port := iface.Port[0]
			if len(chassis.Name) == 0 || len(port.ID) == 0 {
				continue
			}

			neighbors = append(neighbors, &LLDPNeighbor{
				LocalDeviceName:  normalizedSwitchName,
				LocalPort:        iface.Name,
				RemoteSystemName: chassis.Name[0].Value,
				RemotePortId:     port.ID[0].Value,
			})
		}
	}

	sort.Slice(neighbors, func(i, j int) bool {
		if neighbors[i].LocalPort != neighbors[j].LocalPort {
			return neighbors[i].LocalPort < neighbors[j].LocalPort
		}
		if neighbors[i].RemoteSystemName != neighbors[j].RemoteSystemName {
			return neighbors[i].RemoteSystemName < neighbors[j].RemoteSystemName
		}
		return neighbors[i].RemotePortId < neighbors[j].RemotePortId
	})

	return &LLDPNeighborSnapshot{
		SwitchName:    normalizedSwitchName,
		LldpNeighbors: neighbors,
		Generation:    generation,
	}, nil
}

func normalizeSwitchName(name string) string {
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

func snapshotsEqual(previous, next *LLDPNeighborSnapshot) bool {
	if previous == nil || next == nil {
		return previous == next
	}

	previousCopy := proto.Clone(previous).(*LLDPNeighborSnapshot)
	nextCopy := proto.Clone(next).(*LLDPNeighborSnapshot)
	previousCopy.Generation = 0
	nextCopy.Generation = 0
	return proto.Equal(previousCopy, nextCopy)
}
