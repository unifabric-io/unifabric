// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

// Permissions on Kubernetes built-in resources and on the unifabric.io API.
// Read access to third-party workload kinds used for top-level owner
// resolution is maintained by hand in chart/values.yaml under
// agent.workloadOwnerRules and rendered by
// chart/templates/AgentWorkloadOwnersRole.yaml.

// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch;update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=daemonsets;deployments;replicasets;statefulsets,verbs=get
// +kubebuilder:rbac:groups=batch,resources=cronjobs;jobs,verbs=get
// +kubebuilder:rbac:groups=unifabric.io,resources=fabricnodes,verbs=create;delete;get;list;watch;patch;update
// +kubebuilder:rbac:groups=unifabric.io,resources=fabricnodes/status,verbs=get;patch;update
// +kubebuilder:rbac:groups=unifabric.io,resources=rdmaendpoints,verbs=create;delete;get;list;watch;patch
// +kubebuilder:rbac:groups=unifabric.io,resources=rdmaendpoints/status,verbs=patch

package main
