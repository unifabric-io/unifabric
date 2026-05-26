// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type SwitchGroupTier int32

const (
	SwitchGroupTierLeaf  SwitchGroupTier = 1
	SwitchGroupTierSpine SwitchGroupTier = 2
	SwitchGroupTierCore  SwitchGroupTier = 3
)

type SwitchGroupSwitchReference struct {
	Name string `json:"name"`
}

type SwitchGroupNodeReference struct {
	Name string `json:"name"`
}

type SwitchGroupSwitchStatus struct {
	SwitchRef SwitchGroupSwitchReference `json:"switchRef"`
}

type SwitchGroupNodeStatus struct {
	FabricNodeRef SwitchGroupNodeReference `json:"fabricNodeRef"`
}

type SwitchGroupStatus struct {
	// +kubebuilder:validation:Enum=ScaleOut;ScaleUp;Storage
	Role SwitchRole `json:"role,omitempty"`

	// +kubebuilder:validation:Enum=1;2;3
	Tier SwitchGroupTier `json:"tier,omitempty"`

	Healthy bool `json:"healthy,omitempty"`

	Switches []SwitchGroupSwitchStatus `json:"switches,omitempty"`

	Nodes []SwitchGroupNodeStatus `json:"nodes,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Role",type="string",JSONPath=".status.role"
// +kubebuilder:printcolumn:name="Tier",type="integer",JSONPath=".status.tier"
// +kubebuilder:printcolumn:name="Healthy",type="boolean",JSONPath=".status.healthy"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:categories={switchgroup},path="switchgroups",singular="switchgroup",scope="Cluster",shortName={sg}
// +kubebuilder:subresource:status
type SwitchGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status SwitchGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type SwitchGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SwitchGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SwitchGroup{}, &SwitchGroupList{})
}
