// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchtopology

import (
	"strings"
	"testing"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/logger"
)

func TestAddSwitchSubscriptionManagerNoopsWhenDisabled(t *testing.T) {
	if err := addSwitchSubscriptionManager(nil, &config.ControllerConfig{}, logger.MustNew(logger.LevelDebug)); err != nil {
		t.Fatalf("expected disabled switch subscription manager setup to no-op, got %v", err)
	}
}

func TestNewSwitchTopologyDiscoveryControllerRequiresConfig(t *testing.T) {
	err := NewSwitchTopologyDiscoveryController(nil, nil, logger.MustNew(logger.LevelDebug))
	if err == nil {
		t.Fatal("expected nil controller config to be rejected")
	}
	if !strings.Contains(err.Error(), "config must not be nil") {
		t.Fatalf("expected nil config error, got %v", err)
	}
}

func TestConfiguredManagedTopologiesRespectsOwnership(t *testing.T) {
	cfg := &config.ControllerConfig{}
	if err := config.EnsureTopologyLabelTemplates(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.ScaleOutDiscovery.Switches.ManageStorage = true

	topologies := configuredManagedTopologies(cfg)
	if len(topologies) != 1 || topologies[0].name != v1beta1.TopologyStorage {
		t.Fatalf("managed topologies = %#v", topologies)
	}
	if !autoDiscoveryEnabled(cfg) {
		t.Fatal("storage ownership did not enable auto discovery")
	}
}
