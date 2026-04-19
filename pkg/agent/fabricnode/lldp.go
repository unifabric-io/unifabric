// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package fabricnode

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"strings"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
)

// RdmaInterfaceMethod represents the method used to identify RDMA interfaces
type RdmaInterfaceMethod struct {
	Method string
	Value  string
}

func (m RdmaInterfaceMethod) IsInterfaceMethod() bool {
	return m.Method == "interface"
}

func (m RdmaInterfaceMethod) IsValidCidr() bool {
	_, _, err := net.ParseCIDR(m.Value)
	return err == nil
}

func (m RdmaInterfaceMethod) IsCidrMethod() bool {
	return m.Method == "cidr"
}

func (m *RdmaInterfaceMethod) CheckOrParsePattern(pattern string) error {
	if pattern == "" {
		return nil
	}
	patternList := strings.SplitN(pattern, "=", 2)
	if len(patternList) != 2 {
		return fmt.Errorf("invalid interface pattern: %s, expect format like interface=ens1f0* or cidr=192.168.1.0/24", pattern)
	}
	m.Method = patternList[0]
	m.Value = patternList[1]
	return nil
}

type DetectMethod string

const (
	LLDPCLI              = "lldpcli"
	LLDP    DetectMethod = "LLDP"
	ARP     DetectMethod = "ARP"
)

// LLDP JSON parsing structures for lldpcli output
// These structures match the JSON format returned by 'lldpcli show neighbors -f json'

// LLDPCliResponse represents the root structure of LLDP JSON output from lldpcli
type LLDPCliResponse struct {
	LLDP any `json:"lldp"`
}

// LLDPData contains the main LLDP data with interface information
type LLDPData struct {
	Interface any `json:"interface"`
}

// LLDPJSON0Response represents the root structure of LLDP JSON0 output format
type LLDPJSON0Response struct {
	LLDP []LLDPJSON0Data `json:"lldp"`
}

// LLDPJSON0Data contains the main LLDP data with interface information in JSON0 format
type LLDPJSON0Data struct {
	Interface []LLDPJSON0Interface `json:"interface"`
}

// LLDPJSON0Interface represents a single interface in JSON0 format
type LLDPJSON0Interface struct {
	Name    string             `json:"name"`
	Via     string             `json:"via"`
	RID     string             `json:"rid"`
	Age     string             `json:"age"`
	Chassis []LLDPJSON0Chassis `json:"chassis"`
	Port    []LLDPJSON0Port    `json:"port"`
}

// LLDPJSON0Chassis represents chassis information in JSON0 format
type LLDPJSON0Chassis struct {
	ID         []LLDPJSON0IDValue    `json:"id"`
	Name       []LLDPJSON0Value      `json:"name"`
	Descr      []LLDPJSON0Value      `json:"descr"`
	MgmtIP     []LLDPJSON0Value      `json:"mgmt-ip,omitempty"`
	Capability []LLDPJSON0Capability `json:"capability,omitempty"`
}

// LLDPJSON0Port represents port information in JSON0 format
type LLDPJSON0Port struct {
	ID    []LLDPJSON0IDValue `json:"id"`
	Descr []LLDPJSON0Value   `json:"descr"`
	TTL   []LLDPJSON0Value   `json:"ttl"`
}

// LLDPJSON0IDValue represents ID with type and value in JSON0 format
type LLDPJSON0IDValue struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// LLDPJSON0Value represents a simple value wrapper in JSON0 format
type LLDPJSON0Value struct {
	Value string `json:"value"`
}

