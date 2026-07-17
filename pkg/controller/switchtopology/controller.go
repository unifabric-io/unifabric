// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchtopology

import (
	"fmt"
	"log/slog"

	"github.com/unifabric-io/unifabric/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func NewSwitchTopologyDiscoveryController(mgr manager.Manager, cfg *config.ControllerConfig, logger *slog.Logger) error {
	if cfg == nil {
		return fmt.Errorf("switch topology controller config must not be nil")
	}
	if logger == nil {
		return fmt.Errorf("switch topology controller logger must not be nil")
	}

	log := logger.With("controller", "SwitchTopologyDiscovery")
	if err := addSwitchSubscriptionManager(mgr, cfg, log); err != nil {
		return err
	}
	if !switchDiscoveryEnabled(cfg) {
		return nil
	}
	if !autoDiscoveryEnabled(cfg) {
		return nil
	}
	if err := config.EnsureTopologyLabelTemplates(cfg); err != nil {
		return err
	}
	return newAutoDiscoveredTopologyLabelController(mgr, cfg, log)
}

func addSwitchSubscriptionManager(mgr manager.Manager, cfg *config.ControllerConfig, log *slog.Logger) error {
	if !switchDiscoveryEnabled(cfg) {
		return nil
	}

	subscriptionManager, err := newSubscriptionManager(mgr.GetClient(), cfg, log)
	if err != nil {
		return err
	}
	return mgr.Add(subscriptionManager)
}

func switchDiscoveryEnabled(cfg *config.ControllerConfig) bool {
	return cfg.ScaleOutDiscovery.Switches.Enabled
}

func autoDiscoveryEnabled(cfg *config.ControllerConfig) bool {
	return cfg.ScaleOutDiscovery.Switches.ManageScaleOut || cfg.ScaleOutDiscovery.Switches.ManageStorage
}
