// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"time"

	"github.com/unifabric-io/unifabric/pkg/logger"
	yaml "gopkg.in/yaml.v2"
)

const (
	defaultSwitchAgentListenAddress   = ":8090"
	defaultSwitchAgentRefreshInterval = "10s"
	defaultSwitchAgentCertFile        = "/etc/unifabric/switch-mtls/tls.crt"
	defaultSwitchAgentKeyFile         = "/etc/unifabric/switch-mtls/tls.key"
	defaultSwitchAgentPeerCertFile    = "/etc/unifabric/switch-mtls/peer.crt"
)

type SwitchAgentMTLSConfig struct {
	Enabled      *bool  `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	CertFile     string `json:"certFile" yaml:"certFile"`
	KeyFile      string `json:"keyFile" yaml:"keyFile"`
	PeerCertFile string `json:"peerCertFile" yaml:"peerCertFile"`
}

type SwitchAgentLLDPConfig struct {
	RefreshInterval string `json:"refreshInterval" yaml:"refreshInterval"`
}

type SwitchAgentConfig struct {
	LogLevel      string                `json:"logLevel" yaml:"logLevel"`
	SwitchName    string                `json:"switchName" yaml:"switchName"`
	ListenAddress string                `json:"listenAddress" yaml:"listenAddress"`
	MTLS          SwitchAgentMTLSConfig `json:"mtls" yaml:"mtls"`
	LLDP          SwitchAgentLLDPConfig `json:"lldp" yaml:"lldp"`
}

func ReadSwitchAgentConfig(filename string) (*SwitchAgentConfig, error) {
	var cfg SwitchAgentConfig
	if filename != "" {
		data, err := os.ReadFile(filename)
		if err != nil {
			return nil, err
		}

		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
	}

	var err error
	cfg.LogLevel, err = logger.NormalizeLevel(cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("switchAgent.logLevel: %w", err)
	}

	if cfg.ListenAddress == "" {
		cfg.ListenAddress = defaultSwitchAgentListenAddress
	}
	if cfg.SwitchName == "" {
		cfg.SwitchName, err = os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("switchAgent.switchName: failed to read hostname: %w", err)
		}
	}
	if err := normalizeSwitchAgentLLDPConfig(&cfg.LLDP); err != nil {
		return nil, err
	}
	normalizeSwitchAgentMTLSConfig(&cfg.MTLS)

	return &cfg, nil
}

func (cfg *SwitchAgentConfig) MTLSEnabled() bool {
	return cfg.MTLS.Enabled == nil || *cfg.MTLS.Enabled
}

func normalizeSwitchAgentLLDPConfig(cfg *SwitchAgentLLDPConfig) error {
	if cfg.RefreshInterval == "" {
		cfg.RefreshInterval = defaultSwitchAgentRefreshInterval
	} else if _, err := time.ParseDuration(cfg.RefreshInterval); err != nil {
		return fmt.Errorf("switchAgent.lldp.refreshInterval: %s is invalid, expect format like 10s or 1m", cfg.RefreshInterval)
	}
	return nil
}

func normalizeSwitchAgentMTLSConfig(cfg *SwitchAgentMTLSConfig) {
	if cfg.Enabled == nil {
		cfg.Enabled = boolPtr(true)
	}
	if cfg.CertFile == "" {
		cfg.CertFile = defaultSwitchAgentCertFile
	}
	if cfg.KeyFile == "" {
		cfg.KeyFile = defaultSwitchAgentKeyFile
	}
	if cfg.PeerCertFile == "" {
		cfg.PeerCertFile = defaultSwitchAgentPeerCertFile
	}
}
