// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package config

import "testing"

func TestNormalizeEBPFFlowConfigDefaultsFlowEventsEnabled(t *testing.T) {
	var cfg EBPFFlowConfig
	if err := normalizeEBPFFlowConfig(&cfg); err != nil {
		t.Fatalf("normalize config: %v", err)
	}
	if cfg.FlowEvents.Enabled == nil || !*cfg.FlowEvents.Enabled {
		t.Fatalf("flowEvents.enabled = %v, want true", cfg.FlowEvents.Enabled)
	}
}

func TestNormalizeEBPFFlowConfigPreservesFlowEventsDisabled(t *testing.T) {
	disabled := false
	cfg := EBPFFlowConfig{FlowEvents: EBPFFlowFlowEventsConfig{Enabled: &disabled}}
	if err := normalizeEBPFFlowConfig(&cfg); err != nil {
		t.Fatalf("normalize config: %v", err)
	}
	if cfg.FlowEvents.Enabled == nil || *cfg.FlowEvents.Enabled {
		t.Fatalf("flowEvents.enabled = %v, want false", cfg.FlowEvents.Enabled)
	}
}
