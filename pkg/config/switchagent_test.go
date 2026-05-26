// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package config

import "testing"

func TestReadSwitchAgentConfigWithoutFileUsesDefaults(t *testing.T) {
	cfg, err := ReadSwitchAgentConfig("")
	if err != nil {
		t.Fatalf("ReadSwitchAgentConfig returned error: %v", err)
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
}
