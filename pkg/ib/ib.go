// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

// Package ib parses ibnetdiscover output.
//
// The line formats, switch name extraction and node name resolution follow
// github.com/dsx-ai-factory/topograph pkg/ib/ib.go (Apache-2.0,
// Copyright (c) 2024, NVIDIA CORPORATION. All rights reserved.), v1.1.0, so the
// results stay compatible with the labels Topograph writes.
package ib

import (
	"bufio"
	"bytes"
	"net"
	"regexp"
	"slices"
	"strings"
)

var (
	reEmptyLine  = regexp.MustCompile(`^\s*$`)
	reHCA        = regexp.MustCompile(`^Ca\s+\d+\s+"([^"]+)"\s+# "([^"]+)"`)
	reSwitch     = regexp.MustCompile(`^Switch\s+\d+\s+"([^"]+)"\s+# "([^"]+)"`)
	reConn       = regexp.MustCompile(`^\[\d+\]\s+"([^"]+)"\[\d+\](\([0-9a-f]+\))?\s+# "([^"]+)"`)
	reSwitchName = regexp.MustCompile(`^[^;:]+;([^;:]+):[^;:]+$`)
	reNodeName   = regexp.MustCompile(`^(\S+)\s\S+$`)
)

// Switch is one switch block of the ibnetdiscover output.
type Switch struct {
	ID   string
	Name string
	// Links counts the links to each remote GUID (switch or HCA).
	Links map[string]int32
}

// Discovered is a switch with its connections reduced to what unifabric keeps:
// cluster Nodes and other switches.
type Discovered struct {
	ID   string
	Name string
	// Nodes maps a cluster Node name to the number of links.
	Nodes map[string]int32
	// Switches maps the GUID of a neighbor switch to the number of links.
	Switches map[string]int32
}

// Discover parses the ibnetdiscover output and returns every switch with its
// links to the given cluster Nodes and to other switches. Hosts that do not
// match a cluster Node are dropped, like in Topograph.
func Discover(data []byte, clusterNodes map[string]bool) ([]*Discovered, error) {
	switches, hca, err := Parse(data)
	if err != nil {
		return nil, err
	}
	hca = ResolveNodeNames(hca, clusterNodes)

	result := make([]*Discovered, 0, len(switches))
	for _, sw := range switches {
		d := &Discovered{ID: sw.ID, Name: sw.Name, Nodes: map[string]int32{}, Switches: map[string]int32{}}
		for id, links := range sw.Links {
			if node, ok := hca[id]; ok {
				if clusterNodes[node] {
					d.Nodes[node] += links
				}
				continue
			}
			if _, ok := switches[id]; ok && id != sw.ID {
				d.Switches[id] += links
			}
		}
		result = append(result, d)
	}
	slices.SortFunc(result, func(a, b *Discovered) int { return strings.Compare(a.ID, b.ID) })
	return result, nil
}

// Parse processes the output of ibnetdiscover. It returns the switches by GUID
// and the HCA GUID to host name mapping.
func Parse(data []byte) (map[string]*Switch, map[string]string, error) {
	switches := make(map[string]*Switch)
	hca := make(map[string]string)
	var entry *Switch

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "#") {
			continue
		}

		if reEmptyLine.MatchString(line) {
			if entry != nil {
				switches[entry.ID] = entry
				entry = nil
			}
			continue
		}

		if match := reHCA.FindStringSubmatch(line); len(match) != 0 {
			if nodeName := extractNodeName(match[2]); len(nodeName) != 0 {
				hca[match[1]] = nodeName
			}
			continue
		}

		if match := reSwitch.FindStringSubmatch(line); len(match) != 0 {
			entry = &Switch{
				ID:    match[1],
				Name:  extractSwitchName(match[2]),
				Links: make(map[string]int32),
			}
			continue
		}

		if match := reConn.FindStringSubmatch(line); len(match) != 0 && entry != nil {
			entry.Links[match[1]]++
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	if entry != nil {
		switches[entry.ID] = entry
	}
	return switches, hca, nil
}

func extractSwitchName(name string) string {
	if m := reSwitchName.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	if !strings.ContainsAny(name, " ;:") {
		return name
	}
	return ""
}

func extractNodeName(name string) string {
	if m := reNodeName.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	return ""
}

// ResolveNodeNames maps HCA host names to cluster node names.
// The HCA node description usually carries the short host name, while the
// cluster may register nodes by their FQDN, or the other way round. Names that
// match exactly are kept. Otherwise the short names (up to the first dot) are
// compared, if at least one of the two names is short, and a match is used only
// when it identifies exactly one cluster node. Two different FQDNs never match.
//
// A fallback match is also dropped when another HCA reports a different host
// name for the same node, so that a node is never placed under the leaf
// switches of two hosts. Several HCAs reporting the same host name (one host
// with several HCAs) are kept.
func ResolveNodeNames(hca map[string]string, nodesInCluster map[string]bool) map[string]string {
	byShortName := make(map[string][]string)
	for node := range nodesInCluster {
		short := shortHostName(node)
		byShortName[short] = append(byShortName[short], node)
	}

	resolved := make(map[string]string, len(hca))
	reported := make(map[string]map[string]bool)
	fallback := make(map[string]string)
	for id, name := range hca {
		resolved[id] = name
		if nodesInCluster[name] {
			addReported(reported, name, name)
			continue
		}
		var candidates []string
		for _, node := range byShortName[shortHostName(name)] {
			if isShortHostName(name) || isShortHostName(node) {
				candidates = append(candidates, node)
			}
		}
		if len(candidates) == 1 {
			fallback[id] = candidates[0]
			addReported(reported, candidates[0], name)
		}
	}

	for id, node := range fallback {
		if names := reported[node]; len(names) > 1 {
			continue
		}
		resolved[id] = node
	}
	return resolved
}

func addReported(reported map[string]map[string]bool, node, name string) {
	if reported[node] == nil {
		reported[node] = make(map[string]bool)
	}
	reported[node][name] = true
}

func isShortHostName(name string) bool {
	return shortHostName(name) == name
}

func shortHostName(name string) string {
	if net.ParseIP(name) != nil {
		return name
	}
	short, _, _ := strings.Cut(name, ".")
	return short
}
