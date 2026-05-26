// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchtopology

import (
	"net"
	"regexp"
	"strings"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
)

type hostPeerRoleMatcher struct {
	scaleOut topologyInterfaceSelector
	storage  topologyInterfaceSelector
	scaleUp  topologyInterfaceSelector
}

type topologyInterfaceSelector struct {
	method string
	value  string
}

func newHostPeerRoleMatcher(cfg config.ControllerNodeTopologyConfig) hostPeerRoleMatcher {
	return hostPeerRoleMatcher{
		scaleOut: newTopologyInterfaceSelector(cfg.ScaleOutInterfaceSelector),
		storage:  newTopologyInterfaceSelector(cfg.StorageInterfaceSelector),
		scaleUp:  newTopologyInterfaceSelector(cfg.ScaleUpInterfaceSelector),
	}
}

func newTopologyInterfaceSelector(selector string) topologyInterfaceSelector {
	parts := strings.SplitN(selector, "=", 2)
	if len(parts) != 2 {
		return topologyInterfaceSelector{}
	}
	return topologyInterfaceSelector{method: parts[0], value: parts[1]}
}

func (m hostPeerRoleMatcher) roleForHostLink(remotePortID string, nic *v1beta1.NicInfo) v1beta1.SwitchRole {
	if m.storage.match(remotePortID, nic) {
		return v1beta1.SwitchRoleStorage
	}
	if m.scaleUp.match(remotePortID, nic) {
		return v1beta1.SwitchRoleScaleUp
	}
	if m.scaleOut.match(remotePortID, nic) {
		return v1beta1.SwitchRoleScaleOut
	}
	if remotePortID != "" && !m.scaleOut.isSet() {
		return v1beta1.SwitchRoleScaleOut
	}
	return ""
}

func (s topologyInterfaceSelector) isSet() bool {
	return s.method != "" && s.value != ""
}

func (s topologyInterfaceSelector) match(ifname string, nic *v1beta1.NicInfo) bool {
	if !s.isSet() {
		return false
	}
	switch s.method {
	case "interface":
		return matchMultipleInterfacePatterns(ifname, s.value)
	case "cidr":
		if nic == nil {
			return false
		}
		return nicMatchesCIDR(*nic, s.value)
	default:
		return false
	}
}

func matchMultipleInterfacePatterns(ifname, patternList string) bool {
	if patternList == "" {
		return false
	}

	patterns := strings.Split(patternList, ",")
	var inclusions, exclusions []string
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if strings.HasPrefix(pattern, "!") {
			exclusions = append(exclusions, strings.TrimPrefix(pattern, "!"))
		} else {
			inclusions = append(inclusions, pattern)
		}
	}

	for _, pattern := range exclusions {
		if matchInterfacePattern(ifname, pattern) {
			return false
		}
	}
	if len(inclusions) == 0 {
		return len(exclusions) > 0
	}
	for _, pattern := range inclusions {
		if matchInterfacePattern(ifname, pattern) {
			return true
		}
	}
	return false
}

func matchInterfacePattern(ifname, pattern string) bool {
	regexPattern := regexp.QuoteMeta(pattern)
	regexPattern = strings.ReplaceAll(regexPattern, `\*`, ".*")
	regexPattern = strings.ReplaceAll(regexPattern, `\?`, ".")
	matched, err := regexp.MatchString("^"+regexPattern+"$", ifname)
	return err == nil && matched
}

func nicMatchesCIDR(nic v1beta1.NicInfo, cidr string) bool {
	_, cidrNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	for _, addr := range []string{nic.IPv4, nic.IPv6} {
		ip := parseNICAddressIP(addr)
		if ip != nil && cidrNet.Contains(ip) {
			return true
		}
	}
	return false
}

func parseNICAddressIP(addr string) net.IP {
	if addr == "" {
		return nil
	}
	if strings.Contains(addr, "/") {
		ip, _, err := net.ParseCIDR(addr)
		if err == nil {
			return ip
		}
	}
	return net.ParseIP(addr)
}
