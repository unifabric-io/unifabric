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
	"github.com/unifabric-io/unifabric/pkg/controller"
	"github.com/unifabric-io/unifabric/pkg/logger"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to the configuration file")
	flag.Parse()

	log := logger.MustNew(logger.LevelInfo)
	log.Info("load config file", "path", *configPath)
	cfg, err := config.ReadControllerConfig(*configPath)
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
	log.Debug("create controller")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	server, err := controller.New(ctx, cfg, log)
	if err != nil {
		log.Error("error creating controller", "error", err)
		os.Exit(1)
	}

	log.Info("start controller")
	err = server.Start(ctx)
	if err != nil {
		log.Error("error starting controller", "error", err)
		os.Exit(1)
	}
}
