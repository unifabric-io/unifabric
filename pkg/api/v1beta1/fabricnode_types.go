// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NodeRole string

const (
	NodeRoleGPU     NodeRole = "GPU"
	NodeRoleStorage NodeRole = "Storage"
)

const (
	FabricNodeConditionReady              = "Ready"
	FabricNodeConditionLLDPNeighborsReady = "LLDPNeighborsReady"
)

const (
	FabricNodeReasonReady               = "Ready"
	FabricNodeReasonConditionNotReady   = "ConditionNotReady"
	FabricNodeReasonConditionUnknown    = "ConditionUnknown"
	FabricNodeReasonLLDPNeighborsReady  = "LLDPNeighborsReady"
	FabricNodeReasonLLDPNeighborMissing = "LLDPNeighborMissing"
	FabricNodeReasonDiscoveryFailed     = "DiscoveryFailed"
)

// FabricNodeSpec defines desired state (empty for now)
type FabricNodeSpec struct{}

// RdmaPod represents a Pod that uses RDMA
type RdmaPod struct {
	// Namespace is the namespace of the RDMA Pod
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// Name is the name of the RDMA Pod
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// ContainerList is the list of container IDs in the RDMA Pod
	ContainerList []string `json:"containerList,omitempty"`

	// HostRDMA indicates whether the RDMA Pod is running in host RDMA mode.
	// Host RDMA mode typically requires:
	// - /dev/infiniband to be mounted,
	// - privileged mode enabled,
	// - hostNetwork set to true.
	HostRDMA bool `json:"hostRDMA,omitempty"`

	// TopOwner is the top-level owner of the RDMA Pod
	TopOwner *OwnerRef `json:"topOwner,omitempty"`
}

// OwnerRef represents a reference to the owner of the RDMA Pod
type OwnerRef struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name,omitempty"`
}

// NicInfo represents the network interface card (NIC) information
type NicInfo struct {
	// Name is the name of the NIC, e.g., eth0, ib0, etc.
	Name string `json:"name"`

	// RDMA device name, e.g., mlx5_0
	RdmaDeviceName string `json:"rdmaDeviceName"`

	// RDMA indicates if the NIC is an RDMA-capable interface
	RDMA bool `json:"rdma"`

	// IPv4 is the IPv4 address assigned to the NIC, in CIDR notation, e.g., 172.17.0.1/32.
	IPv4 string `json:"ipv4"`

	// IPv6 is the IPv6 address assigned to the NIC, in CIDR notation, e.g., fe80::1/64.
	IPv6 string `json:"ipv6"`

	// State indicates the current state of the NIC, e.g., "up", "down", "unknown"
	State string `json:"state"`

	// LLDPNeighbor contains information about the neighbor device connected to this NIC
	LLDPNeighbor LLDPNeighbor `json:"lldpNeighbor,omitempty"`
}

// LLDPNeighbor represents a neighboring device connected to the NIC
type LLDPNeighbor struct {
	// Hostname is the hostname of the neighbor device
	Hostname string `json:"hostname"`

	// MgmtIP is the management IP address of the neighbor device
	MgmtIP string `json:"mgmtIP"`

	// Mac is the MAC address of the neighbor device
	Mac string `json:"mac"`

	// Port is the port name on the neighbor device, e.g., Ethernet32
	Port string `json:"port"`

	// Description is the description of the neighbor device
	Description string `json:"description"`
}

// FabricNodeStatus defines the observed state of FabricNode
type FabricNodeStatus struct {
	// Conditions describe the current FabricNode status.
	// +listType=map
	// +listMapKey=type
	Conditions   []metav1.Condition `json:"conditions,omitempty"`
	TotalNics    int                `json:"totalNics"`
	RdmaPods     []RdmaPod          `json:"rdmaPods,omitempty"`
	ScaleOutNics []NicInfo          `json:"scaleOutNics,omitempty"`
	StorageNics  []NicInfo          `json:"storageNics,omitempty"`
	// NodeRole is the role reported by the Agent for this node.
	// +kubebuilder:validation:Enum=GPU;Storage
	NodeRole NodeRole `json:"nodeRole,omitempty"`
	NodeIP   string   `json:"nodeIP,omitempty"`
}

// FabricNode represents a node in the cluster.
// It includes the node's compute and storage NIC information, and the list of
// RDMA-enabled pods running on the node.
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="TotalNics",type=integer,JSONPath=`.status.totalNics`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:resource:categories=fabricnode,path=fabricnodes,singular=fabricnode,scope=Cluster,shortName=fn
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Role",type="string",JSONPath=".status.nodeRole",description="Node role"
// +kubebuilder:printcolumn:name="NodeIP",type="string",JSONPath=".status.nodeIP",description="Node IP"
type FabricNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FabricNodeSpec   `json:"spec,omitempty"`
	Status FabricNodeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type FabricNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FabricNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FabricNode{}, &FabricNodeList{})
}
