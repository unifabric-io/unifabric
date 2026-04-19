// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package rdmascraper

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRuntimeScraperAttributesHostRDMAPod(t *testing.T) {
	root := t.TempDir()
	paths := createFakeHostSysfs(t, root)

	node := &v1beta1.FabricNode{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status: v1beta1.FabricNodeStatus{
			RdmaPods: []v1beta1.RdmaPod{
				{
					Namespace: "workloads",
					Name:      "rdma-app",
					HostRDMA:  true,
					TopOwner: &v1beta1.OwnerRef{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Namespace:  "workloads",
						Name:       "trainer",
					},
				},
			},
		},
	}
	scraper := NewRuntimeScraper(fakeFabricNodeClient{node: node}, discardLogger(), config.NodeTopologyDiscoveryConfig{})
	scraper.paths = paths

	snapshot, err := scraper.Scrape(context.Background())
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	podSample := findPodSample(snapshot, "port_rcv_data", MetricSourceHWCounters, "workloads", "rdma-app")
	if podSample == nil {
		t.Fatalf("pod-attributed port_rcv_data sample not found in %#v", snapshot.Samples)
	}
	if podSample.Scope != MetricScopePod {
		t.Fatalf("pod sample scope = %q, want %q", podSample.Scope, MetricScopePod)
	}
	if podSample.Workload.HostRDMA != true {
		t.Fatalf("pod sample host RDMA = %v, want true", podSample.Workload.HostRDMA)
	}
	if podSample.Workload.TopOwner.Kind != "Deployment" || podSample.Workload.TopOwner.Name != "trainer" {
		t.Fatalf("top owner = %#v, want Deployment/trainer", podSample.Workload.TopOwner)
	}
	if podSample.Value != 1024 {
		t.Fatalf("pod sample value = %v, want 1024", podSample.Value)
	}
	if findPodSample(snapshot, "rdma_device_tos", MetricSourceDevice, "workloads", "rdma-app") != nil {
		t.Fatal("host RDMA pod should not duplicate rdma_device_tos sample")
	}
}

func TestCollectPodSysfsAddsWorkloadSamples(t *testing.T) {
	root := t.TempDir()
	paths := createFakeHostSysfs(t, root)

	pod := v1beta1.RdmaPod{
		Namespace: "workloads",
		Name:      "sriov-rdma-app",
		TopOwner: &v1beta1.OwnerRef{
			APIVersion: "batch/v1",
			Kind:       "Job",
			Namespace:  "workloads",
			Name:       "worker",
		},
	}
	scraper := NewRuntimeScraper(fakeFabricNodeClient{}, discardLogger(), config.NodeTopologyDiscoveryConfig{})
	scraper.paths = paths

	var snapshot ScrapeSnapshot
	ifNameToDevice, err := scraper.collectPodSysfs(&snapshot, pod, workloadLabelsForPod(pod), nil)
	if err != nil {
		t.Fatalf("collect pod sysfs: %v", err)
	}
	if ifNameToDevice["eth1"] != "rxe_eth1" {
		t.Fatalf("ifNameToDevice = %#v, want eth1 -> rxe_eth1", ifNameToDevice)
	}

	podSample := findPodSample(snapshot, "port_rcv_data", MetricSourceHWCounters, "workloads", "sriov-rdma-app")
	if podSample == nil {
		t.Fatalf("pod-attributed port_rcv_data sample not found in %#v", snapshot.Samples)
	}
	if podSample.Scope != MetricScopePod {
		t.Fatalf("pod sample scope = %q, want %q", podSample.Scope, MetricScopePod)
	}
	if podSample.Workload.HostRDMA {
		t.Fatal("pod sample host RDMA = true, want false")
	}
	if podSample.Workload.TopOwner.Kind != "Job" || podSample.Workload.TopOwner.Name != "worker" {
		t.Fatalf("top owner = %#v, want Job/worker", podSample.Workload.TopOwner)
	}
	if podSample.Value != 1024 {
		t.Fatalf("pod sample value = %v, want 1024", podSample.Value)
	}
}

func TestContainerInitPID(t *testing.T) {
	root := t.TempDir()
	scraper := NewRuntimeScraper(fakeFabricNodeClient{}, discardLogger(), config.NodeTopologyDiscoveryConfig{})
	scraper.paths.containerdTaskPath = filepath.Join(root, "containerd")

	mkdirAll(t, filepath.Join(scraper.paths.containerdTaskPath, "container-1"))
	writeFile(t, filepath.Join(scraper.paths.containerdTaskPath, "container-1", "init.pid"), "12345\n")

	pid, err := scraper.containerInitPID("containerd://container-1")
	if err != nil {
		t.Fatalf("container init pid: %v", err)
	}
	if pid != 12345 {
		t.Fatalf("pid = %d, want 12345", pid)
	}
	if _, err := scraper.containerInitPID("docker://container-1"); err == nil {
		t.Fatal("container init pid error = nil, want unsupported runtime error")
	}
}

func findPodSample(snapshot ScrapeSnapshot, name string, source MetricSource, namespace, pod string) *MetricSample {
	for i := range snapshot.Samples {
		sample := &snapshot.Samples[i]
		if sample.Name == name &&
			sample.Source == source &&
			sample.Workload.PodNamespace == namespace &&
			sample.Workload.PodName == pod {
			return sample
		}
	}
	return nil
}
