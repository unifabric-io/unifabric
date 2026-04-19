// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package rdmascraper

import (
	"net"
	"regexp"
	"strings"

	"github.com/unifabric-io/unifabric/pkg/config"
)

const (
	rdmaInterfaceKindScaleOut = "scaleOut"
	rdmaInterfaceKindStorage  = "storage"
	rdmaInterfaceKindScaleUp  = "scaleUp"
)

type interfaceSelector struct {
	method string
	value  string
}

type interfaceKindMatcher struct {
	scaleOut interfaceSelector
	storage  interfaceSelector
	scaleUp  interfaceSelector
}

func buildInterfaceKindMatcher(cfg config.NodeTopologyDiscoveryConfig) interfaceKindMatcher {
	return interfaceKindMatcher{
		scaleOut: newInterfaceSelector(cfg.ScaleOutInterfaceSelector),
		storage:  newInterfaceSelector(cfg.StorageInterfaceSelector),
		scaleUp:  newInterfaceSelector(cfg.ScaleUpInterfaceSelector),
	}
}

func newInterfaceSelector(selector string) interfaceSelector {
	parts := strings.SplitN(selector, "=", 2)
	if len(parts) != 2 {
		return interfaceSelector{}
	}
	return interfaceSelector{
		method: parts[0],
		value:  parts[1],
	}
}

func interfaceKind(matcher interfaceKindMatcher, ifname, parentIfname string) string {
	hasCandidate := false
	for _, candidate := range []string{parentIfname, ifname} {
		if candidate == "" {
			continue
		}
		hasCandidate = true
		if kind := matcher.explicitKindForInterface(candidate); kind != "" {
			return kind
		}
	}
	if hasCandidate && !matcher.scaleOut.isSet() {
		return rdmaInterfaceKindScaleOut
	}
	return ""
}

func (m interfaceKindMatcher) explicitKindForInterface(ifname string) string {
	if m.storage.match(ifname) {
		return rdmaInterfaceKindStorage
	}
	if m.scaleUp.match(ifname) {
		return rdmaInterfaceKindScaleUp
	}
	if m.scaleOut.match(ifname) {
		return rdmaInterfaceKindScaleOut
	}
	return ""
}

func (s interfaceSelector) isSet() bool {
	return s.method != "" && s.value != ""
}

func (s interfaceSelector) match(ifname string) bool {
	if !s.isSet() {
		return false
	}
	switch s.method {
	case "interface":
		return matchMultipleInterfacePatterns(ifname, s.value)
	case "cidr":
		return matchInterfaceCIDR(ifname, s.value)
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

func matchInterfaceCIDR(ifname, cidr string) bool {
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		return false
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}
	_, cidrNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		ip := addrIP(addr)
		if ip != nil && cidrNet.Contains(ip) {
			return true
		}
	}
	return false
}

func addrIP(addr net.Addr) net.IP {
	switch v := addr.(type) {
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	default:
		return nil
	}
}
