// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch;update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=daemonsets;deployments;replicasets;statefulsets,verbs=get
// +kubebuilder:rbac:groups=batch,resources=cronjobs;jobs,verbs=get
// +kubebuilder:rbac:groups=kubeflow.org,resources=mpijobs;pytorchjobs;tfjobs;xgboostjobs,verbs=get
// +kubebuilder:rbac:groups=ray.io,resources=rayjobs,verbs=get
// +kubebuilder:rbac:groups=sparkoperator.k8s.io,resources=sparkapplications,verbs=get
// +kubebuilder:rbac:groups=unifabric.io,resources=fabricnodes,verbs=create;delete;get;list;watch;patch;update
// +kubebuilder:rbac:groups=unifabric.io,resources=fabricnodes/status,verbs=get;patch;update

package main