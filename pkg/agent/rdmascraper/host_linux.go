// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package rdmascraper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/safchain/ethtool"
)

func (s *RuntimeScraper) collectHost(ctx context.Context, snapshot *ScrapeSnapshot) hostCollection {
	if err := ctx.Err(); err != nil {
		snapshot.AddWarning(scrapeWarning(MetricScopeHost, "", "", "", "", "host scrape skipped because context is done", err))
		return hostCollection{}
	}

	var collection hostCollection
	collect := func() error {
		var err error
		collection, err = s.collectHostSysfs(snapshot)
		return err
	}

	if s.paths.hostMountNSPID > 0 {
		var collectErr error
		nsErr := withMountNamespace(s.paths.hostProcPath, s.paths.hostMountNSPID, func() error {
			collectErr = collect()
			return nil
		})
		if nsErr != nil {
			snapshot.AddWarning(scrapeWarning(MetricScopeHost, "", "", "", s.hostMountNamespacePath(), "failed to enter host mount namespace", nsErr))
			if err := collect(); err != nil {
				snapshot.AddWarning(scrapeWarning(MetricScopeHost, "", "", "", s.paths.infinibandClassPath, "failed to collect host RDMA metrics", err))
			}
		} else if collectErr != nil {
			snapshot.AddWarning(scrapeWarning(MetricScopeHost, "", "", "", s.paths.infinibandClassPath, "failed to collect host RDMA metrics", collectErr))
		}
	} else if err := collect(); err != nil {
		snapshot.AddWarning(scrapeWarning(MetricScopeHost, "", "", "", s.paths.infinibandClassPath, "failed to collect host RDMA metrics", err))
	}

	s.collectHostEthtool(snapshot, collection.rootIfnames)
	return collection
}

func (s *RuntimeScraper) collectHostSysfs(snapshot *ScrapeSnapshot) (hostCollection, error) {
	pciToIfname := s.buildPciToIfnameMap()
	pciToPfIfname := s.buildPciToPfIfnameMap()
	collection := hostCollection{
		pciToPfIfname: pciToPfIfname,
	}

	devices, err := os.ReadDir(s.paths.infinibandClassPath)
	if err != nil {
		return collection, fmt.Errorf("list infiniband devices: %w", err)
	}

	for _, dev := range devices {
		deviceName := dev.Name()
		ifname, err := s.getIfNameForInfinibandDevice(deviceName, pciToIfname)
		if err != nil {
			snapshot.AddWarning(scrapeWarning(MetricScopeHost, deviceName, "", "", s.devicePath(deviceName, "device"), "no matching netdev for infiniband device", err))
		}
		parentIfname := s.resolveParentIfname(ifname, pciToPfIfname)
		if ifname != "" && parentIfname == ifname {
			collection.rootIfnames = append(collection.rootIfnames, ifname)
		}

		ports, err := s.readRDMAPorts(deviceName)
		if err != nil {
			snapshot.AddWarning(scrapeWarning(MetricScopeHost, deviceName, ifname, "", s.devicePath(deviceName, "ports"), "failed to list RDMA ports", err))
			continue
		}
		snapshot.Devices.AddDevice(RDMADevice{
			Name:         deviceName,
			Provider:     rdmaDeviceProvider(deviceName),
			Ifname:       ifname,
			ParentIfname: parentIfname,
			Ports:        ports,
		})

		s.collectDeviceTOS(snapshot, deviceName, ifname, parentIfname)
		for _, port := range ports {
			s.collectCounterDirectory(snapshot, deviceName, ifname, parentIfname, port.Name, MetricSourceHWCounters)
			s.collectCounterDirectory(snapshot, deviceName, ifname, parentIfname, port.Name, MetricSourceCounters)
			s.collectInterfaceSamples(snapshot, deviceName, ifname, parentIfname, port.Name)
		}
	}

	return collection, nil
}

func (s *RuntimeScraper) readRDMAPorts(device string) ([]RDMAPort, error) {
	portsPath := s.devicePath(device, "ports")
	portEntries, err := os.ReadDir(portsPath)
	if err != nil {
		return nil, err
	}
	ports := make([]RDMAPort, 0, len(portEntries))
	for _, port := range portEntries {
		ports = append(ports, RDMAPort{Name: port.Name()})
	}
	return ports, nil
}

func (s *RuntimeScraper) collectCounterDirectory(snapshot *ScrapeSnapshot, device, ifname, parentIfname, port string, source MetricSource) {
	dirPath := s.counterDirectoryPath(device, port, source)
	files, err := os.ReadDir(dirPath)
	if err != nil {
		if !os.IsNotExist(err) && !os.IsPermission(err) {
			snapshot.AddWarning(scrapeWarning(MetricScopeHost, device, ifname, port, dirPath, "failed to list RDMA counter directory", err))
		}
		return
	}

	for _, f := range files {
		if !f.Type().IsRegular() {
			continue
		}
		valuePath := filepath.Join(dirPath, f.Name())
		value, ok := readCounterValue(valuePath)
		if !ok {
			continue
		}
		snapshot.AddSample(s.hostSample(f.Name(), value, source, device, ifname, parentIfname, port, ""))
	}
}

