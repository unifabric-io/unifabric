// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

// Package topologyapi wires the reusable topology HTTP API (pkg/topologyapi)
// into the controller manager, serving this cluster's CRDs as the single
// cluster "default".
package topologyapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/unifabric-io/unifabric/pkg/config"
	api "github.com/unifabric-io/unifabric/pkg/topologyapi"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const shutdownTimeout = 5 * time.Second

// NewTopologyAPIServer registers the Topology HTTP API with mgr. It is a
// no-op unless cfg.TopologyAPI.Enabled is set, since the API has no
// authentication of its own and should be opted into explicitly.
func NewTopologyAPIServer(mgr manager.Manager, cfg *config.ControllerConfig, logger *slog.Logger) error {
	if cfg == nil {
		return fmt.Errorf("topology API server config must not be nil")
	}
	if logger == nil {
		return fmt.Errorf("topology API server logger must not be nil")
	}
	if !cfg.TopologyAPI.Enabled {
		return nil
	}

	log := logger.With("controller", "TopologyAPI")

	return mgr.Add(&server{
		addr:    cfg.TopologyAPI.BindAddress,
		handler: api.NewHandler(api.SingleCluster(mgr.GetClient()), log),
		log:     log,
	})
}

// server adapts a plain http.Server to the controller-runtime manager.Runnable
// interface so it starts and stops alongside the rest of the manager.
type server struct {
	addr    string
	handler http.Handler
	log     *slog.Logger
}

// NeedLeaderElection reports false: every replica serves reads from its own
// cache, so the API should stay available regardless of which replica leads.
func (s *server) NeedLeaderElection() bool {
	return false
}

func (s *server) Start(ctx context.Context) error {
	httpServer := &http.Server{
		Addr:              s.addr,
		Handler:           s.handler,
		ReadHeaderTimeout: shutdownTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		s.log.Info("starting topology API server", "address", s.addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
