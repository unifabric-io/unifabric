// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package config

import "testing"

func TestReadSwitchAgentConfigFromEnvUsesDefaults(t *testing.T) {
	cfg, err := ReadSwitchAgentConfigFromEnv()
	if err != nil {
		t.Fatalf("ReadSwitchAgentConfigFromEnv returned error: %v", err)
	}

	if cfg.LogLevel != "info" {
		t.Fatalf("expected default log level info, got %q", cfg.LogLevel)
	}
	if cfg.SwitchName == "" {
		t.Fatal("expected default switch name to be populated from hostname")
	}
	if cfg.ListenAddress != defaultSwitchAgentListenAddress {
		t.Fatalf("expected default listen address %q, got %q", defaultSwitchAgentListenAddress, cfg.ListenAddress)
	}
	if cfg.LLDP.RefreshInterval != defaultSwitchAgentRefreshInterval {
		t.Fatalf("expected default refresh interval %q, got %q", defaultSwitchAgentRefreshInterval, cfg.LLDP.RefreshInterval)
	}
	if cfg.LLDP.CollectionMode != defaultSwitchAgentLLDPMode {
		t.Fatalf("expected default lldp collection mode %q, got %q", defaultSwitchAgentLLDPMode, cfg.LLDP.CollectionMode)
	}
	if !cfg.MTLSEnabled() {
		t.Fatal("expected mTLS to be enabled by default")
	}
	if cfg.MTLS.CertFile != defaultSwitchAgentCertFile {
		t.Fatalf("expected default cert file %q, got %q", defaultSwitchAgentCertFile, cfg.MTLS.CertFile)
	}
	if cfg.MTLS.KeyFile != defaultSwitchAgentKeyFile {
		t.Fatalf("expected default key file %q, got %q", defaultSwitchAgentKeyFile, cfg.MTLS.KeyFile)
	}
	if cfg.MTLS.PeerCertFile != defaultSwitchAgentPeerCertFile {
		t.Fatalf("expected default peer cert file %q, got %q", defaultSwitchAgentPeerCertFile, cfg.MTLS.PeerCertFile)
	}
	if cfg.LLDP.SocketPath != defaultSwitchAgentLLDPSocketPath {
		t.Fatalf("expected default lldp socket path %q, got %q", defaultSwitchAgentLLDPSocketPath, cfg.LLDP.SocketPath)
	}
	if cfg.LLDP.CLIVersion != defaultSwitchAgentLLDPCLIVersion {
		t.Fatalf("expected default lldp cli version %q, got %q", defaultSwitchAgentLLDPCLIVersion, cfg.LLDP.CLIVersion)
	}
}

func TestReadSwitchAgentConfigFromEnv(t *testing.T) {
	t.Setenv("UNIFABRIC_SWITCH_AGENT_LOG_LEVEL", "debug")
	t.Setenv("UNIFABRIC_SWITCH_AGENT_SWITCH_NAME", "leaf-1")
	t.Setenv("UNIFABRIC_SWITCH_AGENT_LISTEN_ADDRESS", ":18090")
	t.Setenv("UNIFABRIC_SWITCH_AGENT_MTLS_ENABLED", "false")
	t.Setenv("UNIFABRIC_SWITCH_AGENT_MTLS_CERT_FILE", "/tmp/tls.crt")
	t.Setenv("UNIFABRIC_SWITCH_AGENT_MTLS_KEY_FILE", "/tmp/tls.key")
	t.Setenv("UNIFABRIC_SWITCH_AGENT_MTLS_PEER_CERT_FILE", "/tmp/peer.crt")
	t.Setenv("UNIFABRIC_SWITCH_AGENT_LLDP_REFRESH_INTERVAL", "30s")
	t.Setenv("UNIFABRIC_SWITCH_AGENT_LLDP_COLLECTION_MODE", "hostProc")
	t.Setenv("UNIFABRIC_SWITCH_AGENT_LLDP_SOCKET_PATH", "/tmp/lldpd.socket")
	t.Setenv("UNIFABRIC_SWITCH_AGENT_LLDP_CLI_VERSION", "1.0.4")

	cfg, err := ReadSwitchAgentConfigFromEnv()
	if err != nil {
		t.Fatalf("ReadSwitchAgentConfigFromEnv returned error: %v", err)
	}

	if cfg.LogLevel != "debug" {
		t.Fatalf("expected log level debug, got %q", cfg.LogLevel)
	}
	if cfg.SwitchName != "leaf-1" {
		t.Fatalf("expected switch name leaf-1, got %q", cfg.SwitchName)
	}
	if cfg.ListenAddress != ":18090" {
		t.Fatalf("expected listen address :18090, got %q", cfg.ListenAddress)
	}
	if cfg.MTLSEnabled() {
		t.Fatal("expected mTLS to be disabled from env")
	}
	if cfg.MTLS.CertFile != "/tmp/tls.crt" {
		t.Fatalf("expected env cert file, got %q", cfg.MTLS.CertFile)
	}
	if cfg.MTLS.KeyFile != "/tmp/tls.key" {
		t.Fatalf("expected env key file, got %q", cfg.MTLS.KeyFile)
	}
	if cfg.MTLS.PeerCertFile != "/tmp/peer.crt" {
		t.Fatalf("expected env peer cert file, got %q", cfg.MTLS.PeerCertFile)
	}
	if cfg.LLDP.RefreshInterval != "30s" {
		t.Fatalf("expected refresh interval 30s, got %q", cfg.LLDP.RefreshInterval)
	}
	if cfg.LLDP.CollectionMode != "hostProc" {
		t.Fatalf("expected collection mode hostProc, got %q", cfg.LLDP.CollectionMode)
	}
	if cfg.LLDP.SocketPath != "/tmp/lldpd.socket" {
		t.Fatalf("expected socket path /tmp/lldpd.socket, got %q", cfg.LLDP.SocketPath)
	}
	if cfg.LLDP.CLIVersion != "1.0.4" {
		t.Fatalf("expected cli version 1.0.4, got %q", cfg.LLDP.CLIVersion)
	}
}

func TestReadSwitchAgentConfigFromEnvRejectsInvalidBool(t *testing.T) {
	t.Setenv("UNIFABRIC_SWITCH_AGENT_MTLS_ENABLED", "sometimes")

	_, err := ReadSwitchAgentConfigFromEnv()
	if err == nil {
		t.Fatal("expected invalid boolean env to fail")
	}
}

func TestReadSwitchAgentConfigFromEnvRejectsInvalidCLIVersion(t *testing.T) {
	t.Setenv("UNIFABRIC_SWITCH_AGENT_LLDP_CLI_VERSION", "1.0.20")

	_, err := ReadSwitchAgentConfigFromEnv()
	if err == nil {
		t.Fatal("expected invalid lldp cli version to fail")
	}
}

func TestReadSwitchAgentConfigFromEnvRejectsInvalidCollectionMode(t *testing.T) {
	t.Setenv("UNIFABRIC_SWITCH_AGENT_LLDP_COLLECTION_MODE", "auto")

	_, err := ReadSwitchAgentConfigFromEnv()
	if err == nil {
		t.Fatal("expected invalid lldp collection mode to fail")
	}
}
