// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch;update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups="coordination.k8s.io",resources=leases,verbs=create;get;list;watch;update;patch
// +kubebuilder:rbac:groups=unifabric.io,resources=fabricnodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=unifabric.io,resources=fabricnodes/status,verbs=get;patch;update
// +kubebuilder:rbac:groups=unifabric.io,resources=switches,verbs=create;delete;get;list;watch;patch;update
// +kubebuilder:rbac:groups=unifabric.io,resources=switches/status,verbs=get;patch;update
// +kubebuilder:rbac:groups=unifabric.io,resources=topologies,verbs=create;get;list;watch;patch;update
// +kubebuilder:rbac:groups=unifabric.io,resources=topologies/status,verbs=get;patch;update

package main