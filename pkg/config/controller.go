// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"time"

	"github.com/unifabric-io/unifabric/pkg/logger"
	yaml "gopkg.in/yaml.v2"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	defaultControllerMetricsBindAddress       = ":8080"
	defaultControllerHealthBindAddress        = ":8081"
	defaultLeaderElectionID                   = "unifabric-controller"
	defaultSwitchDialTimeout                  = "5s"
	defaultSwitchReconnectBackoff             = "30s"
	defaultSwitchKeepaliveTime                = "30s"
	defaultSwitchMaxRecvMsgSize               = 4 * 1024 * 1024
	defaultSwitchGrpcPort               int32 = 8090
	defaultSwitchMTLSControllerSecret         = "switch-controller-mtls-controller"
	defaultSwitchMTLSServerSecret             = "switch-controller-mtls-agent"
	defaultSwitchLabelValueFormat             = "hash"
	defaultSwitchHashLength                   = 7

	DefaultLabelScaleUp       = "unifabric.io/scale-up"
	DefaultLabelScaleOutLeaf  = "unifabric.io/scale-out-leaf"
	DefaultLabelScaleOutSpine = "unifabric.io/scale-out-spine"
	DefaultLabelScaleOutCore  = "unifabric.io/scale-out-core"
)

var defaultIgnoredSwitchPorts = []string{"mgmt*", "Management*", "oob*"}

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

