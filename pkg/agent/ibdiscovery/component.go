// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

// Package ibdiscovery discovers InfiniBand switches with ibnetdiscover and
// publishes them as Switch objects.
//
// ibnetdiscover sees a whole subnet from any member port, so one scan per
// subnet is enough. To avoid a scan storm every subnet has a reporter Lease and
// only its holder scans.
package ibdiscovery

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/ib"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	subnetRefresh = time.Minute
	leaseDuration = 30 * time.Second
	renewDeadline = 20 * time.Second
	retryPeriod   = 5 * time.Second
)

type Component struct {
	nodeName  string
	namespace string
	interval  time.Duration
	scanner   scanner
	reader    client.Reader
	writer    *switchWriter
	clientset kubernetes.Interface
	log       *slog.Logger
}

// New creates the discovery component. Add it to the manager with mgr.Add.
func New(mgr manager.Manager, cfg *config.AgentConfig, log *slog.Logger) (*Component, error) {
	interval, err := time.ParseDuration(cfg.IBDiscovery.Interval)
	if err != nil {
		return nil, fmt.Errorf("ibDiscovery.interval: %w", err)
	}
	ttl, err := time.ParseDuration(cfg.IBDiscovery.SwitchTTL)
	if err != nil {
		return nil, fmt.Errorf("ibDiscovery.switchTTL: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(cfg.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}

	var sc scanner = sysfsScanner{root: sysfsInfiniBandRoot}
	if cfg.IBDiscovery.IbnetdiscoverFile != "" {
		sc = fileScanner{path: cfg.IBDiscovery.IbnetdiscoverFile}
	}

	return &Component{
		nodeName:  cfg.Node.Name,
		namespace: cfg.IBDiscovery.Namespace,
		interval:  interval,
		scanner:   sc,
		reader:    mgr.GetAPIReader(),
		writer: &switchWriter{
			client: mgr.GetClient(),
			reader: mgr.GetAPIReader(),
			ttl:    ttl,
			log:    log,
			now:    time.Now,
		},
		clientset: clientset,
		log:       log,
	}, nil
}

// NeedLeaderElection reports false: the agent runs on every node and elects a
// reporter per subnet itself.
func (c *Component) NeedLeaderElection() bool { return false }

// Start keeps one elector per subnet that has an active port on this node.
func (c *Component) Start(ctx context.Context) error {
	electors := map[string]context.CancelFunc{}
	defer func() {
		for _, cancel := range electors {
			cancel()
		}
	}()

	ticker := time.NewTicker(subnetRefresh)
	defer ticker.Stop()
	for {
		subnets, err := c.scanner.Subnets()
		if err != nil {
			c.log.Error("list InfiniBand subnets", "error", err)
		} else {
			c.reconcileElectors(ctx, electors, subnets)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (c *Component) reconcileElectors(ctx context.Context, electors map[string]context.CancelFunc, subnets []string) {
	wanted := make(map[string]struct{}, len(subnets))
	for _, subnet := range subnets {
		wanted[subnet] = struct{}{}
		if _, running := electors[subnet]; running {
			continue
		}
		electorCtx, cancel := context.WithCancel(ctx)
		electors[subnet] = cancel
		go c.elect(electorCtx, subnet)
	}
	for subnet, cancel := range electors {
		if _, ok := wanted[subnet]; !ok {
			cancel()
			delete(electors, subnet)
		}
	}
}

func (c *Component) elect(ctx context.Context, subnet string) {
	name := leaseName(subnet)
	lock, err := resourcelock.New(
		resourcelock.LeasesResourceLock,
		c.namespace,
		name,
		c.clientset.CoreV1(),
		c.clientset.CoordinationV1(),
		resourcelock.ResourceLockConfig{Identity: c.nodeName},
	)
	if err != nil {
		c.log.Error("create reporter lock", "subnet", subnet, "error", err)
		return
	}

	for ctx.Err() == nil {
		leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
			Lock:            lock,
			ReleaseOnCancel: true,
			LeaseDuration:   leaseDuration,
			RenewDeadline:   renewDeadline,
			RetryPeriod:     retryPeriod,
			Name:            name,
			Callbacks: leaderelection.LeaderCallbacks{
				OnStartedLeading: func(leaderCtx context.Context) {
					c.log.Info("became InfiniBand subnet reporter", "subnet", subnet)
					c.scanLoop(leaderCtx, subnet)
				},
				OnStoppedLeading: func() {
					c.log.Info("stopped being InfiniBand subnet reporter", "subnet", subnet)
				},
			},
		})
	}
}

func (c *Component) scanLoop(ctx context.Context, subnet string) {
	for {
		if err := c.scanOnce(ctx, subnet); err != nil {
			c.log.Error("InfiniBand scan failed", "subnet", subnet, "error", err)
		}
		jitter := time.Duration(rand.Int64N(int64(c.interval/10) + 1))
		select {
		case <-ctx.Done():
			return
		case <-time.After(c.interval + jitter):
		}
	}
}

func (c *Component) scanOnce(ctx context.Context, subnet string) error {
	data, err := c.scanner.Scan(ctx, subnet)
	if err != nil {
		return err
	}

	var nodeList corev1.NodeList
	if err := c.reader.List(ctx, &nodeList); err != nil {
		return fmt.Errorf("list Nodes: %w", err)
	}
	nodes := make(map[string]bool, len(nodeList.Items))
	for _, node := range nodeList.Items {
		nodes[node.Name] = true
	}

	discovered, err := ib.Discover(data, nodes)
	if err != nil {
		return fmt.Errorf("parse ibnetdiscover output: %w", err)
	}
	if len(discovered) == 0 {
		// An empty parse usually means a failed scan; keep the existing Switches.
		return fmt.Errorf("ibnetdiscover output contains no switches")
	}
	return c.writer.sync(ctx, subnet, discovered)
}

// leaseName turns a subnet prefix such as fe80:0000:0000:0000 into a valid
// object name.
func leaseName(subnet string) string {
	return "ib-reporter-" + strings.ReplaceAll(subnet, ":", "-")
}