func (s *RuntimeScraper) collectInterfaceSamples(snapshot *ScrapeSnapshot, device, ifname, parentIfname, port string) {
	for _, spec := range []struct {
		name string
		path string
		read func(string) (float64, error)
	}{
		{name: "port_speed_mbps", path: s.netDevicePath(ifname, "speed"), read: readFloatFile},
		{name: "port_mtu", path: s.netDevicePath(ifname, "mtu"), read: readFloatFile},
		{name: "port_oper_state", path: s.netDevicePath(ifname, "operstate"), read: readOperStateFile},
	} {
		value, err := spec.read(spec.path)
		if err != nil {
			snapshot.AddWarning(scrapeWarning(MetricScopeHost, device, ifname, port, spec.path, "failed to collect interface metric", err))
			continue
		}
		snapshot.AddSample(s.hostSample(spec.name, value, MetricSourceInterface, device, ifname, parentIfname, port, ""))
	}
}

// collectDeviceTOS reads /sys/class/infiniband/<dev>/tc/<port>/traffic_class.
func (s *RuntimeScraper) collectDeviceTOS(snapshot *ScrapeSnapshot, device, ifname, parentIfname string) {
	path := s.devicePath(device, "tc", "1", "traffic_class")
	value, err := readDeviceTOS(path)
	if err != nil {
		if !os.IsNotExist(err) {
			snapshot.AddWarning(scrapeWarning(MetricScopeHost, device, ifname, "", path, "failed to read or parse RDMA device ToS", err))
		}
		snapshot.AddSample(s.hostSample("rdma_device_tos", 0, MetricSourceDevice, device, ifname, parentIfname, "", ""))
		return
	}
	snapshot.AddSample(s.hostSample("rdma_device_tos", value, MetricSourceDevice, device, ifname, parentIfname, "", ""))
}

// collectHostEthtool collects the per-priority pause and discard counters that
// some NIC drivers, including Mellanox drivers, expose through ethtool stats:
// rx_prio<N>_pause, tx_prio<N>_pause, rx_prio<N>_discards, and
// tx_prio<N>_discards.
// These per-priority counters are not part of a standard sysfs ABI, so the
// scraper reads them through the kernel ethtool interface instead of trying to
// depend on driver-specific files.
func (s *RuntimeScraper) collectHostEthtool(snapshot *ScrapeSnapshot, rootIfnames []string) {
	if s.paths.hostNetNSPath == "" || len(rootIfnames) == 0 {
		return
	}

	err := ns.WithNetNSPath(s.paths.hostNetNSPath, func(hostNS ns.NetNS) error {
		et, err := ethtool.NewEthtool()
		if err != nil {
			return fmt.Errorf("init ethtool: %w", err)
		}
		defer et.Close()

		for _, ifname := range rootIfnames {
			stats, err := et.Stats(ifname)
			if err != nil {
				snapshot.AddWarning(scrapeWarning(MetricScopeHost, "", ifname, "", s.paths.hostNetNSPath, "failed to read ethtool stats", err))
				continue
			}
			for statName, statValue := range stats {
				name, priority, ok := extractPriorityMetric(statName)
				if !ok {
					continue
				}
				snapshot.AddSample(s.hostSample(name, float64(statValue), MetricSourceEthtool, "", ifname, ifname, "", strconv.Itoa(priority)))
			}
		}
		return nil
	})
	if err != nil {
		snapshot.AddWarning(scrapeWarning(MetricScopeHost, "", "", "", s.paths.hostNetNSPath, "failed to enter host net namespace", err))
	}
}

func (s *RuntimeScraper) hostSample(name string, value float64, source MetricSource, device, ifname, parentIfname, port, priority string) MetricSample {
	return MetricSample{
		Name:         name,
		Value:        value,
		Scope:        MetricScopeHost,
		Source:       source,
		Device:       device,
		Ifname:       ifname,
		ParentIfname: parentIfname,
		Port:         port,
		Priority:     priority,
		IsRoot:       ifname != "" && ifname == parentIfname,
		Kind:         interfaceKind(s.kindMatcher, ifname, parentIfname),
	}
}

