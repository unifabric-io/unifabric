# Topology

中文版：[topology.zh.md](./topology.zh.md)

## Overview

A `Topology` is a read-only, cluster-scoped topology view that the Controller
aggregates from Node and Switch labels. For its role in topology discovery, see the
[architecture overview](../concepts/architecture.md).

## Resource Information

| Item | Value |
| --- | --- |
| API group | `unifabric.io` |
| API version | `v1beta1` |
| Kind | `Topology` |
| Scope | Cluster |
| Plural | `topologies` |
| Singular | `topology` |
| Short name | `topo` |
| Status subresource | Yes |

## Example YAML

The Controller generates objects like this one. Users do not write them.

```yaml
apiVersion: unifabric.io/v1beta1
kind: Topology
metadata:
  # The name must be scaleout, scaleup, or storage.
  name: scaleout
  # In built-in discovery mode, this finalizer makes deletion an
  # explicit reset of the managed topology labels.
  finalizers:
    - unifabric.io/auto-discovered-topology-labels
status:
  # Performance domains aggregated from labels, unique by name.
  domains:
    # A root domain has no parent.
    - name: tier2-group1
      tier: 2
      switchMember:
        - spine01
    - name: tier1-group1
      tier: 1
      parent: tier2-group1
      switchMember:
        - leaf01
        - leaf02
  # Kubernetes Nodes grouped by their complete domain path.
  nodes:
    - name: node-group1
      nodes:
        - node1
        - node2
      # Ordered from the highest tier to the tier nearest to a Node.
      switchDomainPath:
        - tier2-group1
        - tier1-group1
```

This example means that `leaf01` and `leaf02` are in `tier1-group1`, `spine01` is in
the parent domain `tier2-group1`, and `node1` and `node2` share the same scale-out
domain path.

## Field Definitions

### Metadata

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `metadata.name` | string | Yes | Must be `scaleout`, `scaleup`, or `storage`. A CEL validation rule rejects every other name. |
| `metadata.finalizers` | `[]string` | No | In built-in discovery mode, the Controller adds `unifabric.io/auto-discovered-topology-labels`. Deletion then clears the managed Node and Switch topology labels for that network before the object is removed. |

The three fixed names map to the three networks:

| Name | Network |
| --- | --- |
| `scaleout` | Scale-out network topology |
| `scaleup` | Scale-up network topology |
| `storage` | Storage network topology |

### Spec

`Topology` has no `spec`. Node and Switch labels are its only source of truth.

### Status

The Controller maintains `status`. Do not edit it manually. Every field is empty by
default, so the tables omit the Default column.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `domains` | `[]TopologyDomain` | No | Topology domain forest aggregated from labels, unique by `name`. See [TopologyDomain](#topologydomain). |
| `nodes` | `[]TopologyNodeGroup` | No | Kubernetes Nodes grouped by their complete domain path. See [TopologyNodeGroup](#topologynodegroup). |

#### TopologyDomain

Element type of `domains`.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `name` | string | Yes | Value stored in the corresponding Kubernetes topology label, from 1 to 63 characters. |
| `tier` | integer | Yes | Domain tier, minimum `1`. Tier 1 is the domain nearest to a Kubernetes Node. |
| `parent` | string | No | Name of the directly enclosing performance domain. A root domain has no `parent`. |
| `switchMember` | `[]string` (set) | No | Names of `Switch` resources that carry this domain. Empty when the domain has no Switch CRs, for example in Node-only mode. |

#### TopologyNodeGroup

Element type of `nodes`.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `name` | string | Yes | Stable identifier of this group, in the form `node-group<N>`. An existing group keeps its name across reconciles, and a new group receives the smallest unused number. |
| `nodes` | `[]string` (set) | Yes | Kubernetes Nodes that have the same domain path, at least one item. |
| `switchDomainPath` | `[]string` | Yes | Domain names ordered from the highest tier to the tier nearest to a Node, at least one item. |

The resource defines no conditions.

The Controller creates a `Topology` only after non-empty topology data exists for that
network, so `kubectl get topo` can return nothing on a cluster without topology input.

For how to view and export the topology, see the
[topology visualization guide](../usage/topology-visualization.md).
