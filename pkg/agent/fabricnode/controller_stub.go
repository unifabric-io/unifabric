// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package fabricnode

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type Interface interface {
	GetFabricNode() *v1beta1.FabricNode
	IsStorageLeader() bool
}

type unsupportedController struct{}

func (unsupportedController) GetFabricNode() *v1beta1.FabricNode { return nil }

func (unsupportedController) IsStorageLeader() bool { return false }

func NewFabricNodeController(
	_ context.Context,
	_ manager.Manager,
	_ *slog.Logger,
	_ *config.AgentConfig,
) (Interface, error) {
	return unsupportedController{}, fmt.Errorf("fabricnode controller is only supported on linux")
}
