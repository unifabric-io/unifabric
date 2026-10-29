// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	sflowcollector "github.com/unifabric-io/unifabric/pkg/sflow"

	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/logger"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "mock" {
		if err := runMockSender(os.Args[2:]); err != nil {
			slog.Error("mock sender failed", "error", err)
			os.Exit(1)
		}
		return
	}

	configPath := flag.String("config", "config.yaml", "Path to the configuration file")
	flag.Parse()

	log := logger.MustNew(logger.LevelInfo)
	log.Info("load config file", "path", *configPath)
	cfg, err := config.ReadSFlowConfig(*configPath)
	if err != nil {
		log.Error("error reading config file", "error", err)
		os.Exit(1)
	}

	log, err = logger.New(cfg.LogLevel)
	if err != nil {
		slog.Error("error creating logger", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, log); err != nil {
		log.Error("error running sflow collector", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg *config.SFlowConfig, log *slog.Logger) error {
	ctrl.SetLogger(logger.ToLogr(log))

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return err
	}
	mgr, err := ctrl.NewManager(cfg.KubeConfig, ctrl.Options{
		Logger:                 logger.ToLogr(logger.WithName(log, "sflow-runtime")),
		Scheme:                 scheme,
		HealthProbeBindAddress: cfg.HealthProbe.BindAddress,
		Metrics:                serverOptions(cfg.Metrics.BindAddress),
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&corev1.Pod{}: {},
			},
		},
	})
	if err != nil {
		return err
	}
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return err
	}

	podCache := sflowcollector.NewPodIPCache()
	ownerResolver := sflowcollector.KubernetesOwnerResolver{Client: mgr.GetAPIReader()}
	if err := mgr.Add(&podCacheUpdater{
		Client:        mgr.GetClient(),
		Cache:         podCache,
		OwnerResolver: ownerResolver,
		Log:           logger.WithName(log, "pod_cache"),
	}); err != nil {
		return err
	}

	writer, err := sflowcollector.NewOtelWriter(ctx, cfg.Otel, cfg.Writer)
	if err != nil {
		return err
	}
	defer writer.Close()

	collector, err := sflowcollector.NewCollector(cfg, podCache, writer, logger.WithName(log, "sflow_collector"))
	if err != nil {
		return err
	}
	metrics.Registry.MustRegister(collector.Metrics)

	if err := mgr.Add(&collectorRunnable{Collector: collector}); err != nil {
		return err
	}
	log.Info("start sflow collector", "listen", cfg.Listen.BindAddress)
	return mgr.Start(ctx)
}

func serverOptions(bindAddress string) metricsserver.Options {
	return metricsserver.Options{
		BindAddress: bindAddress,
	}
}

type collectorRunnable struct {
	Collector *sflowcollector.Collector
}

func (r *collectorRunnable) Start(ctx context.Context) error {
	return r.Collector.Run(ctx)
}

type podCacheUpdater struct {
	Client        client.Client
	Cache         *sflowcollector.PodIPCache
	OwnerResolver sflowcollector.OwnerResolver
	Log           *slog.Logger
}

func (u *podCacheUpdater) Start(ctx context.Context) error {
	if err := u.refresh(ctx); err != nil && u.Log != nil {
		u.Log.Warn("initial pod cache refresh failed", "error", err)
	}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := u.refresh(ctx); err != nil && u.Log != nil {
				u.Log.Warn("pod cache refresh failed", "error", err)
			}
		}
	}
}

func (u *podCacheUpdater) NeedLeaderElection() bool {
	return false
}

func (u *podCacheUpdater) refresh(ctx context.Context) error {
	podList := &corev1.PodList{}
	if err := u.Client.List(ctx, podList); err != nil {
		return err
	}
	return u.Cache.ReplacePods(ctx, podList.Items, u.OwnerResolver)
}

func (u *podCacheUpdater) InjectScheme(_ *runtime.Scheme) error {
	return nil
}
