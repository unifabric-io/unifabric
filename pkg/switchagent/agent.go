// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package switchagent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
)

const minAcceptedClientKeepaliveTime = 20 * time.Second

type Agent struct {
	UnimplementedSwitchReporterServer

	config *config.SwitchAgentConfig
	log    *slog.Logger
	tls    *tls.Config

	snapshotMu      sync.RWMutex
	currentSnapshot *LLDPNeighborSnapshot
	subscribers     map[int]chan *LLDPNeighborSnapshot
	nextSubscriber  int
}

var _ types.Service = (*Agent)(nil)

func New(_ context.Context, cfg *config.SwitchAgentConfig, log *slog.Logger) (types.Service, error) {
	var tlsConfig *tls.Config
	var err error
	if cfg.MTLSEnabled() {
		tlsConfig, err = loadPinnedMTLSConfig(cfg)
		if err != nil {
			return nil, err
		}
	}

	return &Agent{
		config:      cfg,
		log:         log,
		tls:         tlsConfig,
		subscribers: map[int]chan *LLDPNeighborSnapshot{},
	}, nil
}

func (a *Agent) Start(ctx context.Context) error {
	if err := a.refreshSnapshot(); err != nil {
		a.log.Warn("initial lldp snapshot collection failed", "error", err)
	}

	listenConfig := net.ListenConfig{}
	listener, err := listenConfig.Listen(ctx, "tcp", a.config.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", a.config.ListenAddress, err)
	}
	defer listener.Close()

	serverOptions := grpcServerOptions(a.tls)
	grpcServer := grpc.NewServer(serverOptions...)
	RegisterSwitchReporterServer(grpcServer, a)

	errCh := make(chan error, 1)
	go func() {
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			errCh <- err
		}
	}()
	go a.runSnapshotLoop(ctx)

	a.log.Info(
		"start switch agent",
		"switchName", a.config.SwitchName,
		"listenAddress", a.config.ListenAddress,
		"mtlsEnabled", a.config.MTLSEnabled(),
		"lldpRefreshInterval", a.config.LLDP.RefreshInterval,
		"lldpCollectionMode", a.config.LLDP.CollectionMode,
		"lldpSocketPath", a.config.LLDP.SocketPath,
		"lldpCLIVersion", a.config.LLDP.CLIVersion,
	)

	select {
	case <-ctx.Done():
		grpcServer.GracefulStop()
	case err := <-errCh:
		grpcServer.GracefulStop()
		return fmt.Errorf("serve switch agent grpc server: %w", err)
	}

	a.log.Info("stop switch agent")
	return nil
}

func (a *Agent) WatchLLDPNeighbors(req *WatchLLDPNeighborsRequest, stream grpc.ServerStreamingServer[LLDPNeighborSnapshot]) error {
	if req.GetExpectedSwitchName() != "" && !switchNameMatches(req.GetExpectedSwitchName(), a.config.SwitchName) {
		return status.Errorf(codes.FailedPrecondition, "expected_switch_name %q does not match local switch %q", req.GetExpectedSwitchName(), a.config.SwitchName)
	}

	snapshot, subscriptionID, updates := a.subscribe()
	defer a.unsubscribe(subscriptionID)

	if snapshot == nil {
		return status.Error(codes.Unavailable, "lldp snapshot not ready")
	}

	lastGeneration := snapshot.GetGeneration()
	if err := stream.Send(snapshot); err != nil {
		return err
	}

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case snapshot := <-updates:
			if snapshot == nil || snapshot.GetGeneration() <= lastGeneration {
				continue
			}
			if err := stream.Send(snapshot); err != nil {
				return err
			}
			lastGeneration = snapshot.GetGeneration()
		}
	}
}

func (a *Agent) runSnapshotLoop(ctx context.Context) {
	interval, err := time.ParseDuration(a.config.LLDP.RefreshInterval)
	if err != nil {
		a.log.Error("invalid lldp refresh interval", "error", err, "refreshInterval", a.config.LLDP.RefreshInterval)
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.refreshSnapshot(); err != nil {
				a.log.Warn("refresh lldp snapshot failed", "error", err)
			}
		}
	}
}

