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
	defaultControllerMetricsBindAddress = ":8080"
	defaultControllerHealthBindAddress  = ":8081"
	defaultAgentMetricsBindAddress      = ":8082"
	defaultAgentHealthBindAddress       = ":8083"
	defaultNodeReconcileInterval        = "1m"
	defaultTopologyStartupDelay         = "1m"
	defaultDefaultRouteProbe            = "1.1.1.1:53"
	defaultLeaderElectionID             = "unifabric-controller"

	DefaultLabelAccelerator        = "unifabric.io/accelerator"
	DefaultLabelLeaf               = "unifabric.io/leaf"
	DefaultLabelSpine              = "unifabric.io/spine"
	DefaultLabelCore               = "unifabric.io/core"
	DefaultNodeTopologyLabelKey    = DefaultLabelLeaf
	StorageNodeLeaderAnnotationKey = "unifabric.io/storage-node-leader"
)

type BindAddressConfig struct {
	BindAddress string `json:"bindAddress" yaml:"bindAddress"`
}

type LeaderElectionConfig struct {
	Enabled   bool   `json:"enabled" yaml:"enabled"`
	Namespace string `json:"namespace" yaml:"namespace"`
	ID        string `json:"id" yaml:"id"`
}

type ScaleOutLeafGroupConfig struct {
	Enabled      bool   `json:"enabled" yaml:"enabled"`
	NodeLabelKey string `json:"nodeLabelKey" yaml:"nodeLabelKey"`
}

type ControllerTopologyConfig struct {
	ScaleOutLeafGroups ScaleOutLeafGroupConfig `json:"scaleOutLeafGroups" yaml:"scaleOutLeafGroups"`
}

type ControllerConfig struct {
	LogLevel       string                   `json:"logLevel" yaml:"logLevel"`
	Metrics        BindAddressConfig        `json:"metrics" yaml:"metrics"`
	HealthProbe    BindAddressConfig        `json:"healthProbe" yaml:"healthProbe"`
	Pprof          BindAddressConfig        `json:"pprof" yaml:"pprof"`
	LeaderElection LeaderElectionConfig     `json:"leaderElection" yaml:"leaderElection"`
	Topology       ControllerTopologyConfig `json:"topology" yaml:"topology"`
	KubeConfig     *rest.Config             `json:"-" yaml:"-"`
}

type NodeRole string

const (
	NodeRoleGPU     NodeRole = "gpu"
	NodeRoleStorage NodeRole = "storage"
)

type AgentNodeConfig struct {
	Name              string   `json:"name" yaml:"name"`
	Role              NodeRole `json:"role" yaml:"role"`
	DefaultRouteProbe string   `json:"defaultRouteProbe" yaml:"defaultRouteProbe"`
}

type AgentDiscoveryConfig struct {
	ReconcileInterval      string `json:"reconcileInterval" yaml:"reconcileInterval"`
	StartupDelay           string `json:"startupDelay" yaml:"startupDelay"`
	GPUInterfaceFilter     string `json:"gpuInterfaceFilter" yaml:"gpuInterfaceFilter"`
	StorageInterfaceFilter string `json:"storageInterfaceFilter" yaml:"storageInterfaceFilter"`
}

type AgentConfig struct {
	LogLevel    string               `json:"logLevel" yaml:"logLevel"`
	Metrics     BindAddressConfig    `json:"metrics" yaml:"metrics"`
	HealthProbe BindAddressConfig    `json:"healthProbe" yaml:"healthProbe"`
	Node        AgentNodeConfig      `json:"node" yaml:"node"`
	Discovery   AgentDiscoveryConfig `json:"discovery" yaml:"discovery"`
	KubeConfig  *rest.Config         `json:"-" yaml:"-"`
}

func ReadControllerConfig(filename string) (*ControllerConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var cfg ControllerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	cfg.KubeConfig, err = ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	if cfg.Metrics.BindAddress == "" {
		cfg.Metrics.BindAddress = defaultControllerMetricsBindAddress
	}
	cfg.LogLevel, err = logger.NormalizeLevel(cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("controller.logLevel: %w", err)
	}
	if cfg.HealthProbe.BindAddress == "" {
		cfg.HealthProbe.BindAddress = defaultControllerHealthBindAddress
	}
	if cfg.LeaderElection.ID == "" {
		cfg.LeaderElection.ID = defaultLeaderElectionID
	}
	if cfg.Topology.ScaleOutLeafGroups.NodeLabelKey == "" {
		cfg.Topology.ScaleOutLeafGroups.NodeLabelKey = DefaultNodeTopologyLabelKey
	}

	return &cfg, nil
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
		cfg.Node.Role = NodeRoleGPU
	}
	if cfg.Node.Role != NodeRoleGPU && cfg.Node.Role != NodeRoleStorage {
		return nil, fmt.Errorf("node.role: %s is invalid, expect %s or %s", cfg.Node.Role, NodeRoleGPU, NodeRoleStorage)
	}

	if cfg.Discovery.GPUInterfaceFilter != "" {
		gpuPattern := strings.SplitN(cfg.Discovery.GPUInterfaceFilter, "=", 2)
		if len(gpuPattern) != 2 {
			return nil, fmt.Errorf("discovery.gpuInterfaceFilter %s is invalid, expect format like interface=ens1f0* or cidr=192.168.1.0/24", cfg.Discovery.GPUInterfaceFilter)
		}
	}

	if cfg.Discovery.StorageInterfaceFilter != "" {
		storagePattern := strings.SplitN(cfg.Discovery.StorageInterfaceFilter, "=", 2)
		if len(storagePattern) != 2 {
			return nil, fmt.Errorf("discovery.storageInterfaceFilter %s is invalid, expect format like interface=ens2f0* or cidr=192.168.2.0/24", cfg.Discovery.StorageInterfaceFilter)
		}
	}

	if cfg.Discovery.ReconcileInterval == "" {
		cfg.Discovery.ReconcileInterval = defaultNodeReconcileInterval
	} else if _, err := time.ParseDuration(cfg.Discovery.ReconcileInterval); err != nil {
		return nil, fmt.Errorf("discovery.reconcileInterval: %s is invalid, expect format like 1m or 30s", cfg.Discovery.ReconcileInterval)
	}

	if cfg.Discovery.StartupDelay == "" {
		cfg.Discovery.StartupDelay = defaultTopologyStartupDelay
	} else if _, err := time.ParseDuration(cfg.Discovery.StartupDelay); err != nil {
		return nil, fmt.Errorf("discovery.startupDelay: %s is invalid, expect format like 1m or 30s", cfg.Discovery.StartupDelay)
	}

	if cfg.Node.DefaultRouteProbe == "" {
		cfg.Node.DefaultRouteProbe = defaultDefaultRouteProbe
	} else if _, _, err := net.SplitHostPort(cfg.Node.DefaultRouteProbe); err != nil {
		return nil, fmt.Errorf("node.defaultRouteProbe: %s is invalid, expect host:port", cfg.Node.DefaultRouteProbe)
	}

	return &cfg, nil
}
