// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"log/slog"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	switchtopology "github.com/unifabric-io/unifabric/pkg/controller/switchtopology"
	"github.com/unifabric-io/unifabric/pkg/logger"
	"github.com/unifabric-io/unifabric/pkg/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func New(_ context.Context, cfg *config.ControllerConfig, slogger *slog.Logger) (types.Service, error) {
	log.SetLogger(logger.ToLogr(slogger))

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	mgrOpts := manager.Options{
		Scheme:                  scheme,
		LeaderElection:          cfg.LeaderElection.Enabled,
		LeaderElectionID:        cfg.LeaderElection.ID,
		LeaderElectionNamespace: cfg.LeaderElection.Namespace,
		Logger:                  logger.ToLogr(logger.WithName(slogger, "controller-runtime")),
		PprofBindAddress:        cfg.Pprof.BindAddress,
		HealthProbeBindAddress:  cfg.HealthProbe.BindAddress,
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

	if err := switchtopology.NewSwitchTopologyDiscoveryController(mgr, cfg, slogger); err != nil {
		return nil, err
	}

	return mgr, nil
}