func (a *Agent) refreshSnapshot() error {
	a.snapshotMu.RLock()
	current := a.currentSnapshot
	nextGeneration := uint64(1)
	if current != nil {
		nextGeneration = current.GetGeneration() + 1
	}
	a.snapshotMu.RUnlock()

	snapshot, err := collectSnapshot(a.config, nextGeneration)
	if err != nil {
		return err
	}

	a.snapshotMu.Lock()
	defer a.snapshotMu.Unlock()
	if snapshotsEqual(a.currentSnapshot, snapshot) {
		return nil
	}

	a.currentSnapshot = snapshot
	for _, subscriber := range a.subscribers {
		select {
		case subscriber <- snapshot:
		default:
			select {
			case <-subscriber:
			default:
			}
			subscriber <- snapshot
		}
	}

	a.log.Debug("broadcasted new lldp snapshot", "switchName", snapshot.GetSwitchName(), "generation", snapshot.GetGeneration(), "neighborCount", len(snapshot.GetLldpNeighbors()))
	return nil
}

func (a *Agent) subscribe() (*LLDPNeighborSnapshot, int, chan *LLDPNeighborSnapshot) {
	a.snapshotMu.Lock()
	defer a.snapshotMu.Unlock()

	id := a.nextSubscriber
	a.nextSubscriber++
	updates := make(chan *LLDPNeighborSnapshot, 1)
	a.subscribers[id] = updates

	return a.currentSnapshot, id, updates
}

func (a *Agent) unsubscribe(id int) {
	a.snapshotMu.Lock()
	defer a.snapshotMu.Unlock()

	updates, ok := a.subscribers[id]
	if !ok {
		return
	}
	delete(a.subscribers, id)
	close(updates)
}

func switchNameMatches(expected, actual string) bool {
	if expected == "" {
		return true
	}
	return normalizeSwitchName(expected) == normalizeSwitchName(actual)
}

func grpcKeepaliveEnforcementPolicy() keepalive.EnforcementPolicy {
	return keepalive.EnforcementPolicy{
		// The controller keeps one long-lived stream open and pings every 30s by default.
		// The grpc-go server default minimum is 5m, which triggers GOAWAY too_many_pings.
		MinTime:             minAcceptedClientKeepaliveTime,
		PermitWithoutStream: true,
	}
}

func grpcServerOptions(tlsConfig *tls.Config) []grpc.ServerOption {
	serverOptions := []grpc.ServerOption{
		grpc.KeepaliveEnforcementPolicy(grpcKeepaliveEnforcementPolicy()),
	}
	if tlsConfig != nil {
		serverOptions = append(serverOptions, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}
	return serverOptions
}

func loadPinnedMTLSConfig(cfg *config.SwitchAgentConfig) (*tls.Config, error) {
	serverCert, err := tls.LoadX509KeyPair(cfg.MTLS.CertFile, cfg.MTLS.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load switch-agent server keypair: %w", err)
	}

	peerPEM, err := os.ReadFile(cfg.MTLS.PeerCertFile)
	if err != nil {
		return nil, fmt.Errorf("load pinned controller certificate: %w", err)
	}

	block, _ := pem.Decode(peerPEM)
	if block == nil {
		return nil, fmt.Errorf("parse pinned controller certificate: no PEM data found")
	}
	expectedPeer, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse pinned controller certificate: %w", err)
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAnyClientCert,
		NextProtos:   []string{"h2"},
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("missing client certificate")
			}

			presentedPeer, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				return fmt.Errorf("parse presented client certificate: %w", err)
			}
			if !bytes.Equal(presentedPeer.Raw, expectedPeer.Raw) {
				return fmt.Errorf("pinned controller certificate mismatch")
			}

			return nil
		},
	}, nil
}
