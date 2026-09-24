// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/unifabric-io/unifabric/pkg/logger"
	yaml "gopkg.in/yaml.v2"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	defaultAgentMetricsBindAddress      = ":8082"
	defaultAgentHealthBindAddress       = ":8083"
	defaultNodeTopologyRefreshInterval  = "1m"
	defaultNodeTopologyInitialScanDelay = "1m"
	defaultRouteProbeAddress            = "1.1.1.1:53"

	defaultEBPFFlowBPFObject           = "/usr/lib/unifabric/rdma_flow.bpf.o"
	defaultEBPFFlowPinDir              = "/sys/fs/bpf/unifabric"
	defaultEBPFFlowOffsetCache         = "/var/lib/unifabric/ebpf-flow/callback-offsets.json"
	defaultEBPFFlowDiscoveryInterval   = "5s"
	defaultEBPFFlowReportInterval      = "5s"
	defaultEBPFFlowEndpointInterval    = "30s"
	defaultEBPFFlowEndpointMinInterval = "3s"
	defaultEBPFFlowSendQueue           = 200000
	defaultEBPFFlowSendBatch           = 10000
	defaultEBPFFlowSendFlush           = "1s"
	defaultEBPFFlowOTLPTimeout         = "10s"

	StorageNodeLeaderAnnotationKey = "unifabric.io/storage-node-leader"
)

type AgentNodeRole string

const (
	AgentNodeRoleGPU     AgentNodeRole = "gpu"
	AgentNodeRoleStorage AgentNodeRole = "storage"
)

type AgentNodeConfig struct {
	Name              string        `json:"name" yaml:"name"`
	Role              AgentNodeRole `json:"role" yaml:"role"`
	DefaultRouteProbe string        `json:"defaultRouteProbe" yaml:"defaultRouteProbe"`
}

// NodeTopologyDiscoveryConfig controls local RDMA interface and LLDP neighbor discovery.
type NodeTopologyDiscoveryConfig struct {
	RefreshInterval           string `json:"refreshInterval" yaml:"refreshInterval"`
	InitialScanDelay          string `json:"initialScanDelay" yaml:"initialScanDelay"`
	ScaleOutInterfaceSelector string `json:"scaleOutInterfaceSelector" yaml:"scaleOutInterfaceSelector"`
	StorageInterfaceSelector  string `json:"storageInterfaceSelector" yaml:"storageInterfaceSelector"`
	ScaleUpInterfaceSelector  string `json:"scaleUpInterfaceSelector" yaml:"scaleUpInterfaceSelector"`
}

// EBPFFlowEndpointConfig controls RDMAEndpoint publishing.
type EBPFFlowEndpointConfig struct {
	// Enabled publishes one RDMAEndpoint per Pod holding an RDMA device. Defaults to true.
	Enabled *bool `json:"enabled" yaml:"enabled"`
	// Interval is the fallback publish period covering lost events.
	Interval string `json:"interval" yaml:"interval"`
	// MinInterval is the minimum spacing between event driven updates.
	MinInterval string `json:"minInterval" yaml:"minInterval"`
	// AllPods also publishes Pods with their own network namespace.
	AllPods bool `json:"allPods" yaml:"allPods"`
}

// EBPFFlowFlowEventsConfig controls per-send ring-buffer events and their storage path.
type EBPFFlowFlowEventsConfig struct {
	// Enabled emits per-send events and defaults to true.
	Enabled *bool `json:"enabled" yaml:"enabled"`
	// Queue is the number of rows buffered per sink before new rows are dropped.
	Queue int `json:"queue" yaml:"queue"`
	// Batch is the number of rows per sink export.
	Batch int `json:"batch" yaml:"batch"`
	// Flush is the maximum time a partial batch waits.
	Flush string `json:"flush" yaml:"flush"`
	// Sample stores one event in every N, metrics still see every event.
	Sample uint64 `json:"sample" yaml:"sample"`
	// Aggregate merges identical submissions of a QP inside the window into one row.
	Aggregate string `json:"aggregate" yaml:"aggregate"`
}

// EBPFFlowOTLPConfig points the send event stream at an OpenTelemetry Collector.
type EBPFFlowOTLPConfig struct {
	// Endpoint is the OTLP gRPC host:port, empty disables the stream.
	Endpoint string `json:"endpoint" yaml:"endpoint"`
	// Insecure uses plaintext gRPC. Defaults to true.
	Insecure *bool `json:"insecure" yaml:"insecure"`
	// Timeout bounds one export.
	Timeout string `json:"timeout" yaml:"timeout"`
}

