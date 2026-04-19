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

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
)

func (s *RuntimeScraper) collectPods(ctx context.Context, snapshot *ScrapeSnapshot, pods []v1beta1.RdmaPod, hostSamples []MetricSample, host hostCollection) {
	for i := range pods {
		if err := ctx.Err(); err != nil {
			snapshot.AddWarning(scrapeWarning(MetricScopePod, "", "", "", "", "pod scrape stopped because context is done", err))
			return
		}

		pod := pods[i]
		workload := workloadLabelsForPod(pod)
		if pod.HostRDMA {
			s.collectHostRDMAPod(snapshot, hostSamples, workload)
			continue
		}
		s.collectNamespacedPod(snapshot, pod, workload, host)
	}
}

func (s *RuntimeScraper) collectHostRDMAPod(snapshot *ScrapeSnapshot, hostSamples []MetricSample, workload WorkloadLabels) {
	for _, sample := range hostSamples {
		if sample.Name == "rdma_device_tos" {
			continue
		}
		sample.Scope = MetricScopePod
		sample.Workload = workload
		snapshot.AddSample(sample)
	}
}

func (s *RuntimeScraper) collectNamespacedPod(snapshot *ScrapeSnapshot, pod v1beta1.RdmaPod, workload WorkloadLabels, host hostCollection) {
	if len(pod.ContainerList) == 0 {
		snapshot.AddWarning(scrapeWarning(MetricScopePod, "", "", "", "", fmt.Sprintf("RDMA pod %s/%s has no container IDs", pod.Namespace, pod.Name), nil))
		return
	}

	for _, containerID := range pod.ContainerList {
		pid, err := s.containerInitPID(containerID)
		if err != nil {
			snapshot.AddWarning(scrapeWarning(MetricScopePod, "", "", "", s.containerInitPIDPath(containerID), "failed to resolve container init pid", err))
			continue
		}

		sampleCountBefore := len(snapshot.Samples)
		var ifNameToDevice map[string]string
		var collectErr error
		nsErr := withMountNamespace(s.paths.hostProcPath, pid, func() error {
			ifNameToDevice, collectErr = s.collectPodSysfs(snapshot, pod, workload, host.pciToPfIfname)
			return nil
		})
		if nsErr != nil {
			snapshot.AddWarning(scrapeWarning(MetricScopePod, "", "", "", s.mountNamespacePath(pid), "failed to enter pod mount namespace", nsErr))
			continue
		}
		if collectErr != nil {
			snapshot.AddWarning(scrapeWarning(MetricScopePod, "", "", "", s.paths.infinibandClassPath, "failed to collect pod RDMA sysfs metrics", collectErr))
		}

		ethtoolBefore := len(snapshot.Samples)
		if len(ifNameToDevice) > 0 {
			s.collectPodEthtool(snapshot, workload, pid, ifNameToDevice)
		}
		if len(snapshot.Samples) > sampleCountBefore || len(snapshot.Samples) > ethtoolBefore {
			return
		}
	}
}

func (s *RuntimeScraper) collectPodSysfs(snapshot *ScrapeSnapshot, pod v1beta1.RdmaPod, workload WorkloadLabels, hostPciToPfIfname map[string]string) (map[string]string, error) {
	pciToIfname := s.buildPciToIfnameMap()
	devices, err := os.ReadDir(s.paths.infinibandClassPath)
	if err != nil {
		return nil, fmt.Errorf("list pod infiniband devices for %s/%s: %w", pod.Namespace, pod.Name, err)
	}

	ifNameToDevice := make(map[string]string, len(devices))
	for _, dev := range devices {
		deviceName := dev.Name()
		ifname, err := s.getIfNameForInfinibandDevice(deviceName, pciToIfname)
		if err != nil {
			snapshot.AddWarning(scrapeWarning(MetricScopePod, deviceName, "", "", s.devicePath(deviceName, "device"), "no matching pod netdev for infiniband device", err))
			continue
		}
		parentIfname := s.resolveParentIfname(ifname, hostPciToPfIfname)
		ports, err := s.readRDMAPorts(deviceName)
		if err != nil {
			snapshot.AddWarning(scrapeWarning(MetricScopePod, deviceName, ifname, "", s.devicePath(deviceName, "ports"), "failed to list pod RDMA ports", err))
			continue
		}
		for _, port := range ports {
			s.collectPodCounterDirectory(snapshot, workload, deviceName, ifname, parentIfname, port.Name, MetricSourceHWCounters)
			s.collectPodCounterDirectory(snapshot, workload, deviceName, ifname, parentIfname, port.Name, MetricSourceCounters)
		}
		s.collectPodInterfaceSamples(snapshot, workload, deviceName, ifname, parentIfname)
		s.collectPodDeviceTOS(snapshot, workload, deviceName, ifname, parentIfname)
		ifNameToDevice[ifname] = deviceName
	}
	return ifNameToDevice, nil
}

