// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"time"

	"github.com/unifabric-io/unifabric/pkg/logger"
	"github.com/unifabric-io/unifabric/pkg/topologylabel"
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

	DefaultLabelScaleUpTemplate  = "scale-up.unifabric.io/tier-{{ .Tier }}"
	DefaultLabelScaleOutTemplate = "scale-out.unifabric.io/tier-{{ .Tier }}"
	DefaultLabelStorageTemplate  = "storage.unifabric.io/tier-{{ .Tier }}"

	// Deprecated fixed keys are retained only for source compatibility with
	// the pre-Topology-CRD projection helpers.
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
	ScaleUp  string `json:"scaleUp" yaml:"scaleUp"`
	ScaleOut string `json:"scaleOut" yaml:"scaleOut"`
	Storage  string `json:"storage" yaml:"storage"`

	ScaleOutLeaf  string `json:"-" yaml:"-"`
	ScaleOutSpine string `json:"-" yaml:"-"`
	ScaleOutCore  string `json:"-" yaml:"-"`
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
	ManageScaleOut    bool                      `json:"manageScaleOut" yaml:"manageScaleOut"`
	ManageStorage     bool                      `json:"manageStorage" yaml:"manageStorage"`
	DialTimeout       string                    `json:"dialTimeout" yaml:"dialTimeout"`
	ReconnectBackoff  string                    `json:"reconnectBackoff" yaml:"reconnectBackoff"`
	MaxRecvMsgSize    int                       `json:"maxRecvMsgSize" yaml:"maxRecvMsgSize"`
	KeepaliveTime     string                    `json:"keepaliveTime" yaml:"keepaliveTime"`
	DefaultGrpcPort   int32                     `json:"defaultGrpcPort" yaml:"defaultGrpcPort"`
	IgnoreSwitchPorts []string                  `json:"ignoreSwitchPorts" yaml:"ignoreSwitchPorts"`
	MTLS              SwitchDiscoveryMTLSConfig `json:"mtls" yaml:"mtls"`
	GroupNaming       TopologyGroupNamingConfig `json:"-" yaml:"-"`
}

type ScaleOutDiscoveryConfig struct {
	Switches ScaleOutSwitchesConfig `json:"switches" yaml:"switches"`
}

type InternalTopologyLabelWriterConfig struct {
	Enabled *bool `json:"enabled" yaml:"enabled"`
}

type ControllerNodeTopologyConfig struct {
	ScaleOutInterfaceSelector string `json:"scaleOutInterfaceSelector" yaml:"scaleOutInterfaceSelector"`
	StorageInterfaceSelector  string `json:"storageInterfaceSelector" yaml:"storageInterfaceSelector"`
	ScaleUpInterfaceSelector  string `json:"scaleUpInterfaceSelector" yaml:"scaleUpInterfaceSelector"`
}

type ControllerConfig struct {
	LogLevel                    string                            `json:"logLevel" yaml:"logLevel"`
	Metrics                     BindAddressConfig                 `json:"metrics" yaml:"metrics"`
	HealthProbe                 BindAddressConfig                 `json:"healthProbe" yaml:"healthProbe"`
	Pprof                       BindAddressConfig                 `json:"pprof" yaml:"pprof"`
	LeaderElection              LeaderElectionConfig              `json:"leaderElection" yaml:"leaderElection"`
	TopologyLabels              TopologyLabelsConfig              `json:"topologyLabels" yaml:"topologyLabels"`
	InternalTopologyLabelWriter InternalTopologyLabelWriterConfig `json:"-" yaml:"-"`
	NodeTopologyDiscovery       ControllerNodeTopologyConfig      `json:"nodeTopologyDiscovery" yaml:"nodeTopologyDiscovery"`
	ScaleOutDiscovery           ScaleOutDiscoveryConfig           `json:"scaleOutDiscovery" yaml:"scaleOutDiscovery"`
	KubeConfig                  *rest.Config                      `json:"-" yaml:"-"`
	TopologyLabelTemplates      *topologylabel.Set                `json:"-" yaml:"-"`
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
	if err := normalizeTopologyLabels(&cfg); err != nil {
		return nil, err
	}
	normalizeInternalTopologyLabelWriter(&cfg.InternalTopologyLabelWriter)
	if err := normalizeControllerNodeTopologyConfig(&cfg.NodeTopologyDiscovery); err != nil {
		return nil, err
	}
	if err := normalizeScaleOutDiscoveryConfig(&cfg.ScaleOutDiscovery); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func normalizeTopologyLabels(controllerCfg *ControllerConfig) error {
	cfg := &controllerCfg.TopologyLabels
	if cfg.ScaleUp == "" {
		cfg.ScaleUp = DefaultLabelScaleUpTemplate
	}
	if cfg.ScaleOut == "" {
		cfg.ScaleOut = DefaultLabelScaleOutTemplate
	}
	if cfg.Storage == "" {
		cfg.Storage = DefaultLabelStorageTemplate
	}

	// Keep old helper defaults available to legacy unit coverage. These fields
	// are not accepted from YAML and are not used by the new controllers.
	if cfg.ScaleOutLeaf == "" {
		cfg.ScaleOutLeaf = DefaultLabelScaleOutLeaf
	}
	if cfg.ScaleOutSpine == "" {
		cfg.ScaleOutSpine = DefaultLabelScaleOutSpine
	}
	if cfg.ScaleOutCore == "" {
		cfg.ScaleOutCore = DefaultLabelScaleOutCore
	}

	compiled, err := topologylabel.CompileSet(cfg.ScaleUp, cfg.ScaleOut, cfg.Storage)
	if err != nil {
		return err
	}
	controllerCfg.TopologyLabelTemplates = compiled
	return nil
}

// EnsureTopologyLabelTemplates applies defaults and compiles templates for
// programmatically constructed controller configurations.
func EnsureTopologyLabelTemplates(cfg *ControllerConfig) error {
	if cfg == nil {
		return fmt.Errorf("controller config must not be nil")
	}
	if cfg.TopologyLabelTemplates != nil {
		return nil
	}
	return normalizeTopologyLabels(cfg)
}

func normalizeInternalTopologyLabelWriter(cfg *InternalTopologyLabelWriterConfig) {
	if cfg.Enabled == nil {
		cfg.Enabled = boolPtr(true)
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
	// Keep direct controller configuration compatible with the former single
	// switch-discovery toggle. The Helm chart always renders both ownership
	// fields explicitly from topoDiscovery modes.
	if cfg.Switches.Enabled && !cfg.Switches.ManageScaleOut && !cfg.Switches.ManageStorage {
		cfg.Switches.ManageScaleOut = true
		cfg.Switches.ManageStorage = true
	}
	if cfg.Switches.ManageScaleOut || cfg.Switches.ManageStorage {
		cfg.Switches.Enabled = true
	}

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
