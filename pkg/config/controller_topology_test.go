// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package config

import "testing"

func TestEnsureTopologyLabelTemplatesAppliesDefaults(t *testing.T) {
	cfg := &ControllerConfig{}
	if err := EnsureTopologyLabelTemplates(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.TopologyLabels.ScaleUp != DefaultLabelScaleUpTemplate ||
		cfg.TopologyLabels.ScaleOut != DefaultLabelScaleOutTemplate ||
		cfg.TopologyLabels.Storage != DefaultLabelStorageTemplate {
		t.Fatalf("topology labels = %#v", cfg.TopologyLabels)
	}
	key, err := cfg.TopologyLabelTemplates.ScaleOut.Render(4)
	if err != nil || key != "scale-out.unifabric.io/tier-4" {
		t.Fatalf("scale-out tier 4 key = %q, %v", key, err)
	}
}

func TestEnsureTopologyLabelTemplatesRejectsInvalidTemplate(t *testing.T) {
	cfg := &ControllerConfig{TopologyLabels: TopologyLabelsConfig{
		ScaleUp:  DefaultLabelScaleUpTemplate,
		ScaleOut: "scale-out.unifabric.io/static",
		Storage:  DefaultLabelStorageTemplate,
	}}
	if err := EnsureTopologyLabelTemplates(cfg); err == nil {
		t.Fatal("EnsureTopologyLabelTemplates() accepted a static label key")
	}
}

func TestNormalizeScaleOutDiscoveryDerivesWriterOwnership(t *testing.T) {
	tests := []struct {
		name         string
		input        ScaleOutSwitchesConfig
		wantEnabled  bool
		wantScaleOut bool
		wantStorage  bool
	}{
		{
			name:         "legacy enabled owns both topologies",
			input:        ScaleOutSwitchesConfig{Enabled: true},
			wantEnabled:  true,
			wantScaleOut: true,
			wantStorage:  true,
		},
		{
			name:        "storage ownership enables discovery",
			input:       ScaleOutSwitchesConfig{ManageStorage: true},
			wantEnabled: true,
			wantStorage: true,
		},
		{
			name:         "scale-out ownership enables discovery",
			input:        ScaleOutSwitchesConfig{ManageScaleOut: true},
			wantEnabled:  true,
			wantScaleOut: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := ScaleOutDiscoveryConfig{Switches: test.input}
			if err := normalizeScaleOutDiscoveryConfig(&cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.Switches.Enabled != test.wantEnabled ||
				cfg.Switches.ManageScaleOut != test.wantScaleOut ||
				cfg.Switches.ManageStorage != test.wantStorage {
				t.Fatalf("normalized switches = %#v", cfg.Switches)
			}
		})
	}
}
