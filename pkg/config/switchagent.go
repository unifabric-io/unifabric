// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"time"

	"github.com/unifabric-io/unifabric/pkg/logger"
)

const (
	defaultSwitchAgentListenAddress   = ":8090"
	defaultSwitchAgentRefreshInterval = "10s"
	defaultSwitchAgentCertFile        = "/etc/unifabric/switch-mtls/tls.crt"
	defaultSwitchAgentKeyFile         = "/etc/unifabric/switch-mtls/tls.key"
	defaultSwitchAgentPeerCertFile    = "/etc/unifabric/switch-mtls/peer.crt"
	defaultSwitchAgentLLDPMode        = "socket"
	defaultSwitchAgentLLDPSocketPath  = "/run/lldpd.socket"
	defaultSwitchAgentLLDPCLIVersion  = "1.0.16"
)

type SwitchAgentMTLSConfig struct {
	Enabled      *bool  `env:"UNIFABRIC_SWITCH_AGENT_MTLS_ENABLED"`
	CertFile     string `env:"UNIFABRIC_SWITCH_AGENT_MTLS_CERT_FILE"`
	KeyFile      string `env:"UNIFABRIC_SWITCH_AGENT_MTLS_KEY_FILE"`
	PeerCertFile string `env:"UNIFABRIC_SWITCH_AGENT_MTLS_PEER_CERT_FILE"`
}

type SwitchAgentLLDPConfig struct {
	RefreshInterval string `env:"UNIFABRIC_SWITCH_AGENT_LLDP_REFRESH_INTERVAL"`
	CollectionMode  string `env:"UNIFABRIC_SWITCH_AGENT_LLDP_COLLECTION_MODE"`
	SocketPath      string `env:"UNIFABRIC_SWITCH_AGENT_LLDP_SOCKET_PATH"`
	CLIVersion      string `env:"UNIFABRIC_SWITCH_AGENT_LLDP_CLI_VERSION"`
}

type SwitchAgentConfig struct {
	LogLevel      string `env:"UNIFABRIC_SWITCH_AGENT_LOG_LEVEL"`
	SwitchName    string `env:"UNIFABRIC_SWITCH_AGENT_SWITCH_NAME"`
	ListenAddress string `env:"UNIFABRIC_SWITCH_AGENT_LISTEN_ADDRESS"`
	MTLS          SwitchAgentMTLSConfig
	LLDP          SwitchAgentLLDPConfig
}

func ReadSwitchAgentConfigFromEnv() (*SwitchAgentConfig, error) {
	var cfg SwitchAgentConfig
	if err := applyEnvTags(&cfg); err != nil {
		return nil, err
	}
	if err := normalizeSwitchAgentConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (cfg *SwitchAgentConfig) MTLSEnabled() bool {
	return cfg.MTLS.Enabled == nil || *cfg.MTLS.Enabled
}

func normalizeSwitchAgentConfig(cfg *SwitchAgentConfig) error {
	var err error
	cfg.LogLevel, err = logger.NormalizeLevel(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("switchAgent.logLevel: %w", err)
	}

	if cfg.ListenAddress == "" {
		cfg.ListenAddress = defaultSwitchAgentListenAddress
	}
	if cfg.SwitchName == "" {
		cfg.SwitchName, err = os.Hostname()
		if err != nil {
			return fmt.Errorf("switchAgent.switchName: failed to read hostname: %w", err)
		}
	}
	if err := normalizeSwitchAgentLLDPConfig(&cfg.LLDP); err != nil {
		return err
	}
	normalizeSwitchAgentMTLSConfig(&cfg.MTLS)

	return nil
}

func normalizeSwitchAgentLLDPConfig(cfg *SwitchAgentLLDPConfig) error {
	if cfg.RefreshInterval == "" {
		cfg.RefreshInterval = defaultSwitchAgentRefreshInterval
	} else if _, err := time.ParseDuration(cfg.RefreshInterval); err != nil {
		return fmt.Errorf("switchAgent.lldp.refreshInterval: %s is invalid, expect format like 10s or 1m", cfg.RefreshInterval)
	}
	if cfg.CollectionMode == "" {
		cfg.CollectionMode = defaultSwitchAgentLLDPMode
	}
	switch cfg.CollectionMode {
	case "socket", "hostProc":
	default:
		return fmt.Errorf("switchAgent.lldp.collectionMode: %s is invalid, expect socket or hostProc", cfg.CollectionMode)
	}
	if cfg.SocketPath == "" {
		cfg.SocketPath = defaultSwitchAgentLLDPSocketPath
	}
	if cfg.CLIVersion == "" {
		cfg.CLIVersion = defaultSwitchAgentLLDPCLIVersion
	}
	switch cfg.CLIVersion {
	case "1.0.4", "1.0.16":
	default:
		return fmt.Errorf("switchAgent.lldp.cliVersion: %s is invalid, expect 1.0.4 or 1.0.16", cfg.CLIVersion)
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
