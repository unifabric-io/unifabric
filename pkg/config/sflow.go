// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"time"

	"github.com/unifabric-io/unifabric/pkg/logger"
	yaml "gopkg.in/yaml.v2"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	defaultSFlowListenBindAddress  = ":6343"
	defaultSFlowMetricsBindAddress = ":8084"
	defaultSFlowHealthBindAddress  = ":8085"
	defaultSFlowDatabase           = "default"
	defaultSFlowUsername           = "default"
	defaultSFlowTable              = "flows_raw"
	defaultSFlowBatchSize          = 2000
	defaultSFlowFlushInterval      = "2s"
	defaultSFlowQueueSize          = 65536
)

var sflowTableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)

type SFlowClickHouseConfig struct {
	Address  string `json:"address" yaml:"address"`
	Database string `json:"database" yaml:"database"`
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
	Table    string `json:"table" yaml:"table"`
}

type SFlowWriterConfig struct {
	BatchSize     int    `json:"batchSize" yaml:"batchSize"`
	FlushInterval string `json:"flushInterval" yaml:"flushInterval"`
	QueueSize     int    `json:"queueSize" yaml:"queueSize"`
}

type SFlowConfig struct {
	LogLevel    string                `json:"logLevel" yaml:"logLevel"`
	Listen      BindAddressConfig     `json:"listen" yaml:"listen"`
	Metrics     BindAddressConfig     `json:"metrics" yaml:"metrics"`
	HealthProbe BindAddressConfig     `json:"healthProbe" yaml:"healthProbe"`
	ClickHouse  SFlowClickHouseConfig `json:"clickhouse" yaml:"clickhouse"`
	Writer      SFlowWriterConfig     `json:"writer" yaml:"writer"`
	KubeConfig  *rest.Config          `json:"-" yaml:"-"`
}

func ReadSFlowConfig(filename string) (*SFlowConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	cfg, err := ParseSFlowConfig(data)
	if err != nil {
		return nil, err
	}

	cfg.KubeConfig, err = ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}
	return cfg, nil
}

func ParseSFlowConfig(data []byte) (*SFlowConfig, error) {
	var cfg SFlowConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if err := NormalizeSFlowConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func NormalizeSFlowConfig(cfg *SFlowConfig) error {
	var err error
	cfg.LogLevel, err = logger.NormalizeLevel(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("sflow.logLevel: %w", err)
	}

	if cfg.Listen.BindAddress == "" {
		cfg.Listen.BindAddress = defaultSFlowListenBindAddress
	}
	if cfg.Metrics.BindAddress == "" {
		cfg.Metrics.BindAddress = defaultSFlowMetricsBindAddress
	}
	if cfg.HealthProbe.BindAddress == "" {
		cfg.HealthProbe.BindAddress = defaultSFlowHealthBindAddress
	}
	if err := validateBindAddress("sflow.listen.bindAddress", cfg.Listen.BindAddress); err != nil {
		return err
	}
	if err := validateBindAddress("sflow.metrics.bindAddress", cfg.Metrics.BindAddress); err != nil {
		return err
	}
	if err := validateBindAddress("sflow.healthProbe.bindAddress", cfg.HealthProbe.BindAddress); err != nil {
		return err
	}

	if cfg.ClickHouse.Address == "" {
		return fmt.Errorf("sflow.clickhouse.address is required")
	}
	if _, _, err := net.SplitHostPort(cfg.ClickHouse.Address); err != nil {
		return fmt.Errorf("sflow.clickhouse.address: %s is invalid, expect host:port", cfg.ClickHouse.Address)
	}
	if cfg.ClickHouse.Database == "" {
		cfg.ClickHouse.Database = defaultSFlowDatabase
	}
	if cfg.ClickHouse.Username == "" {
		cfg.ClickHouse.Username = defaultSFlowUsername
	}
	if cfg.ClickHouse.Table == "" {
		cfg.ClickHouse.Table = defaultSFlowTable
	}
	if !sflowTableNamePattern.MatchString(cfg.ClickHouse.Table) {
		return fmt.Errorf("sflow.clickhouse.table: %s is invalid", cfg.ClickHouse.Table)
	}

	if cfg.Writer.BatchSize == 0 {
		cfg.Writer.BatchSize = defaultSFlowBatchSize
	}
	if cfg.Writer.BatchSize < 0 {
		return fmt.Errorf("sflow.writer.batchSize must be positive")
	}
	if cfg.Writer.FlushInterval == "" {
		cfg.Writer.FlushInterval = defaultSFlowFlushInterval
	}
	flushInterval, err := time.ParseDuration(cfg.Writer.FlushInterval)
	if err != nil {
		return fmt.Errorf("sflow.writer.flushInterval: %s is invalid, expect format like 2s or 500ms", cfg.Writer.FlushInterval)
	}
	if flushInterval <= 0 {
		return fmt.Errorf("sflow.writer.flushInterval must be positive")
	}
	if cfg.Writer.QueueSize == 0 {
		cfg.Writer.QueueSize = defaultSFlowQueueSize
	}
	if cfg.Writer.QueueSize < 0 {
		return fmt.Errorf("sflow.writer.queueSize must be positive")
	}
	return nil
}

func validateBindAddress(field, address string) error {
	if _, _, err := net.SplitHostPort(address); err != nil {
		return fmt.Errorf("%s: %s is invalid, expect host:port or :port", field, address)
	}
	return nil
}
