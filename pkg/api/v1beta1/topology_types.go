// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	TopologyScaleOut = "scaleout"
	TopologyScaleUp  = "scaleup"
	TopologyStorage  = "storage"
)

const TopologyDomainLabel = "unifabric.io/domain"

var FixedTopologyNames = []string{TopologyScaleOut, TopologyScaleUp, TopologyStorage}

type TopologyDomain struct {
	// Name is the value stored in the corresponding Kubernetes topology label.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`

	// Tier starts at one for the domain nearest to a Kubernetes Node.
	// +kubebuilder:validation:Minimum=1
	Tier int32 `json:"tier"`

	// Parent is the directly enclosing performance domain.
	// +optional
	Parent string `json:"parent,omitempty"`

	// Members contains the names of Switch resources carrying this domain.
	// +optional
	// +listType=set
	Members []string `json:"members,omitempty"`
}

type TopologyNodeGroup struct {
	// Nodes contains Kubernetes Nodes that have the same domain path.
	// +listType=set
	// +kubebuilder:validation:MinItems=1
	Nodes []string `json:"nodes"`

	// DomainPath is ordered from the highest tier to the tier nearest to a Node.
	// +kubebuilder:validation:MinItems=1
	DomainPath []string `json:"domainPath"`
}

type TopologyStatus struct {
	// Domains is the topology domain forest aggregated from labels.
	// +optional
	// +listType=map
	// +listMapKey=name
	Domains []TopologyDomain `json:"domains,omitempty"`

	// Nodes groups Kubernetes Nodes by their complete domain path.
	// +optional
	Nodes []TopologyNodeGroup `json:"nodes,omitempty"`
}

// Topology is a read-only, cluster-scoped topology view. It intentionally has
// no spec: Node and Switch labels are its only source of truth.
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=topologies,singular=topology,scope=Cluster,shortName=topo
// +kubebuilder:subresource:status
// +kubebuilder:validation:XValidation:rule="self.metadata.name in ['scaleout', 'scaleup', 'storage']",message="metadata.name must be scaleout, scaleup, or storage"
type Topology struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status TopologyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type TopologyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Topology `json:"items"`
}

func IsFixedTopologyName(name string) bool {
	for _, candidate := range FixedTopologyNames {
		if candidate == name {
			return true
		}
	}
	return false
}

func IsTopologyDomainLabel(label string) bool {
	return label == TopologyDomainLabel
}

func init() {
	SchemeBuilder.Register(&Topology{}, &TopologyList{})
}
