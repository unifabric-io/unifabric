// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ScaleoutNeighborGroupSpec defines the desired state of ScaleoutNeighborGroup(empty for now)
type ScaleOutLeafGroupSpec struct {
}

// ScaleoutGroupStatus defines the observed state of ScaleoutGroup, If different
// nodes share the same set of switch neighbors, then these nodes and switches belong to the same ScaleoutGroup.
type ScaleOutLeafGroupStatus struct {
	HealthyNodes int      `json:"healthyNodes"`
	TotalNodes   int      `json:"totalNodes"`
	Healthy      bool     `json:"healthy"`
	Nodes        []Node   `json:"nodes"`
	Switches     []Switch `json:"switches"`
}

type Node struct {
	Name    string `json:"name"`
	Healthy bool   `json:"healthy"`
}

type Switch struct {
	Name   string `json:"name"`
	MgmtIP string `json:"mgmtIP"`
}

// ScaleOutGroupSpec defines the desired state of ScaleoutGroup
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="healthyNodes",type="integer",JSONPath=`.status.healthyNodes`
// +kubebuilder:printcolumn:name="totalNodes",type="integer",JSONPath=`.status.totalNodes`
// +kubebuilder:printcolumn:name="Healthy",type="boolean",JSONPath=`.status.healthy`
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:categories={scaleoutleafgroup},path="scaleoutleafgroups",singular="scaleoutleafgroup",scope="Cluster",shortName={slg}
// +kubebuilder:subresource:status
type ScaleOutLeafGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScaleOutLeafGroupSpec   `json:"spec,omitempty"`
	Status ScaleOutLeafGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ScaleOutLeafGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:",inline"`
	Items           []ScaleOutLeafGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ScaleOutLeafGroup{}, &ScaleOutLeafGroupList{})
}
