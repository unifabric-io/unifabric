// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ibdiscovery

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	sysfsInfiniBandRoot = "/sys/class/infiniband"
	fileSubnet          = "file"
	ibnetdiscoverBin    = "ibnetdiscover"
	ibnetdiscoverLimit  = 2 * time.Minute
)

const gidPrefixLength = 4

var (
	caNamePattern  = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	portNumPattern = regexp.MustCompile(`^[0-9]+$`)
)

// ibPort is an active InfiniBand port and the subnet it belongs to.
type ibPort struct {
	CA     string
	Port   string
	Subnet string
}

// listActivePorts returns the active InfiniBand ports below root, sorted by
// CA name and port number. The subnet is the 64-bit prefix of GID index 0
// (colon-separated), which every port of one subnet shares.
func listActivePorts(root string) ([]ibPort, error) {
	cas, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var ports []ibPort
	for _, ca := range cas {
		if !caNamePattern.MatchString(ca.Name()) {
			continue
		}
		portDirs, err := os.ReadDir(filepath.Join(root, ca.Name(), "ports"))
		if err != nil {
			continue
		}
		for _, portDir := range portDirs {
			if !portNumPattern.MatchString(portDir.Name()) {
				continue
			}
			dir := filepath.Join(root, ca.Name(), "ports", portDir.Name())
			if !isActiveInfiniBand(dir) {
				continue
			}
			subnet, ok := readSubnetPrefix(filepath.Join(dir, "gids", "0"))
			if !ok {
				continue
			}
			ports = append(ports, ibPort{CA: ca.Name(), Port: portDir.Name(), Subnet: subnet})
		}
	}
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].CA != ports[j].CA {
			return ports[i].CA < ports[j].CA
		}
		return ports[i].Port < ports[j].Port
	})
	return ports, nil
}

func isActiveInfiniBand(portDir string) bool {
	state, err := os.ReadFile(filepath.Join(portDir, "state"))
	if err != nil || !strings.Contains(string(state), "ACTIVE") {
		return false
	}
	// RoCE ports are Ethernet and have no subnet to scan.
	layer, err := os.ReadFile(filepath.Join(portDir, "link_layer"))
	return err == nil && strings.TrimSpace(string(layer)) == "InfiniBand"
}

func readSubnetPrefix(gidFile string) (string, bool) {
	raw, err := os.ReadFile(gidFile)
	if err != nil {
		return "", false
	}
	groups := strings.Split(strings.ToLower(strings.TrimSpace(string(raw))), ":")
	if len(groups) != 8 {
		return "", false
	}
	return strings.Join(groups[:gidPrefixLength], ":"), true
}

// scanner returns the raw ibnetdiscover output of a subnet.
type scanner interface {
	Subnets() ([]string, error)
	Scan(ctx context.Context, subnet string) ([]byte, error)
}

// sysfsScanner probes the fabric: one ibnetdiscover run on one active port of
// the subnet. ibnetdiscover sees the whole subnet from any member port.
type sysfsScanner struct {
	root string
}

func (s sysfsScanner) Subnets() ([]string, error) {
	ports, err := listActivePorts(s.root)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var subnets []string
	for _, p := range ports {
		if _, ok := seen[p.Subnet]; !ok {
			seen[p.Subnet] = struct{}{}
			subnets = append(subnets, p.Subnet)
		}
	}
	return subnets, nil
}

func (s sysfsScanner) Scan(ctx context.Context, subnet string) ([]byte, error) {
	ports, err := listActivePorts(s.root)
	if err != nil {
		return nil, err
	}
	for _, p := range ports {
		if p.Subnet != subnet {
			continue
		}
		runCtx, cancel := context.WithTimeout(ctx, ibnetdiscoverLimit)
		out, err := exec.CommandContext(runCtx, ibnetdiscoverBin, "--Ca", p.CA, "--Port", p.Port).Output() //nolint:gosec // CA and port are validated against strict patterns.
		cancel()
		if err != nil {
			return nil, fmt.Errorf("ibnetdiscover on %s port %s: %w", p.CA, p.Port, err)
		}
		return out, nil
	}
	return nil, fmt.Errorf("no active port in subnet %s", subnet)
}

// fileScanner replays a saved ibnetdiscover output as a single synthetic subnet.
type fileScanner struct {
	path string
}

func (f fileScanner) Subnets() ([]string, error) { return []string{fileSubnet}, nil }

func (f fileScanner) Scan(context.Context, string) ([]byte, error) {
	return os.ReadFile(f.path)
}
