// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

// Command mock-topology-api serves fixture Topology/Switch/FabricNode data
// for local development of the Unifabric Grafana datasource/panel plugins.
// It reuses the real HTTP handler (pkg/topologyapi) on top of fake clients,
// so it doubles as an integration exercise of that package's multi-cluster
// ClusterResolver surface: the contract can never drift from the controller.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"github.com/unifabric-io/unifabric/pkg/topologyapi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// clusterNames is the ordered list of clusters GET /clusters returns. The mock
// exposes multiple clusters so local dev can exercise the "Cluster" dropdown /
// $cluster template variable, even though the OSS backend itself only ever
// has one real cluster ("default").
var clusterNames = []string{"default", "minicube", "minicube-with-outsider-nodes"}

// fixtureClusters implements topologyapi.ClusterResolver over fake clients.
type fixtureClusters map[string]client.Client

func (f fixtureClusters) GetClusterClient(clusterName string) (client.Client, error) {
	c, ok := f[clusterName]
	if !ok {
		return nil, fmt.Errorf("unknown cluster %q", clusterName)
	}
	return c, nil
}

func (f fixtureClusters) ListClusters(context.Context) ([]string, error) {
	return clusterNames, nil
}

func named(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name}
}

func newSwitch(name string, role v1beta1.SwitchRole) client.Object {
	return &v1beta1.Switch{ObjectMeta: named(name), Spec: v1beta1.SwitchSpec{Role: role}}
}

func newFabricNode(name, ip string) client.Object {
	return &v1beta1.FabricNode{
		ObjectMeta: named(name),
		Status:     v1beta1.FabricNodeStatus{NodeRole: v1beta1.NodeRoleGPU, NodeIP: ip},
	}
}

func newMasterNode(name, ip string) client.Object {
	return &v1beta1.FabricNode{
		ObjectMeta: named(name),
		Status:     v1beta1.FabricNodeStatus{NodeIP: ip},
	}
}

