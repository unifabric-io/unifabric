// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/unifabric-io/unifabric/pkg/agent/ebpfflow"
	"github.com/unifabric-io/unifabric/pkg/agent/fabricnode"
	"github.com/unifabric-io/unifabric/pkg/agent/rdmascraper"
	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	"github.com/unifabric-io/unifabric/pkg/logger"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

func New(ctx context.Context, cfg *config.AgentConfig, log *slog.Logger) (ctrl.Manager, error) {
	if cfg.Node.Name == "" {
		cfg.Node.Name = os.Getenv("NODE_NAME")
		if cfg.Node.Name == "" {
			data, err := os.ReadFile("/etc/hostname")
			if err != nil {
				return nil, fmt.Errorf("failed to read /etc/hostname: %w", err)
			}
			cfg.Node.Name = strings.TrimSpace(string(data))
		}
	}
	if cfg.Node.Name == "" {
		return nil, fmt.Errorf("node name is not specified in config file or environment variable NODE_NAME")
	}
	ctrl.SetLogger(logger.ToLogr(log))

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := v1beta1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	cacheOption := cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&v1.Pod{}: {
				Field: fields.OneTermEqualSelector("spec.nodeName", cfg.Node.Name),
			},
		},
	}
	mgrOpts := ctrl.Options{
		Logger:                 logger.ToLogr(logger.WithName(log, "agent-runtime")),
		Cache:                  cacheOption,
		Scheme:                 scheme,
		HealthProbeBindAddress: cfg.HealthProbe.BindAddress,
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

	fabricNodeCli, err := fabricnode.NewFabricNodeController(ctx, mgr, log, cfg)
	if err != nil {
		return nil, err
	}

	scraper := rdmascraper.NewRuntimeScraper(fabricNodeCli, logger.WithName(log, "rdma_scraper"), cfg.NodeTopologyDiscovery)
	collector := rdmascraper.NewCollector(scraper, logger.WithName(log, "rdma_collector"))
	metrics.Registry.MustRegister(collector)

	if cfg.EBPFFlow.Enabled {
		if err := addEBPFFlow(mgr, cfg, log); err != nil {
			return nil, err
		}
	}

	return mgr, nil
}

// addEBPFFlow starts the eBPF flow component. Peer resolution needs the IPs
// of Pods on every node while the manager cache only holds this node's Pods,
// so the component gets a second cache restricted to Pods and stripped of
// the fields it does not read.
func addEBPFFlow(mgr ctrl.Manager, cfg *config.AgentConfig, log *slog.Logger) error {
	podCache, err := cache.New(cfg.KubeConfig, cache.Options{
		Scheme: mgr.GetScheme(),
		Mapper: mgr.GetRESTMapper(),
		ByObject: map[client.Object]cache.ByObject{
			&corev1.Pod{}: {Transform: stripPodForFlow},
		},
		ReaderFailOnMissingInformer: true,
	})
	if err != nil {
		return fmt.Errorf("create cluster pod cache: %w", err)
	}
	if _, err := podCache.GetInformer(context.Background(), &corev1.Pod{}); err != nil {
		return fmt.Errorf("register cluster pod informer: %w", err)
	}
	if err := mgr.Add(podCache); err != nil {
		return err
	}
	component, err := ebpfflow.New(cfg.EBPFFlow, cfg.Node.Name, logger.WithName(log, "ebpf_flow"),
		mgr.GetClient(), mgr.GetAPIReader(), podCache, metrics.Registry)
	if err != nil {
		return err
	}
	return mgr.Add(component)
}

// stripPodForFlow keeps only the Pod fields the flow resolver reads:
// identity, owner references, the CNI annotations carrying extra IPs and
// MACs, hostNetwork and the Pod IPs.
func stripPodForFlow(obj any) (any, error) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return obj, nil
	}
	stripped := &corev1.Pod{
		TypeMeta: pod.TypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:            pod.Name,
			Namespace:       pod.Namespace,
			UID:             pod.UID,
			ResourceVersion: pod.ResourceVersion,
			Annotations:     pod.Annotations,
			OwnerReferences: pod.OwnerReferences,
		},
		Spec:   corev1.PodSpec{HostNetwork: pod.Spec.HostNetwork, NodeName: pod.Spec.NodeName},
		Status: corev1.PodStatus{PodIP: pod.Status.PodIP, PodIPs: pod.Status.PodIPs},
	}
	return stripped, nil
}
