// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package rdmascraper

import (
	"path/filepath"
	"strconv"
	"strings"
)

type scraperPaths struct {
	infinibandClassPath string
	netClassPath        string
	hostNetNSPath       string
	hostProcPath        string
	containerdTaskPath  string
	hostMountNSPID      int
}

func defaultScraperPaths() scraperPaths {
	return scraperPaths{
		infinibandClassPath: "/sys/class/infiniband",
		netClassPath:        "/sys/class/net",
		hostNetNSPath:       "/host/proc/1/ns/net",
		hostProcPath:        "/host/proc",
		containerdTaskPath:  "/host/run/containerd/io.containerd.runtime.v2.task/k8s.io",
		hostMountNSPID:      1,
	}
}

func (s *RuntimeScraper) counterDirectoryPath(device, port string, source MetricSource) string {
	dir := "counters"
	if source == MetricSourceHWCounters {
		dir = "hw_counters"
	}
	return s.devicePath(device, "ports", port, dir)
}

func (s *RuntimeScraper) devicePath(device string, elem ...string) string {
	parts := append([]string{s.paths.infinibandClassPath, device}, elem...)
	return filepath.Join(parts...)
}

func (s *RuntimeScraper) netDevicePath(ifname string, elem ...string) string {
	parts := append([]string{s.paths.netClassPath, ifname}, elem...)
	return filepath.Join(parts...)
}

func (s *RuntimeScraper) hostMountNamespacePath() string {
	return filepath.Join(s.paths.hostProcPath, strconv.Itoa(s.paths.hostMountNSPID), "ns", "mnt")
}

func (s *RuntimeScraper) containerInitPIDPath(containerID string) string {
	realContainerID := strings.TrimPrefix(containerID, "containerd://")
	return filepath.Join(s.paths.containerdTaskPath, realContainerID, "init.pid")
}

func (s *RuntimeScraper) mountNamespacePath(pid int) string {
	return filepath.Join(s.paths.hostProcPath, strconv.Itoa(pid), "ns", "mnt")
}

func (s *RuntimeScraper) netNamespacePath(pid int) string {
	return filepath.Join(s.paths.hostProcPath, strconv.Itoa(pid), "ns", "net")
}
