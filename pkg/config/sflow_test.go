// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package config

import "testing"

func TestParseSFlowConfigDefaults(t *testing.T) {
	cfg, err := ParseSFlowConfig([]byte(`
clickhouse:
  address: clickhouse.default.svc:9000
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
	if cfg.ClickHouse.Database != "default" || cfg.ClickHouse.Username != "default" || cfg.ClickHouse.Table != "flows_raw" {
		t.Fatalf("clickhouse defaults = %#v", cfg.ClickHouse)
	}
	if cfg.ClickHouse.Schema.RetentionDays != 3 {
		t.Fatalf("clickhouse schema defaults = %#v", cfg.ClickHouse.Schema)
	}
	if cfg.ClickHouse.Schema.Lock.Name != "unifabric-sflow-clickhouse-schema" ||
		cfg.ClickHouse.Schema.Lock.Namespace != "default" ||
		cfg.ClickHouse.Schema.Lock.LeaseDuration != "2m" ||
		cfg.ClickHouse.Schema.Lock.RetryInterval != "2s" {
		t.Fatalf("clickhouse schema lock defaults = %#v", cfg.ClickHouse.Schema.Lock)
	}
	if cfg.Writer.BatchSize != 2000 || cfg.Writer.FlushInterval != "2s" || cfg.Writer.QueueSize != 65536 {
		t.Fatalf("writer defaults = %#v", cfg.Writer)
	}
}

func TestParseSFlowConfigUsesSchemaRetention(t *testing.T) {
	cfg, err := ParseSFlowConfig([]byte(`
clickhouse:
  address: clickhouse.default.svc:9000
  schema:
    retentionDays: 7
`))
	if err != nil {
		t.Fatalf("ParseSFlowConfig() error = %v", err)
	}
	if cfg.ClickHouse.Schema.RetentionDays != 7 {
		t.Fatalf("retentionDays = %d, want 7", cfg.ClickHouse.Schema.RetentionDays)
	}
}

func TestParseSFlowConfigValidation(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "missing clickhouse address",
			yaml: `clickhouse: {}`,
		},
		{
			name: "bad listen",
			yaml: `
listen:
  bindAddress: "6343"
clickhouse:
  address: clickhouse.default.svc:9000
`,
		},
		{
			name: "bad flush",
			yaml: `
clickhouse:
  address: clickhouse.default.svc:9000
writer:
  flushInterval: nope
`,
		},
		{
			name: "zero flush",
			yaml: `
clickhouse:
  address: clickhouse.default.svc:9000
writer:
  flushInterval: 0s
`,
		},
		{
			name: "bad table",
			yaml: `
clickhouse:
  address: clickhouse.default.svc:9000
  table: "flows_raw;drop"
`,
		},
		{
			name: "negative schema retention",
			yaml: `
clickhouse:
  address: clickhouse.default.svc:9000
  schema:
    retentionDays: -1
`,
		},
		{
			name: "zero schema retention",
			yaml: `
clickhouse:
  address: clickhouse.default.svc:9000
  schema:
    retentionDays: 0
`,
		},
		{
			name: "bad schema lock timing",
			yaml: `
clickhouse:
  address: clickhouse.default.svc:9000
  schema:
    lock:
      leaseDuration: 1s
      retryInterval: 1s
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
