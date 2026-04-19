// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"

	"github.com/unifabric-io/unifabric/pkg/logger"
	yaml "gopkg.in/yaml.v2"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	defaultControllerMetricsBindAddress = ":8080"
	defaultControllerHealthBindAddress  = ":8081"
	defaultLeaderElectionID             = "unifabric-controller"

	DefaultLabelScaleUp       = "unifabric.io/scale-up"
	DefaultLabelScaleOutLeaf  = "unifabric.io/scale-out-leaf"
	DefaultLabelScaleOutSpine = "unifabric.io/scale-out-spine"
	DefaultLabelScaleOutCore  = "unifabric.io/scale-out-core"
)

type BindAddressConfig struct {
	BindAddress string `json:"bindAddress" yaml:"bindAddress"`
}

type LeaderElectionConfig struct {
	Enabled   bool   `json:"enabled" yaml:"enabled"`
	Namespace string `json:"namespace" yaml:"namespace"`
	ID        string `json:"id" yaml:"id"`
}

type TopologyLabelsConfig struct {
	ScaleUp       string `json:"scaleUp" yaml:"scaleUp"`
	ScaleOutLeaf  string `json:"scaleOutLeaf" yaml:"scaleOutLeaf"`
	ScaleOutSpine string `json:"scaleOutSpine" yaml:"scaleOutSpine"`
	ScaleOutCore  string `json:"scaleOutCore" yaml:"scaleOutCore"`
}

type ScaleOutLeafGroupsConfig struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}

type ScaleOutDiscoveryConfig struct {
	LeafGroups ScaleOutLeafGroupsConfig `json:"leafGroups" yaml:"leafGroups"`
}

type ControllerConfig struct {
	LogLevel          string                  `json:"logLevel" yaml:"logLevel"`
	Metrics           BindAddressConfig       `json:"metrics" yaml:"metrics"`
	HealthProbe       BindAddressConfig       `json:"healthProbe" yaml:"healthProbe"`
	Pprof             BindAddressConfig       `json:"pprof" yaml:"pprof"`
	LeaderElection    LeaderElectionConfig    `json:"leaderElection" yaml:"leaderElection"`
	TopologyLabels    TopologyLabelsConfig    `json:"topologyLabels" yaml:"topologyLabels"`
	ScaleOutDiscovery ScaleOutDiscoveryConfig `json:"scaleOutDiscovery" yaml:"scaleOutDiscovery"`
	KubeConfig        *rest.Config            `json:"-" yaml:"-"`
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
	normalizeTopologyLabels(&cfg.TopologyLabels)

	return &cfg, nil
}

func normalizeTopologyLabels(cfg *TopologyLabelsConfig) {
	if cfg.ScaleUp == "" {
		cfg.ScaleUp = DefaultLabelScaleUp
	}
	if cfg.ScaleOutLeaf == "" {
		cfg.ScaleOutLeaf = DefaultLabelScaleOutLeaf
	}
	if cfg.ScaleOutSpine == "" {
		cfg.ScaleOutSpine = DefaultLabelScaleOutSpine
	}
	if cfg.ScaleOutCore == "" {
		cfg.ScaleOutCore = DefaultLabelScaleOutCore
	}
}
