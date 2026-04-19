// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/unifabric-io/unifabric/pkg/agent/fabricnode"
	"github.com/unifabric-io/unifabric/pkg/agent/rdmascraper"
	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/logger"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

func New(ctx context.Context, cfg *config.AgentConfig, log *slog.Logger) (ctrl.Manager, error) {
	if cfg.Node.Name == "" {
		cfg.Node.Name = os.Getenv("NODE_NAME")
		if cfg.Node.Name == "" {
			data, err := os.ReadFile("/etc/hostname")
			if err != nil {
				return nil, fmt.Errorf("failed to read /etc/hostname: %w", err)
			}
			cfg.Node.Name = strings.TrimSpace(string(data))
		}
	}
	if cfg.Node.Name == "" {
		return nil, fmt.Errorf("node name is not specified in config file or environment variable NODE_NAME")
	}
	ctrl.SetLogger(logger.ToLogr(log))

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	cacheOption := cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&v1.Pod{}: {
				Field: fields.OneTermEqualSelector("spec.nodeName", cfg.Node.Name),
			},
		},
	}
	mgrOpts := ctrl.Options{
		Logger:                 logger.ToLogr(logger.WithName(log, "agent-runtime")),
		Cache:                  cacheOption,
		Scheme:                 scheme,
		HealthProbeBindAddress: cfg.HealthProbe.BindAddress,
	}

	if cfg.Metrics.BindAddress != "" {
		mgrOpts.Metrics.BindAddress = cfg.Metrics.BindAddress
	}

	mgr, err := ctrl.NewManager(cfg.KubeConfig, mgrOpts)
	if err != nil {
		return nil, err
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return nil, err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return nil, err
	}

	fabricNodeCli, err := fabricnode.NewFabricNodeController(ctx, mgr, log, cfg)
	if err != nil {
		return nil, err
	}

	scraper := rdmascraper.NewRuntimeScraper(fabricNodeCli, logger.WithName(log, "rdma_scraper"), cfg.NodeTopologyDiscovery)
	collector := rdmascraper.NewCollector(scraper, logger.WithName(log, "rdma_collector"))
	metrics.Registry.MustRegister(collector)

	return mgr, nil
}