// fixtureObjects lists each cluster's Topology/Switch/FabricNode fixtures.
// "default" is a synthetic 2-tier scaleout fabric matching
// docs/getting-started-sonic-roce.zh.md's example topology. "minicube"
// mirrors a real cluster snapshot (4 GPU hosts, a 2-tier scaleout fabric,
// 1 storage switch); that cluster has no scale-up fabric, so its "scaleup"
// entry is a synthetic addition reusing the same 4 hosts so all 3 fixed
// topologies still render locally.
var fixtureObjects = map[string][]client.Object{
	"default": {
		&v1beta1.Topology{ObjectMeta: named(v1beta1.TopologyScaleOut), Status: v1beta1.TopologyStatus{
			Domains: []v1beta1.TopologyDomain{
				{Name: "tier2-group1", Tier: 2, SwitchMember: []string{"spine1", "spine2"}},
				{Name: "tier1-group1", Tier: 1, Parent: "tier2-group1", SwitchMember: []string{"leaf1", "leaf2"}},
				{Name: "tier1-group2", Tier: 1, Parent: "tier2-group1", SwitchMember: []string{"leaf3", "leaf4"}},
			},
			Nodes: []v1beta1.TopologyNodeGroup{
				{Name: "node-group1", Nodes: []string{"node1", "node2"}, SwitchDomainPath: []string{"tier2-group1", "tier1-group1"}},
				{Name: "node-group2", Nodes: []string{"node3", "node4"}, SwitchDomainPath: []string{"tier2-group1", "tier1-group2"}},
			},
		}},
		&v1beta1.Topology{ObjectMeta: named(v1beta1.TopologyScaleUp), Status: v1beta1.TopologyStatus{
			Domains: []v1beta1.TopologyDomain{
				{Name: "nvlink-group1", Tier: 1, SwitchMember: []string{"nvswitch1"}},
			},
			Nodes: []v1beta1.TopologyNodeGroup{
				{Name: "node-group1", Nodes: []string{"node1", "node2", "node3", "node4"}, SwitchDomainPath: []string{"nvlink-group1"}},
			},
		}},
		&v1beta1.Topology{ObjectMeta: named(v1beta1.TopologyStorage), Status: v1beta1.TopologyStatus{
			Domains: []v1beta1.TopologyDomain{
				{Name: "storage-leaf1", Tier: 1, SwitchMember: []string{"storage-switch1", "storage-switch2"}},
			},
			Nodes: []v1beta1.TopologyNodeGroup{
				{Name: "node-group1", Nodes: []string{"node1", "node2", "node3", "node4"}, SwitchDomainPath: []string{"storage-leaf1"}},
			},
		}},
		newSwitch("spine1", v1beta1.SwitchRoleScaleOut),
		newSwitch("spine2", v1beta1.SwitchRoleScaleOut),
		newSwitch("leaf1", v1beta1.SwitchRoleScaleOut),
		newSwitch("leaf2", v1beta1.SwitchRoleScaleOut),
		newSwitch("leaf3", v1beta1.SwitchRoleScaleOut),
		newSwitch("leaf4", v1beta1.SwitchRoleScaleOut),
		newSwitch("nvswitch1", v1beta1.SwitchRoleScaleUp),
		newSwitch("storage-switch1", v1beta1.SwitchRoleStorage),
		newSwitch("storage-switch2", v1beta1.SwitchRoleStorage),
		newFabricNode("node1", "192.0.2.1"),
		newFabricNode("node2", "192.0.2.2"),
		newFabricNode("node3", "192.0.2.3"),
		newFabricNode("node4", "192.0.2.4"),
	},
	"minicube": {
		&v1beta1.Topology{ObjectMeta: named(v1beta1.TopologyScaleOut), Status: v1beta1.TopologyStatus{
			Domains: []v1beta1.TopologyDomain{
				{Name: "tier2-group1", Tier: 2, SwitchMember: []string{"spine01", "spine02"}},
				{Name: "tier1-group1", Tier: 1, Parent: "tier2-group1", SwitchMember: []string{"leaf01", "leaf02", "leaf03", "leaf04"}},
			},
			Nodes: []v1beta1.TopologyNodeGroup{
				{Name: "node-group1", Nodes: []string{"jx-cube-g09-01", "jx-cube-g09-02", "jx-cube-g09-03", "jx-cube-g09-04"}, SwitchDomainPath: []string{"tier2-group1", "tier1-group1"}},
			},
		}},
		&v1beta1.Topology{ObjectMeta: named(v1beta1.TopologyScaleUp), Status: v1beta1.TopologyStatus{
			Domains: []v1beta1.TopologyDomain{
				{Name: "nvlink-group1", Tier: 1, SwitchMember: []string{"nvsw01"}},
			},
			Nodes: []v1beta1.TopologyNodeGroup{
				{Name: "node-group1", Nodes: []string{"jx-cube-g09-01", "jx-cube-g09-02", "jx-cube-g09-03", "jx-cube-g09-04"}, SwitchDomainPath: []string{"nvlink-group1"}},
			},
		}},
		&v1beta1.Topology{ObjectMeta: named(v1beta1.TopologyStorage), Status: v1beta1.TopologyStatus{
			Domains: []v1beta1.TopologyDomain{
				{Name: "tier1-group1", Tier: 1, SwitchMember: []string{"storagesw"}},
			},
			Nodes: []v1beta1.TopologyNodeGroup{
				{Name: "node-group1", Nodes: []string{"jx-cube-g09-01", "jx-cube-g09-02", "jx-cube-g09-03", "jx-cube-g09-04"}, SwitchDomainPath: []string{"tier1-group1"}},
			},
		}},
		newSwitch("spine01", v1beta1.SwitchRoleScaleOut),
		newSwitch("spine02", v1beta1.SwitchRoleScaleOut),
		newSwitch("leaf01", v1beta1.SwitchRoleScaleOut),
		newSwitch("leaf02", v1beta1.SwitchRoleScaleOut),
		newSwitch("leaf03", v1beta1.SwitchRoleScaleOut),
		newSwitch("leaf04", v1beta1.SwitchRoleScaleOut),
		newSwitch("nvsw01", v1beta1.SwitchRoleScaleUp),
		newSwitch("storagesw", v1beta1.SwitchRoleStorage),
		newFabricNode("jx-cube-g09-01", "10.103.1.13"),
		newFabricNode("jx-cube-g09-02", "10.103.1.14"),
		newFabricNode("jx-cube-g09-03", "10.103.1.15"),
		newFabricNode("jx-cube-g09-04", "10.103.1.16"),
	},
}

func addMinicubeWithOutsiderNodesFixture() {
	objects := make([]client.Object, 0, len(fixtureObjects["minicube"])+3)
	for _, object := range fixtureObjects["minicube"] {
		objects = append(objects, object.DeepCopyObject().(client.Object))
	}
	fixtureObjects["minicube-with-outsider-nodes"] = append(objects,
		newMasterNode("master-01", "10.103.1.10"),
		newMasterNode("master-02", "10.103.1.11"),
		newMasterNode("master-03", "10.103.1.12"),
	)
}

func main() {
	scheme := runtime.NewScheme()
	if err := v1beta1.AddToScheme(scheme); err != nil {
		log.Fatalf("add API types to scheme: %v", err)
	}

	addMinicubeWithOutsiderNodesFixture()

	resolver := fixtureClusters{}
	for name, objects := range fixtureObjects {
		resolver[name] = fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	}

	handler := topologyapi.NewHandler(resolver, slog.Default())

	const addr = ":8080"
	log.Printf("mock-topology-api listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
