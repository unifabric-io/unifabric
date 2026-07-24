// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type SwitchRole string

const (
	SwitchRoleScaleOut SwitchRole = "ScaleOut"
	SwitchRoleScaleUp  SwitchRole = "ScaleUp"
	SwitchRoleStorage  SwitchRole = "Storage"
)

const SwitchNeighborsAnnotation = "unifabric.io/neighbors"

const (
	SwitchConditionConnected = "Connected"
	SwitchConditionReady     = "Ready"

	SwitchReasonStreamReady          = "StreamReady"
	SwitchReasonDialFailed           = "DialFailed"
	SwitchReasonAuthenticationFailed = "AuthenticationFailed"
	SwitchReasonSnapshotAccepted     = "SnapshotAccepted"
	SwitchReasonSnapshotRejected     = "SnapshotRejected"
	SwitchReasonDataStale            = "DataStale"
)

type SwitchSpec struct {
	// MgmtIP is the optional controller dial target for this switch. When it is
	// omitted, the Switch can still contribute topology membership and manual
	// adjacency through metadata without starting a switch-agent subscription.
	// +optional
	// +kubebuilder:validation:MinLength=1
	MgmtIP string `json:"mgmtIP,omitempty"`

	// Role classifies this switch by fabric domain.
	// +optional
	// +kubebuilder:default=ScaleOut
	// +kubebuilder:validation:Enum=ScaleOut;ScaleUp;Storage
	Role SwitchRole `json:"role,omitempty"`

	// GrpcPort overrides the default switch-agent gRPC listen port for this switch.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	GrpcPort *int32 `json:"grpcPort,omitempty"`
}

type SwitchLLDPRemoteSystemType string

const (
	SwitchLLDPRemoteSystemTypeKubernetesNode SwitchLLDPRemoteSystemType = "KubernetesNode"
	SwitchLLDPRemoteSystemTypeSwitch         SwitchLLDPRemoteSystemType = "Switch"
)

type SwitchNeighbor struct {
	// RemoteSystemType identifies whether the remote system resolves to a Kubernetes Node or another switch.
	// +optional
	// +kubebuilder:validation:Enum=KubernetesNode;Switch
	RemoteSystemType SwitchLLDPRemoteSystemType `json:"remoteSystemType,omitempty"`
	// RemoteSystemName identifies the remote system for this LLDP entry.
	RemoteSystemName string `json:"remoteSystemName"`
}

type SwitchStatus struct {
	// Hostname is the switch-reported system name from the latest accepted snapshot.
	Hostname string `json:"hostname,omitempty"`

	Healthy bool `json:"healthy,omitempty"`

	// Conditions describe the current switch stream and data freshness state.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LLDPNeighborCount reports the number of stored LLDP neighbor entries.
	LLDPNeighborCount int32 `json:"lldpNeighborCount,omitempty"`

	// LLDPNeighbors stores unique remote neighbors observed for this switch.
	LLDPNeighbors []SwitchNeighbor `json:"lldpNeighbors,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="MgmtIP",type="string",JSONPath=".spec.mgmtIP"
// +kubebuilder:printcolumn:name="Role",type="string",JSONPath=".spec.role"
// +kubebuilder:printcolumn:name="Healthy",type="boolean",JSONPath=".status.healthy"
// +kubebuilder:printcolumn:name="Neighbors",type="integer",JSONPath=".status.lldpNeighborCount"
// +kubebuilder:resource:categories={switch},path="switches",singular="switch",scope="Cluster",shortName={sw}
// +kubebuilder:subresource:status
type Switch struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SwitchSpec   `json:"spec,omitempty"`
	Status SwitchStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type SwitchList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Switch `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Switch{}, &SwitchList{})
}
