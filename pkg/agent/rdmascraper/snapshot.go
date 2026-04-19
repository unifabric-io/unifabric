// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package rdmascraper

type MetricScope string

const (
	MetricScopeHost MetricScope = "host"
	MetricScopePod  MetricScope = "pod"
)

type MetricSource string

const (
	MetricSourceCounters   MetricSource = "counters"
	MetricSourceHWCounters MetricSource = "hw_counters"
	MetricSourceInterface  MetricSource = "interface"
	MetricSourceDevice     MetricSource = "device"
	MetricSourceEthtool    MetricSource = "ethtool"
)

type DeviceProvider string

const (
	DeviceProviderUnknown DeviceProvider = "unknown"
	DeviceProviderMLX5    DeviceProvider = "mlx5"
	DeviceProviderRXE     DeviceProvider = "rxe"
)

type ScrapeSnapshot struct {
	NodeName string
	Devices  DeviceInventory
	Samples  []MetricSample
	Warnings []ScrapeWarning
}

func (s *ScrapeSnapshot) AddSample(sample MetricSample) {
	s.Samples = append(s.Samples, sample)
}

func (s *ScrapeSnapshot) AddWarning(warning ScrapeWarning) {
	s.Warnings = append(s.Warnings, warning)
}

type MetricSample struct {
	Name   string
	Value  float64
	Scope  MetricScope
	Source MetricSource

	Device       string
	Ifname       string
	ParentIfname string
	Port         string
	Priority     string
	IsRoot       bool
	Kind         string

	Workload WorkloadLabels
}

type WorkloadLabels struct {
	PodName      string
	PodNamespace string
	HostRDMA     bool
	TopOwner     TopOwnerLabels
}

type TopOwnerLabels struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
}

type DeviceInventory struct {
	Devices []RDMADevice
}

func (i *DeviceInventory) AddDevice(device RDMADevice) {
	i.Devices = append(i.Devices, device)
}

type RDMADevice struct {
	Name         string // mlx5_0 or rxe_eth1
	Provider     DeviceProvider
	Ifname       string // eth1
	ParentIfname string // PF/VF ifname for the device, empty if no parent or parent ifname cannot be determined
	Ports        []RDMAPort
}

func (d RDMADevice) IsRoot() bool {
	return d.Ifname != "" && d.Ifname == d.ParentIfname
}

type RDMAPort struct {
	Name string
}

type ScrapeWarning struct {
	Scope   MetricScope
	Device  string
	Ifname  string
	Port    string
	Path    string
	Message string
	Error   string
}