func (s *RuntimeScraper) collectPodCounterDirectory(snapshot *ScrapeSnapshot, workload WorkloadLabels, device, ifname, parentIfname, port string, source MetricSource) {
	dirPath := s.counterDirectoryPath(device, port, source)
	files, err := os.ReadDir(dirPath)
	if err != nil {
		if !os.IsNotExist(err) && !os.IsPermission(err) {
			snapshot.AddWarning(scrapeWarning(MetricScopePod, device, ifname, port, dirPath, "failed to list pod RDMA counter directory", err))
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
		snapshot.AddSample(s.podSample(f.Name(), value, source, workload, device, ifname, parentIfname, port, ""))
	}
}

func (s *RuntimeScraper) collectPodInterfaceSamples(snapshot *ScrapeSnapshot, workload WorkloadLabels, device, ifname, parentIfname string) {
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
			snapshot.AddWarning(scrapeWarning(MetricScopePod, device, ifname, "", spec.path, "failed to collect pod interface metric", err))
			continue
		}
		snapshot.AddSample(s.podSample(spec.name, value, MetricSourceInterface, workload, device, ifname, parentIfname, "", ""))
	}
}

// collectPodDeviceTOS reads /sys/class/infiniband/<dev>/tc/<port>/traffic_class.
func (s *RuntimeScraper) collectPodDeviceTOS(snapshot *ScrapeSnapshot, workload WorkloadLabels, device, ifname, parentIfname string) {
	path := s.devicePath(device, "tc", "1", "traffic_class")
	value, err := readDeviceTOS(path)
	if err != nil {
		if !os.IsNotExist(err) {
			snapshot.AddWarning(scrapeWarning(MetricScopePod, device, ifname, "", path, "failed to read or parse pod RDMA device ToS", err))
		}
		snapshot.AddSample(s.podSample("rdma_device_tos", 0, MetricSourceDevice, workload, device, ifname, parentIfname, "", ""))
		return
	}
	snapshot.AddSample(s.podSample("rdma_device_tos", value, MetricSourceDevice, workload, device, ifname, parentIfname, "", ""))
}

func (s *RuntimeScraper) collectPodEthtool(snapshot *ScrapeSnapshot, workload WorkloadLabels, pid int, ifNameToDevice map[string]string) {
	netnsPath := s.netNamespacePath(pid)
	err := ns.WithNetNSPath(netnsPath, func(hostNS ns.NetNS) error {
		et, err := ethtool.NewEthtool()
		if err != nil {
			return fmt.Errorf("init ethtool: %w", err)
		}
		defer et.Close()

		for ifname, device := range ifNameToDevice {
			stats, err := et.Stats(ifname)
			if err != nil {
				snapshot.AddWarning(scrapeWarning(MetricScopePod, device, ifname, "", netnsPath, "failed to read pod ethtool stats", err))
				continue
			}
			for statName, statValue := range stats {
				name, priority, ok := extractPriorityMetric(statName)
				if !ok {
					continue
				}
				snapshot.AddSample(s.podSample(name, float64(statValue), MetricSourceEthtool, workload, device, ifname, ifname, "", strconv.Itoa(priority)))
			}
		}
		return nil
	})
	if err != nil {
		snapshot.AddWarning(scrapeWarning(MetricScopePod, "", "", "", netnsPath, "failed to enter pod net namespace", err))
	}
}

func (s *RuntimeScraper) podSample(name string, value float64, source MetricSource, workload WorkloadLabels, device, ifname, parentIfname, port, priority string) MetricSample {
	return MetricSample{
		Name:         name,
		Value:        value,
		Scope:        MetricScopePod,
		Source:       source,
		Device:       device,
		Ifname:       ifname,
		ParentIfname: parentIfname,
		Port:         port,
		Priority:     priority,
		IsRoot:       ifname != "" && ifname == parentIfname,
		Kind:         interfaceKind(s.kindMatcher, ifname, parentIfname),
		Workload:     workload,
	}
}

func workloadLabelsForPod(pod v1beta1.RdmaPod) WorkloadLabels {
	labels := WorkloadLabels{
		PodName:      pod.Name,
		PodNamespace: pod.Namespace,
		HostRDMA:     pod.HostRDMA,
	}
	if pod.TopOwner != nil {
		labels.TopOwner = TopOwnerLabels{
			APIVersion: pod.TopOwner.APIVersion,
			Kind:       pod.TopOwner.Kind,
			Namespace:  pod.TopOwner.Namespace,
			Name:       pod.TopOwner.Name,
		}
	}
	return labels
}

func (s *RuntimeScraper) containerInitPID(containerID string) (int, error) {
	if !strings.HasPrefix(containerID, "containerd://") {
		return 0, fmt.Errorf("not a containerd container: %s", containerID)
	}
	pidPath := s.containerInitPIDPath(containerID)
	pidBytes, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", pidPath, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil {
		return 0, fmt.Errorf("parse pid from %s: %w", pidPath, err)
	}
	return pid, nil
}
