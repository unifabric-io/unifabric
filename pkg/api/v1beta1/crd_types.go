// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// +kubebuilder:object:generate=true
// +groupName=unifabric.io

var (
	// SchemeBuilder is used to register the types with the scheme
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	// AddToScheme adds the types in this group-version to the given scheme
	AddToScheme = SchemeBuilder.AddToScheme
)

// GroupVersion is group version used to register these objects
var GroupVersion = schema.GroupVersion{Group: "unifabric.io", Version: "v1beta1"}
