// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package ebpfflow

import (
	"testing"

	"github.com/unifabric-io/unifabric/pkg/config"
)

func TestOptionsFromConfigFlowEventsEnabled(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		enabled *bool
		want    bool
	}{
		{name: "legacy nil defaults on", want: true},
		{name: "explicitly enabled", enabled: boolPtr(true), want: true},
		{name: "flow events disabled", enabled: boolPtr(false), want: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := config.EBPFFlowConfig{
				DiscoveryInterval: "5s",
				ReportInterval:    "5s",
				Endpoint: config.EBPFFlowEndpointConfig{
					Interval:    "30s",
					MinInterval: "3s",
				},
				FlowEvents: config.EBPFFlowFlowEventsConfig{
					Enabled:   testCase.enabled,
					Flush:     "1s",
					Aggregate: "0s",
				},
				OTLP: config.EBPFFlowOTLPConfig{Timeout: "10s"},
			}
			opts, err := optionsFromConfig(cfg, "node-a")
			if err != nil {
				t.Fatalf("options from config: %v", err)
			}
			if opts.sendsEnabled != testCase.want {
				t.Fatalf("sendsEnabled = %v, want %v", opts.sendsEnabled, testCase.want)
			}
		})
	}
}