// EBPFFlowConfig enables and tunes the eBPF RDMA flow attribution component.
type EBPFFlowConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
	// BPFObject is the path of the compiled BPF program.
	BPFObject string `json:"bpfObject" yaml:"bpfObject"`
	// PinDir is the bpffs directory for pinned maps, empty disables pinning.
	PinDir string `json:"pinDir" yaml:"pinDir"`
	// OffsetCache is the host file persisting learned provider callback offsets.
	OffsetCache string `json:"offsetCache" yaml:"offsetCache"`
	// DiscoveryInterval is the fallback period for scanning new processes and libraries.
	DiscoveryInterval string `json:"discoveryInterval" yaml:"discoveryInterval"`
	// ReportInterval is the period for joining BPF maps into metrics.
	ReportInterval string                   `json:"reportInterval" yaml:"reportInterval"`
	Endpoint       EBPFFlowEndpointConfig   `json:"endpoint" yaml:"endpoint"`
	FlowEvents     EBPFFlowFlowEventsConfig `json:"flowEvents" yaml:"flowEvents"`
	OTLP           EBPFFlowOTLPConfig       `json:"otlp" yaml:"otlp"`
}

type AgentConfig struct {
	LogLevel              string                      `json:"logLevel" yaml:"logLevel"`
	Metrics               BindAddressConfig           `json:"metrics" yaml:"metrics"`
	HealthProbe           BindAddressConfig           `json:"healthProbe" yaml:"healthProbe"`
	Node                  AgentNodeConfig             `json:"node" yaml:"node"`
	NodeTopologyDiscovery NodeTopologyDiscoveryConfig `json:"nodeTopologyDiscovery" yaml:"nodeTopologyDiscovery"`
	EBPFFlow              EBPFFlowConfig              `json:"ebpfFlow" yaml:"ebpfFlow"`
	KubeConfig            *rest.Config                `json:"-" yaml:"-"`
}

func ReadAgentConfig(filename string) (*AgentConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var cfg AgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	cfg.KubeConfig, err = ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	if cfg.Metrics.BindAddress == "" {
		cfg.Metrics.BindAddress = defaultAgentMetricsBindAddress
	}
	cfg.LogLevel, err = logger.NormalizeLevel(cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("agent.logLevel: %w", err)
	}
	if cfg.HealthProbe.BindAddress == "" {
		cfg.HealthProbe.BindAddress = defaultAgentHealthBindAddress
	}

	if cfg.Node.Role == "" {
		cfg.Node.Role = AgentNodeRoleGPU
	}
	if cfg.Node.Role != AgentNodeRoleGPU && cfg.Node.Role != AgentNodeRoleStorage {
		return nil, fmt.Errorf("node.role: %s is invalid, expect %s or %s", cfg.Node.Role, AgentNodeRoleGPU, AgentNodeRoleStorage)
	}

	if err := normalizeNodeTopologyDiscoveryConfig(&cfg.NodeTopologyDiscovery); err != nil {
		return nil, err
	}
	if err := normalizeEBPFFlowConfig(&cfg.EBPFFlow); err != nil {
		return nil, err
	}

	if cfg.Node.DefaultRouteProbe == "" {
		cfg.Node.DefaultRouteProbe = defaultRouteProbeAddress
	} else if _, _, err := net.SplitHostPort(cfg.Node.DefaultRouteProbe); err != nil {
		return nil, fmt.Errorf("node.defaultRouteProbe: %s is invalid, expect host:port", cfg.Node.DefaultRouteProbe)
	}

	return &cfg, nil
}

