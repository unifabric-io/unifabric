// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LinkLayer is the link layer of an RDMA port as reported by sysfs.
type LinkLayer string

const (
	LinkLayerInfiniBand LinkLayer = "InfiniBand"
	LinkLayerEthernet   LinkLayer = "Ethernet"
)

// RDMAEndpointLabelNode marks the node whose agent publishes the object,
// so a restarted agent can adopt the objects left by its previous instance.
const RDMAEndpointLabelNode = "unifabric.io/node"

// RDMAGid is one entry of a port GID table. Zero GIDs and prefix only
// entries such as fe80:: are not listed.
type RDMAGid struct {
	// Index is the position in the sysfs GID table.
	Index int `json:"index"`

	// Gid is the GID in the colon separated sysfs form,
	// e.g. 0000:0000:0000:0000:0000:ffff:ac11:0367.
	Gid string `json:"gid"`
}

// RDMAQueuePair is a QP that reached RTR on a port. QPNs are unique per HCA
// port, so a sender matches its dest_qp_num here to pick the receiving Pod
// when several Pods share a GID or LID.
type RDMAQueuePair struct {
	// Qpn is the queue pair number.
	Qpn uint32 `json:"qpn"`

	// Pid is the host tgid of the process that created the QP.
	Pid uint32 `json:"pid"`
}

// RDMAPort is one port of an RDMA device.
type RDMAPort struct {
	// Port is the 1-based port number.
	Port int `json:"port"`

	// LinkLayer is InfiniBand or Ethernet.
	// +kubebuilder:validation:Enum=InfiniBand;Ethernet
	LinkLayer LinkLayer `json:"linkLayer"`

	// Lid is the LID assigned by the subnet manager, 0 on Ethernet ports and
	// on InfiniBand ports without an active SM.
	Lid uint16 `json:"lid"`

	// SubnetPrefix is the upper 64 bits of GID index 0 as 16 hex digits.
	// LIDs are only unique inside a subnet, so a LID must always be read
	// together with this field.
	SubnetPrefix string `json:"subnetPrefix,omitempty"`

	// Gids lists the non zero GIDs of the port.
	Gids []RDMAGid `json:"gids,omitempty"`

	// QueuePairs lists the QPs of this Pod that reached RTR on the port.
	QueuePairs []RDMAQueuePair `json:"queuePairs,omitempty"`
}

// RDMAInterface is one RDMA device held open by the Pod.
type RDMAInterface struct {
	// Name is the netdev bound to the device, e.g. ib0 or eth1, empty when
	// the device has no netdev visible from the Pod.
	Name string `json:"name,omitempty"`

	// RdmaDevice is the ibdev name, e.g. mlx5_0.
	RdmaDevice string `json:"rdmaDevice"`

	// Ports lists the ports of the device.
	Ports []RDMAPort `json:"ports,omitempty"`
}

// RDMAEndpointStatus is what the node agent observed for the Pod.
type RDMAEndpointStatus struct {
	// NodeName is the node running the Pod and publishing this object.
	NodeName string `json:"nodeName"`

	// HostNetwork reports whether the Pod shares the node network namespace.
	// Its RDMA addresses then belong to the node NIC and can only be
	// attributed through this object.
	HostNetwork bool `json:"hostNetwork"`

	// LastUpdateTime is when the agent last observed the Pod. Readers treat
	// objects not refreshed for 15 minutes as stale.
	LastUpdateTime metav1.Time `json:"lastUpdateTime"`

	// Interfaces lists the RDMA devices the Pod holds open.
	Interfaces []RDMAInterface `json:"interfaces,omitempty"`
}

// RDMAEndpoint records the RDMA devices, addresses and queue pairs of one
// Pod. It is named after the Pod and owned by it so it is garbage collected
// with the Pod. It has no spec because every field is observed.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=rdmaendpoints,singular=rdmaendpoint,scope=Namespaced,shortName=rep
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.nodeName`
// +kubebuilder:printcolumn:name="HostNetwork",type=boolean,JSONPath=`.status.hostNetwork`
// +kubebuilder:printcolumn:name="Updated",type=date,JSONPath=`.status.lastUpdateTime`
type RDMAEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status RDMAEndpointStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type RDMAEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RDMAEndpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RDMAEndpoint{}, &RDMAEndpointList{})
}