type SwitchDiscoveryMTLSConfig struct {
	Enabled               *bool  `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	ControllerSecretName  string `json:"controllerSecretName" yaml:"controllerSecretName"`
	SwitchAgentSecretName string `json:"switchAgentSecretName" yaml:"switchAgentSecretName"`
}

type TopologyGroupNamingConfig struct {
	LabelValueFormat string `json:"labelValueFormat" yaml:"labelValueFormat"`
	HashLength       int    `json:"hashLength" yaml:"hashLength"`
}

type ScaleOutSwitchesConfig struct {
	Enabled           bool                      `json:"enabled" yaml:"enabled"`
	DialTimeout       string                    `json:"dialTimeout" yaml:"dialTimeout"`
	ReconnectBackoff  string                    `json:"reconnectBackoff" yaml:"reconnectBackoff"`
	MaxRecvMsgSize    int                       `json:"maxRecvMsgSize" yaml:"maxRecvMsgSize"`
	KeepaliveTime     string                    `json:"keepaliveTime" yaml:"keepaliveTime"`
	DefaultGrpcPort   int32                     `json:"defaultGrpcPort" yaml:"defaultGrpcPort"`
	IgnoreSwitchPorts []string                  `json:"ignoreSwitchPorts" yaml:"ignoreSwitchPorts"`
	MTLS              SwitchDiscoveryMTLSConfig `json:"mtls" yaml:"mtls"`
	GroupNaming       TopologyGroupNamingConfig `json:"groupNaming" yaml:"groupNaming"`
}

type ScaleOutDiscoveryConfig struct {
	LeafGroups ScaleOutLeafGroupsConfig `json:"leafGroups" yaml:"leafGroups"`
	Switches   ScaleOutSwitchesConfig   `json:"switches" yaml:"switches"`
}

type ControllerNodeTopologyConfig struct {
	ScaleOutInterfaceSelector string `json:"scaleOutInterfaceSelector" yaml:"scaleOutInterfaceSelector"`
	StorageInterfaceSelector  string `json:"storageInterfaceSelector" yaml:"storageInterfaceSelector"`
	ScaleUpInterfaceSelector  string `json:"scaleUpInterfaceSelector" yaml:"scaleUpInterfaceSelector"`
}

type ControllerConfig struct {
	LogLevel              string                       `json:"logLevel" yaml:"logLevel"`
	Metrics               BindAddressConfig            `json:"metrics" yaml:"metrics"`
	HealthProbe           BindAddressConfig            `json:"healthProbe" yaml:"healthProbe"`
	Pprof                 BindAddressConfig            `json:"pprof" yaml:"pprof"`
	LeaderElection        LeaderElectionConfig         `json:"leaderElection" yaml:"leaderElection"`
	TopologyLabels        TopologyLabelsConfig         `json:"topologyLabels" yaml:"topologyLabels"`
	NodeTopologyDiscovery ControllerNodeTopologyConfig `json:"nodeTopologyDiscovery" yaml:"nodeTopologyDiscovery"`
	ScaleOutDiscovery     ScaleOutDiscoveryConfig      `json:"scaleOutDiscovery" yaml:"scaleOutDiscovery"`
	KubeConfig            *rest.Config                 `json:"-" yaml:"-"`
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
	if err := normalizeControllerNodeTopologyConfig(&cfg.NodeTopologyDiscovery); err != nil {
		return nil, err
	}
	if err := normalizeScaleOutDiscoveryConfig(&cfg.ScaleOutDiscovery); err != nil {
		return nil, err
	}

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

func normalizeControllerNodeTopologyConfig(cfg *ControllerNodeTopologyConfig) error {
	if err := validateInterfaceSelector("nodeTopologyDiscovery.scaleOutInterfaceSelector", cfg.ScaleOutInterfaceSelector); err != nil {
		return err
	}
	if err := validateInterfaceSelector("nodeTopologyDiscovery.storageInterfaceSelector", cfg.StorageInterfaceSelector); err != nil {
		return err
	}
	if err := validateInterfaceSelector("nodeTopologyDiscovery.scaleUpInterfaceSelector", cfg.ScaleUpInterfaceSelector); err != nil {
		return err
	}
	return nil
}

func normalizeScaleOutDiscoveryConfig(cfg *ScaleOutDiscoveryConfig) error {
	if err := normalizeSwitchDiscoveryDuration("scaleOutDiscovery.switches.dialTimeout", &cfg.Switches.DialTimeout, defaultSwitchDialTimeout); err != nil {
		return err
	}
	if err := normalizeSwitchDiscoveryDuration("scaleOutDiscovery.switches.reconnectBackoff", &cfg.Switches.ReconnectBackoff, defaultSwitchReconnectBackoff); err != nil {
		return err
	}
	if err := normalizeSwitchDiscoveryDuration("scaleOutDiscovery.switches.keepaliveTime", &cfg.Switches.KeepaliveTime, defaultSwitchKeepaliveTime); err != nil {
		return err
	}

	if cfg.Switches.MaxRecvMsgSize <= 0 {
		cfg.Switches.MaxRecvMsgSize = defaultSwitchMaxRecvMsgSize
	}
	if cfg.Switches.DefaultGrpcPort == 0 {
		cfg.Switches.DefaultGrpcPort = defaultSwitchGrpcPort
	}
	if cfg.Switches.DefaultGrpcPort < 1 || cfg.Switches.DefaultGrpcPort > 65535 {
		return fmt.Errorf("scaleOutDiscovery.switches.defaultGrpcPort: %d is invalid, expect 1-65535", cfg.Switches.DefaultGrpcPort)
	}
	if len(cfg.Switches.IgnoreSwitchPorts) == 0 {
		cfg.Switches.IgnoreSwitchPorts = append([]string(nil), defaultIgnoredSwitchPorts...)
	}

	if cfg.Switches.MTLS.Enabled == nil {
		cfg.Switches.MTLS.Enabled = boolPtr(true)
	}
	if cfg.Switches.MTLS.ControllerSecretName == "" {
		cfg.Switches.MTLS.ControllerSecretName = defaultSwitchMTLSControllerSecret
	}
	if cfg.Switches.MTLS.SwitchAgentSecretName == "" {
		cfg.Switches.MTLS.SwitchAgentSecretName = defaultSwitchMTLSServerSecret
	}

	if cfg.Switches.GroupNaming.LabelValueFormat == "" {
		cfg.Switches.GroupNaming.LabelValueFormat = defaultSwitchLabelValueFormat
	}
	switch cfg.Switches.GroupNaming.LabelValueFormat {
	case "name", "hash":
	default:
		return fmt.Errorf("scaleOutDiscovery.switches.groupNaming.labelValueFormat: %s is invalid, expect name or hash", cfg.Switches.GroupNaming.LabelValueFormat)
	}
	if cfg.Switches.GroupNaming.HashLength <= 0 {
		cfg.Switches.GroupNaming.HashLength = defaultSwitchHashLength
	}

	return nil
}

func normalizeSwitchDiscoveryDuration(field string, value *string, fallback string) error {
	if *value == "" {
		*value = fallback
		return nil
	}
	if _, err := time.ParseDuration(*value); err != nil {
		return fmt.Errorf("%s: %s is invalid, expect format like 5s or 30s", field, *value)
	}
	return nil
}

func boolPtr(v bool) *bool {
	return &v
}