func (s *RuntimeScraper) buildPciToIfnameMap() map[string]string {
	m := make(map[string]string)
	entries, err := os.ReadDir(s.paths.netClassPath)
	if err != nil {
		return m
	}
	for _, entry := range entries {
		ifname := entry.Name()
		if shouldSkipNetDevice(ifname) {
			continue
		}
		devPath := s.netDevicePath(ifname, "device")
		absPci, err := filepath.EvalSymlinks(devPath)
		if err != nil {
			continue
		}
		m[absPci] = ifname
	}
	return m
}

func (s *RuntimeScraper) buildPciToPfIfnameMap() map[string]string {
	m := make(map[string]string)
	entries, err := os.ReadDir(s.paths.netClassPath)
	if err != nil {
		return m
	}
	for _, entry := range entries {
		ifname := entry.Name()
		if shouldSkipNetDevice(ifname) {
			continue
		}
		devPath := s.netDevicePath(ifname, "device")
		absPci, err := filepath.EvalSymlinks(devPath)
		if err != nil {
			continue
		}
		physfnPath := filepath.Join(devPath, "physfn")
		if _, err := os.Lstat(physfnPath); os.IsNotExist(err) {
			m[absPci] = ifname
		}
	}
	return m
}

func (s *RuntimeScraper) getIfNameForInfinibandDevice(device string, pciToIfname map[string]string) (string, error) {
	if ifname, ok := s.getIfNameFromGIDAttrs(device); ok {
		return ifname, nil
	}

	devicePath := s.devicePath(device, "device")
	absPciPath, err := filepath.EvalSymlinks(devicePath)
	if err != nil {
		return "", err
	}
	ifname, ok := pciToIfname[absPciPath]
	if !ok {
		return "", fmt.Errorf("no matching netdev for %s", device)
	}
	return ifname, nil
}

func (s *RuntimeScraper) getIfNameFromGIDAttrs(device string) (string, bool) {
	ports, err := s.readRDMAPorts(device)
	if err != nil {
		return "", false
	}

	for _, port := range ports {
		ndevsPath := s.devicePath(device, "ports", port.Name, "gid_attrs", "ndevs")
		entries, err := os.ReadDir(ndevsPath)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			content, err := os.ReadFile(filepath.Join(ndevsPath, entry.Name()))
			if err != nil {
				continue
			}
			ifname := strings.TrimSpace(string(content))
			if ifname == "" {
				continue
			}
			if _, err := os.Stat(s.netDevicePath(ifname)); err != nil {
				continue
			}
			return ifname, true
		}
	}
	return "", false
}

func (s *RuntimeScraper) resolveParentIfname(ifname string, pciToPfIfname map[string]string) string {
	parentIfname := ifname
	if ifname == "" {
		return parentIfname
	}

	physfnPath := s.netDevicePath(ifname, "device", "physfn")
	pfPciPath, err := filepath.EvalSymlinks(physfnPath)
	if err == nil {
		if pfIfname, ok := pciToPfIfname[pfPciPath]; ok {
			parentIfname = pfIfname
		}
	}
	return parentIfname
}

func shouldSkipNetDevice(ifname string) bool {
	return ifname == "lo" || strings.HasPrefix(ifname, "cali")
}

func rdmaDeviceProvider(device string) DeviceProvider {
	switch {
	case strings.HasPrefix(device, "mlx5_"):
		return DeviceProviderMLX5
	case strings.HasPrefix(device, "rxe_"):
		return DeviceProviderRXE
	default:
		return DeviceProviderUnknown
	}
}

func extractPriorityMetric(statName string) (name string, priority int, ok bool) {
	if !isPriorityMetric(statName) {
		return "", 0, false
	}

	parts := strings.Split(statName, "_")
	if len(parts) != 3 {
		return "", 0, false
	}
	rawPriority := parts[1]
	if !strings.HasPrefix(rawPriority, "prio") {
		return "", 0, false
	}
	priority, err := strconv.Atoi(strings.TrimPrefix(rawPriority, "prio"))
	if err != nil {
		return "", 0, false
	}
	return parts[0] + "_" + parts[2], priority, true
}

func isPriorityMetric(statName string) bool {
	if len(statName) == 14 {
		return (strings.HasPrefix(statName, "rx_prio") || strings.HasPrefix(statName, "tx_prio")) && strings.HasSuffix(statName, "_pause")
	}
	if len(statName) == 17 {
		return (strings.HasPrefix(statName, "rx_prio") || strings.HasPrefix(statName, "tx_prio")) && strings.HasSuffix(statName, "_discards")
	}
	return false
}

func scrapeWarning(scope MetricScope, device, ifname, port, path, message string, err error) ScrapeWarning {
	warning := ScrapeWarning{
		Scope:   scope,
		Device:  device,
		Ifname:  ifname,
		Port:    port,
		Path:    path,
		Message: message,
	}
	if err != nil {
		warning.Error = err.Error()
	}
	return warning
}