// LLDPJSON0Capability represents capability information in JSON0 format
type LLDPJSON0Capability struct {
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`
}

// LLDPInterfaceInfo contains LLDP information for a specific interface
type LLDPInterfaceInfo struct {
	Via         string           `json:"via"`
	RID         string           `json:"rid"`
	Age         string           `json:"age"`
	Chassis     LLDPChassisInfo  `json:"chassis"`
	Port        LLDPPortInfo     `json:"port"`
	UnknownTLVs *LLDPUnknownTLVs `json:"unknown-tlvs,omitempty"`
}

// LLDPChassisInfo contains chassis (neighbor device) information
// This is a map where the key is the hostname/chassis name
type LLDPChassisInfo map[string]LLDPChassisData

// LLDPChassisData contains detailed chassis information
type LLDPChassisData struct {
	ID struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"id"`
	Descr      string `json:"descr"`
	MgmtIP     any    `json:"mgmt-ip,omitempty"`
	Capability []struct {
		Type    string `json:"type"`
		Enabled bool   `json:"enabled"`
	} `json:"capability,omitempty"`
}

// getMgmtIPs get mgmt ip list from mgmt ip, mgmt ip can be string or []any or []string
func getMgmtIPs(mgmtIP any) []string {
	var ips []string

	switch v := mgmtIP.(type) {
	case string:
		// single ip
		ips = []string{v}
	case []any:
		// multiple ip as interface array
		for _, ip := range v {
			if ipStr, ok := ip.(string); ok {
				ips = append(ips, ipStr)
			}
		}
	case []string:
		// multiple ip as string array
		ips = v
	}

	return ips
}

