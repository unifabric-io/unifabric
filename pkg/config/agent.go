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

type AgentConfig struct {
	LogLevel              string                      `json:"logLevel" yaml:"logLevel"`
	Metrics               BindAddressConfig           `json:"metrics" yaml:"metrics"`
	HealthProbe           BindAddressConfig           `json:"healthProbe" yaml:"healthProbe"`
	Node                  AgentNodeConfig             `json:"node" yaml:"node"`
	NodeTopologyDiscovery NodeTopologyDiscoveryConfig `json:"nodeTopologyDiscovery" yaml:"nodeTopologyDiscovery"`
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
