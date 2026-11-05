// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package kube

type SwitchList struct {
	Items []Switch `json:"items"`
}

type Switch struct {
	Metadata struct {
		Name        string            `json:"name"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata"`
	Spec struct {
		MgmtIP string `json:"mgmtIP"`
		Role   string `json:"role"`
	} `json:"spec"`
	Status struct {
		Hostname          string `json:"hostname"`
		Healthy           bool   `json:"healthy"`
		LLDPNeighborCount int32  `json:"lldpNeighborCount"`
	} `json:"status"`
}

type Topology struct {
	Metadata struct {
		Name       string   `json:"name"`
		UID        string   `json:"uid"`
		Finalizers []string `json:"finalizers"`
	} `json:"metadata"`
	Status TopologyStatus `json:"status"`
}

type TopologyStatus struct {
	Domains []TopologyDomain    `json:"domains"`
	Nodes   []TopologyNodeGroup `json:"nodes"`
}

type TopologyDomain struct {
	Name         string   `json:"name"`
	Tier         int      `json:"tier"`
	Parent       string   `json:"parent"`
	SwitchMember []string `json:"switchMember"`
}

type TopologyNodeGroup struct {
	Nodes            []string `json:"nodes"`
	SwitchDomainPath []string `json:"switchDomainPath"`
}

type NodeList struct {
	Items []Node `json:"items"`
}

type Node struct {
	Metadata struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
}
