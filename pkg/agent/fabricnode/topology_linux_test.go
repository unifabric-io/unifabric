// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package fabricnode

import (
	"errors"
	"testing"

	"github.com/unifabric-io/unifabric/pkg/api/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClassifyTopologyNICSelectorOrder(t *testing.T) {
	tests := []struct {
		name     string
		ifname   string
		storage  string
		scaleUp  string
		scaleOut string
		want     topologyNICClass
	}{
		{
			name:     "storage wins over scale up and scale out",
			ifname:   "eth9",
			storage:  "interface=eth9",
			scaleUp:  "interface=eth9",
			scaleOut: "interface=eth*",
			want:     topologyNICClassStorage,
		},
		{
			name:     "scale up is excluded from FabricNode status",
			ifname:   "eth1",
			scaleUp:  "interface=eth1",
			scaleOut: "interface=eth*",
			want:     topologyNICClassNone,
		},
		{
			name:    "default scale out after filters",
			ifname:  "eth2",
			storage: "interface=eth9",
			scaleUp: "interface=eth8",
			want:    topologyNICClassScaleOut,
		},
		{
			name:     "configured scale out must match",
			ifname:   "eth2",
			scaleOut: "interface=eth1",
			want:     topologyNICClassNone,
		},
		{
			name:     "explicit scale out match",
			ifname:   "eth2",
			scaleOut: "interface=eth*",
			want:     topologyNICClassScaleOut,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reconciler := &fabricNodeReconciler{
				storageRdmaPattern:  mustRdmaInterfacePattern(t, tt.storage),
				scaleUpRdmaPattern:  mustRdmaInterfacePattern(t, tt.scaleUp),
				scaleOutRdmaPattern: mustRdmaInterfacePattern(t, tt.scaleOut),
			}
			got := reconciler.classifyTopologyNIC(v1beta1.NicInfo{Name: tt.ifname}, nil)
			if got != tt.want {
				t.Fatalf("classifyTopologyNIC() = %q, want %q", got, tt.want)
			}
		})
	}
}

func mustRdmaInterfacePattern(t *testing.T, selector string) RdmaInterfaceMethod {
	t.Helper()
	var method RdmaInterfaceMethod
	if err := method.CheckOrParsePattern(selector); err != nil {
		t.Fatalf("parse selector %q: %v", selector, err)
	}
	return method
}

func TestSetLLDPNeighborsReadyConditionSkipsInfiniBandNics(t *testing.T) {
	status := &v1beta1.FabricNodeStatus{}
	nics := []v1beta1.NicInfo{
		{
			Name:  "enp83s0f0np0",
			State: "up",
			LLDPNeighbor: v1beta1.LLDPNeighbor{
				Hostname: "leaf-1",
			},
		},
		{
			Name:  "ibp25s0",
			State: "up",
		},
	}
	lldpOptionalInterfaces := map[string]struct{}{
		"ibp25s0": {},
	}

	setLLDPNeighborsReadyCondition(status, len(nics), nics, lldpOptionalInterfaces, nil)

	condition := meta.FindStatusCondition(status.Conditions, v1beta1.FabricNodeConditionLLDPNeighborsReady)
	if condition == nil {
		t.Fatal("LLDPNeighborsReady condition was not set")
	}
	if condition.Status != metav1.ConditionTrue {
		t.Fatalf("LLDPNeighborsReady status = %s, want %s", condition.Status, metav1.ConditionTrue)
	}
}

func TestSetLLDPNeighborsReadyConditionRequiresEthernetNeighbors(t *testing.T) {
	status := &v1beta1.FabricNodeStatus{}
	nics := []v1beta1.NicInfo{
		{
			Name:  "enp83s0f0np0",
			State: "up",
		},
		{
			Name:  "ibp25s0",
			State: "up",
		},
	}
	lldpOptionalInterfaces := map[string]struct{}{
		"ibp25s0": {},
	}

	setLLDPNeighborsReadyCondition(status, len(nics), nics, lldpOptionalInterfaces, nil)

	condition := meta.FindStatusCondition(status.Conditions, v1beta1.FabricNodeConditionLLDPNeighborsReady)
	if condition == nil {
		t.Fatal("LLDPNeighborsReady condition was not set")
	}
	if condition.Status != metav1.ConditionFalse {
		t.Fatalf("LLDPNeighborsReady status = %s, want %s", condition.Status, metav1.ConditionFalse)
	}
	want := "Selected RDMA interfaces are missing LLDP neighbors: enp83s0f0np0"
	if condition.Message != want {
		t.Fatalf("LLDPNeighborsReady message = %q, want %q", condition.Message, want)
	}
}

func TestSetLLDPNeighborsReadyConditionIgnoresLLDPErrorForInfiniBandOnly(t *testing.T) {
	status := &v1beta1.FabricNodeStatus{}
	nics := []v1beta1.NicInfo{
		{
			Name:  "ibp25s0",
			State: "up",
		},
	}
	lldpOptionalInterfaces := map[string]struct{}{
		"ibp25s0": {},
	}

	setLLDPNeighborsReadyCondition(status, len(nics), nics, lldpOptionalInterfaces, errors.New("lldp unavailable"))

	condition := meta.FindStatusCondition(status.Conditions, v1beta1.FabricNodeConditionLLDPNeighborsReady)
	if condition == nil {
		t.Fatal("LLDPNeighborsReady condition was not set")
	}
	if condition.Status != metav1.ConditionTrue {
		t.Fatalf("LLDPNeighborsReady status = %s, want %s", condition.Status, metav1.ConditionTrue)
	}
}
