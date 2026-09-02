// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package sflow

import (
	"context"
	"fmt"
	"time"

	"github.com/unifabric-io/unifabric/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

func ApplyClickHouseSchemaWithKubernetesLock(ctx context.Context, restConfig *rest.Config, cfg config.SFlowClickHouseConfig, apply func(context.Context) error) error {
	if restConfig == nil {
		return fmt.Errorf("kubernetes config is required for clickhouse schema migration lock")
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create kubernetes client for clickhouse schema migration lock: %w", err)
	}

	leaseDuration, err := time.ParseDuration(cfg.Schema.Lock.LeaseDuration)
	if err != nil {
		return fmt.Errorf("parse clickhouse schema migration lock lease duration: %w", err)
	}
	retryInterval, err := time.ParseDuration(cfg.Schema.Lock.RetryInterval)
	if err != nil {
		return fmt.Errorf("parse clickhouse schema migration lock retry interval: %w", err)
	}
	renewDeadline, err := clickHouseSchemaLockTiming(leaseDuration, retryInterval)
	if err != nil {
		return err
	}

	identity := fmt.Sprintf("sflow-%s", uuid.NewUUID())
	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      cfg.Schema.Lock.Name,
			Namespace: cfg.Schema.Lock.Namespace,
		},
		Client: clientset.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: string(identity),
		},
		Labels: map[string]string{
			"app.kubernetes.io/component": "unifabric-sflow",
			"unifabric.io/lock":           "sflow-clickhouse-schema",
		},
	}

	return withLeaderElectionLock(ctx, lock, leaseDuration, renewDeadline, retryInterval, apply)
}

func clickHouseSchemaLockTiming(leaseDuration, retryInterval time.Duration) (time.Duration, error) {
	if leaseDuration <= 0 {
		return 0, fmt.Errorf("clickhouse schema migration lock leaseDuration must be positive")
	}
	if retryInterval <= 0 {
		return 0, fmt.Errorf("clickhouse schema migration lock retryInterval must be positive")
	}
	if retryInterval*2 >= leaseDuration {
		return 0, fmt.Errorf("clickhouse schema migration lock retryInterval must be less than half of leaseDuration")
	}
	renewDeadline := leaseDuration * 2 / 3
	if renewDeadline <= time.Duration(leaderelection.JitterFactor*float64(retryInterval)) {
		return 0, fmt.Errorf("clickhouse schema migration lock renewDeadline must be greater than retryInterval*JitterFactor")
	}
	return renewDeadline, nil
}

func withLeaderElectionLock(ctx context.Context, lock resourcelock.Interface, leaseDuration, renewDeadline, retryInterval time.Duration, apply func(context.Context) error) error {
	if apply == nil {
		return fmt.Errorf("clickhouse schema migration apply function is nil")
	}
	lockCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan error, 1)
	config := leaderelection.LeaderElectionConfig{
		Lock:            lock,
		LeaseDuration:   leaseDuration,
		RenewDeadline:   renewDeadline,
		RetryPeriod:     retryInterval,
		ReleaseOnCancel: true,
		Name:            "sflow-clickhouse-schema",
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(leaderCtx context.Context) {
				done <- apply(leaderCtx)
				cancel()
			},
			OnStoppedLeading: func() {},
		},
	}
	elector, err := leaderelection.NewLeaderElector(config)
	if err != nil {
		return fmt.Errorf("create clickhouse schema migration leader elector: %w", err)
	}

	go elector.Run(lockCtx)

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
