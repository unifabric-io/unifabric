// Copyright 2026 Authors of unifabric-io
// SPDX-License-Identifier: Apache-2.0

package topologyapi

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultClusterName is the cluster name SingleCluster serves under.
const DefaultClusterName = "default"

// ClusterResolver supplies the clusters this API serves and a client for
// each. Implementations decide what a cluster name means: SingleCluster
// serves one in-process client, while multi-cluster builds resolve names
// through their own management plane.
type ClusterResolver interface {
	GetClusterClient(clusterName string) (client.Client, error)
	ListClusters(ctx context.Context) ([]string, error)
}

// SingleCluster adapts one client into a ClusterResolver that serves it as
// the sole cluster named DefaultClusterName.
func SingleCluster(c client.Client) ClusterResolver {
	return singleCluster{c: c}
}

type singleCluster struct {
	c client.Client
}

func (s singleCluster) GetClusterClient(clusterName string) (client.Client, error) {
	if clusterName != DefaultClusterName {
		return nil, fmt.Errorf("unknown cluster %q", clusterName)
	}
	return s.c, nil
}

func (s singleCluster) ListClusters(context.Context) ([]string, error) {
	return []string{DefaultClusterName}, nil
}
