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

	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/logger"
	"github.com/unifabric-io/unifabric/pkg/switchagent"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to the configuration file")
	flag.Parse()

	log := logger.MustNew(logger.LevelInfo)
	log.Info("load config file", "path", *configPath)
	cfg, err := config.ReadSwitchAgentConfig(*configPath)
	if err != nil && os.IsNotExist(err) && *configPath == "config.yaml" {
		log.Info("switch-agent config file not found, using built-in defaults", "path", *configPath)
		cfg, err = config.ReadSwitchAgentConfig("")
	}
	if err != nil {
		log.Error("error reading config file", "error", err)
		os.Exit(1)
	}

	log, err = logger.New(cfg.LogLevel)
	if err != nil {
		slog.Error("error creating logger", "error", err)
		os.Exit(1)
	}
	log.Debug("config file context", "config", cfg)
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