// LLDPPortInfo contains port information from the neighbor
type LLDPPortInfo struct {
	ID struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"id"`
	Descr string `json:"descr"`
	TTL   string `json:"ttl"`
}

// LLDPUnknownTLVs contains unknown TLV information
type LLDPUnknownTLVs struct {
	UnknownTLV struct {
		OUI     string `json:"oui"`
		Subtype string `json:"subtype"`
		Len     string `json:"len"`
		Value   string `json:"value"`
	} `json:"unknown-tlv"`
}

// LldpCliShowNeighbors execute lldpcli show neighbors -f json0, why is json0 rather than json?
// We found that in all cases, json0 maintains a stable structure, while in some special scenarios,
// the JSON format might change, leading to JSON deserialization failures.
func LldpCliShowNeighbors() ([]byte, error) {
	cmd := exec.Command(LLDPCLI, "show", "neighbors", "-f", "json0")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get lldp info: %w", err)
	}
	return output, nil
}

// GetLLDPNeighbors executes lldpcli and returns neighbor information for each interface
func GetLLDPNeighbors(logger *slog.Logger) (map[string]v1beta1.LLDPNeighbor, error) {
	output, err := LldpCliShowNeighbors()
	if err != nil {
		return nil, err
	}

	return ParseLLDPNeighbors(logger, output)
}

func ParseLLDPNeighbors(logger *slog.Logger, output []byte) (map[string]v1beta1.LLDPNeighbor, error) {
	// First try to detect if this is json0 format
	var json0Resp LLDPJSON0Response
	if err := json.Unmarshal(output, &json0Resp); err != nil {
		return nil, fmt.Errorf("Unmarshal: %w", err)
	}

	if len(json0Resp.LLDP) > 0 && len(json0Resp.LLDP[0].Interface) > 0 {
		logger.Debug("lldp neighbors", "response", string(output))
		return parseJSON0Format(json0Resp), nil
	}

	return nil, fmt.Errorf("invalid lldp response: %s", string(output))
}

// parseJSON0Format parses the JSON0 format LLDP data
func parseJSON0Format(resp LLDPJSON0Response) map[string]v1beta1.LLDPNeighbor {
	neighbors := make(map[string]v1beta1.LLDPNeighbor)

	// Process each LLDP data block
	for _, lldpData := range resp.LLDP {
		// Process each interface
		for _, iface := range lldpData.Interface {
			interfaceName := iface.Name

			// Skip if no chassis information
			if len(iface.Chassis) == 0 || len(iface.Port) == 0 {
				continue
			}

			// Get chassis information
			chassis := iface.Chassis[0]
			port := iface.Port[0]

			// Skip if no ID information
			if len(chassis.ID) == 0 || len(port.ID) == 0 || len(chassis.Name) == 0 {
				continue
			}

			// Create neighbor information
			neighbor := v1beta1.LLDPNeighbor{
				Hostname:    chassis.Name[0].Value,
				Mac:         chassis.ID[0].Value,
				Port:        port.ID[0].Value,
				Description: chassis.Descr[0].Value,
			}

			// Process management IPs if available
			if len(chassis.MgmtIP) > 0 {
				var mgmtIPs []string
				for _, ip := range chassis.MgmtIP {
					mgmtIPs = append(mgmtIPs, ip.Value)
				}

				// Set IP (prioritize IPv4)
				if len(mgmtIPs) > 0 {
					for _, ip := range mgmtIPs {
						if net.ParseIP(ip).To4() != nil {
							neighbor.MgmtIP = ip
							break
						}
					}

					// If no IPv4 found, use the first IP
					if neighbor.MgmtIP == "" && len(mgmtIPs) > 0 {
						neighbor.MgmtIP = mgmtIPs[0]
					}
				}
			}

			neighbors[interfaceName] = neighbor
		}
	}

	return neighbors
}

func parseJSONFormat(resp LLDPCliResponse) map[string]v1beta1.LLDPNeighbor {
	neighbors := make(map[string]v1beta1.LLDPNeighbor)
	// Process based on LLDP field type
	switch lldpData := resp.LLDP.(type) {
	// Standard format with LLDPData structure
	case map[string]interface{}:
		// Get the interface field
		interfaceField, ok := lldpData["interface"]
		if !ok {
			return neighbors
		}

		// Process the interface field based on its type
		switch interfaces := interfaceField.(type) {
		// Case 1: interface is an object {"interface": {"eth0": {...}, "eth1": {...}}}
		case map[string]interface{}:
			// Process each interface directly
			for interfaceName, info := range interfaces {
				processInterface(interfaceName, info, neighbors)
			}

		// Case 2: interface is an array {"interface": [{"eth0": {...}}, {"eth1": {...}}]}
		case []interface{}:
			// Iterate through each item in the array
			for _, item := range interfaces {
				interfaceMap, ok := item.(map[string]interface{})
				if !ok {
					continue
				}

				// Process each interface
				for interfaceName, info := range interfaceMap {
					processInterface(interfaceName, info, neighbors)
				}
			}
		}
	}

	return neighbors
}

// processInterface processes a single interface's LLDP information and extracts neighbor data
func processInterface(interfaceName string, info interface{}, neighbors map[string]v1beta1.LLDPNeighbor) {
	lldpInfo, ok := info.(map[string]interface{})
	if !ok {
		return
	}

	// Process chassis information
	chassis, ok := lldpInfo["chassis"].(map[string]interface{})
	if !ok {
		return
	}

	// For each chassis entry
	for hostname, chassisDataRaw := range chassis {
		chassisData, ok := chassisDataRaw.(map[string]interface{})
		if !ok {
			continue
		}

		// Get ID information
		idData, ok := chassisData["id"].(map[string]interface{})
		if !ok {
			continue
		}

		// Get port information
		port, ok := lldpInfo["port"].(map[string]interface{})
		if !ok {
			continue
		}

		portID, ok := port["id"].(map[string]interface{})
		if !ok {
			continue
		}

		// Create neighbor information
		neighbor := v1beta1.LLDPNeighbor{
			Hostname: hostname,
			Mac:      fmt.Sprint(idData["value"]),
			Port:     fmt.Sprint(portID["value"]),
		}

		// Process mgmt-ip
		if mgmtIPRaw, exists := chassisData["mgmt-ip"]; exists {
			mgmtIPs := getMgmtIPs(mgmtIPRaw)

			// Set IP (prioritize IPv4)
			if len(mgmtIPs) > 0 {
				for _, ip := range mgmtIPs {
					if net.ParseIP(ip).To4() != nil {
						neighbor.MgmtIP = ip
						break
					}
				}

				// If no IPv4 found, use the first IP
				if neighbor.MgmtIP == "" && len(mgmtIPs) > 0 {
					neighbor.MgmtIP = mgmtIPs[0]
				}
			}
		}

		neighbors[interfaceName] = neighbor
		break // Only take the first chassis entry for each interface
	}
}
