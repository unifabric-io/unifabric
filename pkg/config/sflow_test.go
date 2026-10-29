// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package config

import "testing"

func TestParseSFlowConfigDefaults(t *testing.T) {
	cfg, err := ParseSFlowConfig([]byte(`
otel:
  endpoint: collector.default.svc:4319
`))
	if err != nil {
		t.Fatalf("ParseSFlowConfig() error = %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("log level = %q, want info", cfg.LogLevel)
	}
	if cfg.Listen.BindAddress != ":6343" {
		t.Fatalf("listen = %q, want :6343", cfg.Listen.BindAddress)
	}
	if cfg.Metrics.BindAddress != ":8084" || cfg.HealthProbe.BindAddress != ":8085" {
		t.Fatalf("metrics/health = %q/%q", cfg.Metrics.BindAddress, cfg.HealthProbe.BindAddress)
	}
	if cfg.Otel.Timeout != "10s" || cfg.Otel.Insecure {
		t.Fatalf("otel defaults = %#v", cfg.Otel)
	}
	if cfg.Writer.BatchSize != 2000 || cfg.Writer.FlushInterval != "2s" || cfg.Writer.QueueSize != 65536 {
		t.Fatalf("writer defaults = %#v", cfg.Writer)
	}
}

func TestParseSFlowConfigValidation(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "missing otel endpoint",
			yaml: `otel: {}`,
		},
		{
			name: "bad otel endpoint",
			yaml: `
otel:
  endpoint: collector.default.svc
`,
		},
		{
			name: "bad otel timeout",
			yaml: `
otel:
  endpoint: collector.default.svc:4319
  timeout: nope
`,
		},
		{
			name: "zero otel timeout",
			yaml: `
otel:
  endpoint: collector.default.svc:4319
  timeout: 0s
`,
		},
		{
			name: "bad listen",
			yaml: `
listen:
  bindAddress: "6343"
otel:
  endpoint: collector.default.svc:4319
`,
		},
		{
			name: "bad flush",
			yaml: `
otel:
  endpoint: collector.default.svc:4319
writer:
  flushInterval: nope
`,
		},
		{
			name: "zero flush",
			yaml: `
otel:
  endpoint: collector.default.svc:4319
writer:
  flushInterval: 0s
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseSFlowConfig([]byte(tt.yaml)); err == nil {
				t.Fatalf("ParseSFlowConfig() error = nil")
			}
		})
	}
}
