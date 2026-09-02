// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/unifabric-io/unifabric/pkg/config"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

func TestWithLeaderElectionLockRejectsNilApply(t *testing.T) {
	err := withLeaderElectionLock(context.Background(), &noopLock{}, time.Second, 500*time.Millisecond, 100*time.Millisecond, nil)
	if err == nil || !strings.Contains(err.Error(), "apply function is nil") {
		t.Fatalf("withLeaderElectionLock() error = %v, want nil apply error", err)
	}
}

func TestApplyClickHouseSchemaWithKubernetesLockRequiresKubeConfig(t *testing.T) {
	err := ApplyClickHouseSchemaWithKubernetesLock(context.Background(), nil, testLockConfig(), func(context.Context) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "kubernetes config is required") {
		t.Fatalf("ApplyClickHouseSchemaWithKubernetesLock() error = %v, want kubeconfig error", err)
	}
}

func TestApplyClickHouseSchemaWithKubernetesLockValidatesTiming(t *testing.T) {
	cfg := testLockConfig()
	cfg.Schema.Lock.LeaseDuration = "1s"
	cfg.Schema.Lock.RetryInterval = "1s"
	err := ApplyClickHouseSchemaWithKubernetesLock(context.Background(), &restConfigStub, cfg, func(context.Context) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "retryInterval must be less than half") {
		t.Fatalf("ApplyClickHouseSchemaWithKubernetesLock() error = %v, want timing error", err)
	}
}

func TestClickHouseSchemaLockTiming(t *testing.T) {
	renewDeadline, err := clickHouseSchemaLockTiming(2*time.Minute, 2*time.Second)
	if err != nil {
		t.Fatalf("clickHouseSchemaLockTiming() error = %v", err)
	}
	if renewDeadline <= time.Duration(leadElectionJitterFactorForTest*float64(2*time.Second)) {
		t.Fatalf("renewDeadline = %s, want greater than retryInterval jitter", renewDeadline)
	}

	if _, err := clickHouseSchemaLockTiming(10*time.Second, 5*time.Second); err == nil {
		t.Fatalf("clickHouseSchemaLockTiming() error = nil, want invalid timing")
	}
}

const leadElectionJitterFactorForTest = 1.2

var restConfigStub = restConfigForTest()

func restConfigForTest() rest.Config {
	return rest.Config{Host: "https://127.0.0.1"}
}

func testLockConfig() config.SFlowClickHouseConfig {
	return config.SFlowClickHouseConfig{
		Schema: config.SFlowClickHouseSchemaConfig{
			Lock: config.SFlowClickHouseSchemaLockConfig{
				Name:          "schema-lock",
				Namespace:     "default",
				LeaseDuration: "2s",
				RetryInterval: "100ms",
			},
		},
	}
}

type noopLock struct{}

func (noopLock) Get(context.Context) (*resourcelock.LeaderElectionRecord, []byte, error) {
	return nil, nil, nil
}

func (noopLock) Create(context.Context, resourcelock.LeaderElectionRecord) error {
	return nil
}

func (noopLock) Update(context.Context, resourcelock.LeaderElectionRecord) error {
	return nil
}

func (noopLock) RecordEvent(string) {}

func (noopLock) Identity() string {
	return "noop"
}

func (noopLock) Describe() string {
	return "noop"
}
