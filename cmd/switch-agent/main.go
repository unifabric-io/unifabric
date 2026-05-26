// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/logger"
	"github.com/unifabric-io/unifabric/pkg/switchagent"
)

func main() {
	log := logger.MustNew(logger.LevelInfo)
	log.Info("load switch-agent config", "source", "env")
	cfg, err := config.ReadSwitchAgentConfigFromEnv()
	if err != nil {
		log.Error("error reading switch-agent config", "error", err)
		os.Exit(1)
	}

	log, err = logger.New(cfg.LogLevel)
	if err != nil {
		slog.Error("error creating logger", "error", err)
		os.Exit(1)
	}
	log.Debug("switch-agent config context", "config", cfg)
	log.Debug("create switch agent")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	agent, err := switchagent.New(ctx, cfg, log)
	if err != nil {
		log.Error("error creating switch agent", "error", err)
		os.Exit(1)
	}

	log.Info("start switch agent")
	if err := agent.Start(ctx); err != nil {
		log.Error("error starting switch agent", "error", err)
		os.Exit(1)
	}
}