func normalizeNodeTopologyDiscoveryConfig(cfg *NodeTopologyDiscoveryConfig) error {
	refreshIntervalField := "nodeTopologyDiscovery.refreshInterval"
	initialScanDelayField := "nodeTopologyDiscovery.initialScanDelay"
	scaleOutSelectorField := "nodeTopologyDiscovery.scaleOutInterfaceSelector"
	storageSelectorField := "nodeTopologyDiscovery.storageInterfaceSelector"
	scaleUpSelectorField := "nodeTopologyDiscovery.scaleUpInterfaceSelector"

	if err := validateInterfaceSelector(scaleOutSelectorField, cfg.ScaleOutInterfaceSelector); err != nil {
		return err
	}
	if err := validateInterfaceSelector(storageSelectorField, cfg.StorageInterfaceSelector); err != nil {
		return err
	}
	if err := validateInterfaceSelector(scaleUpSelectorField, cfg.ScaleUpInterfaceSelector); err != nil {
		return err
	}

	if cfg.RefreshInterval == "" {
		cfg.RefreshInterval = defaultNodeTopologyRefreshInterval
	} else if _, err := time.ParseDuration(cfg.RefreshInterval); err != nil {
		return fmt.Errorf("%s: %s is invalid, expect format like 1m or 30s", refreshIntervalField, cfg.RefreshInterval)
	}

	if cfg.InitialScanDelay == "" {
		cfg.InitialScanDelay = defaultNodeTopologyInitialScanDelay
	} else if _, err := time.ParseDuration(cfg.InitialScanDelay); err != nil {
		return fmt.Errorf("%s: %s is invalid, expect format like 1m or 30s", initialScanDelayField, cfg.InitialScanDelay)
	}
	return nil
}

// normalizeEBPFFlowConfig fills defaults and validates durations. Defaults
// are applied even when the component is disabled so a later enable through
// the same file behaves the same as a fresh config.
func normalizeEBPFFlowConfig(cfg *EBPFFlowConfig) error {
	setDefault(&cfg.BPFObject, defaultEBPFFlowBPFObject)
	setDefault(&cfg.PinDir, defaultEBPFFlowPinDir)
	setDefault(&cfg.OffsetCache, defaultEBPFFlowOffsetCache)
	setDefault(&cfg.DiscoveryInterval, defaultEBPFFlowDiscoveryInterval)
	setDefault(&cfg.ReportInterval, defaultEBPFFlowReportInterval)
	setDefaultBool(&cfg.Endpoint.Enabled, true)
	setDefault(&cfg.Endpoint.Interval, defaultEBPFFlowEndpointInterval)
	setDefault(&cfg.Endpoint.MinInterval, defaultEBPFFlowEndpointMinInterval)
	setDefaultBool(&cfg.FlowEvents.Enabled, true)
	if cfg.FlowEvents.Queue == 0 {
		cfg.FlowEvents.Queue = defaultEBPFFlowSendQueue
	}
	if cfg.FlowEvents.Batch == 0 {
		cfg.FlowEvents.Batch = defaultEBPFFlowSendBatch
	}
	setDefault(&cfg.FlowEvents.Flush, defaultEBPFFlowSendFlush)
	if cfg.FlowEvents.Sample == 0 {
		cfg.FlowEvents.Sample = 1
	}
	setDefault(&cfg.FlowEvents.Aggregate, "0s")
	setDefaultBool(&cfg.OTLP.Insecure, true)
	setDefault(&cfg.OTLP.Timeout, defaultEBPFFlowOTLPTimeout)

	durations := map[string]string{
		"ebpfFlow.discoveryInterval":    cfg.DiscoveryInterval,
		"ebpfFlow.reportInterval":       cfg.ReportInterval,
		"ebpfFlow.endpoint.interval":    cfg.Endpoint.Interval,
		"ebpfFlow.endpoint.minInterval": cfg.Endpoint.MinInterval,
		"ebpfFlow.flowEvents.flush":     cfg.FlowEvents.Flush,
		"ebpfFlow.flowEvents.aggregate": cfg.FlowEvents.Aggregate,
		"ebpfFlow.otlp.timeout":         cfg.OTLP.Timeout,
	}
	for field, value := range durations {
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("%s: %s is invalid, expect format like 5s or 100ms", field, value)
		}
	}
	if cfg.FlowEvents.Queue < 0 || cfg.FlowEvents.Batch < 0 {
		return fmt.Errorf("ebpfFlow.flowEvents.queue and ebpfFlow.flowEvents.batch must not be negative")
	}
	return nil
}

func setDefault(field *string, value string) {
	if *field == "" {
		*field = value
	}
}

func setDefaultBool(field **bool, value bool) {
	if *field == nil {
		v := value
		*field = &v
	}
}

func validateInterfaceSelector(field, selector string) error {
	if selector == "" {
		return nil
	}
	pattern := strings.SplitN(selector, "=", 2)
	if len(pattern) != 2 {
		return fmt.Errorf("%s %s is invalid, expect format like interface=ens1f0* or cidr=192.168.1.0/24", field, selector)
	}
	return nil
}
