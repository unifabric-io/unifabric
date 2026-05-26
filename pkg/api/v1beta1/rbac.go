// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

// +kubebuilder:rbac:groups="coordination.k8s.io",resources=leases,verbs=create;get;update;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;get;list;watch;patch;update;delete

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get;list;watch

// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;patch;update
// +kubebuilder:rbac:groups="",resources=nodes/status,verbs=get;list;watch

// +kubebuilder:rbac:groups=unifabric.io,resources=fabricnodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=unifabric.io,resources=fabricnodes/status,verbs=get;update;patch

// RBAC for retrieving the top owner of Pods (for fabric node status)
// Common Kubernetes controllers
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=get
// +kubebuilder:rbac:groups="apps",resources=statefulsets,verbs=get
// +kubebuilder:rbac:groups="apps",resources=replicasets,verbs=get
// +kubebuilder:rbac:groups="apps",resources=daemonsets,verbs=get
// +kubebuilder:rbac:groups="batch",resources=jobs,verbs=get
// +kubebuilder:rbac:groups="batch",resources=cronjobs,verbs=get

// Third-party ML/Workflow controllers (optional, for ML/AI workloads)
// +kubebuilder:rbac:groups="kubeflow.org",resources=tfjobs,verbs=get
// +kubebuilder:rbac:groups="kubeflow.org",resources=pytorchjobs,verbs=get
// +kubebuilder:rbac:groups="kubeflow.org",resources=mpijobs,verbs=get
// +kubebuilder:rbac:groups="kubeflow.org",resources=xgboostjobs,verbs=get
// +kubebuilder:rbac:groups="ray.io",resources=rayjobs,verbs=get
// +kubebuilder:rbac:groups="sparkoperator.k8s.io",resources=sparkapplications,verbs=get

package v1beta1
