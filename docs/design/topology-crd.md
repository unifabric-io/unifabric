# Topology CRD Design

中文版本：[topology-crd.zh.md](./topology-crd.zh.md)

## Summary

`Topology` is the cluster-scoped, read-only topology view of Unifabric. It serves
queries, visualization, diagnostics, and observability. The Controller aggregates
performance domains, their parent relationships, and Node paths from Kubernetes Node
labels into the `status` of three fixed objects: `scaleout`, `scaleup`, and `storage`.
The resource has no `spec`, and users cannot write its status. This document records
the API contract, the label contract, and the reconciliation design. The current
field reference is [Topology reference](../reference/topology.md). How to view the
topology in Grafana is in [Topology visualization usage](../usage/topology-visualization.md).

## Motivation

Unifabric discovers RDMA topology from several sources: built-in LLDP discovery,
NVIDIA Topograph, and administrator-managed labels. Before this design, no single API
exposed the aggregated result. Schedulers read Node labels directly, but operators and
tools had no cluster-level object that showed domains, their hierarchy, and the path
of every Node. Each consumer had to rebuild the tree from raw labels.

A second problem was stability. Discovered domain names must survive device
disconnection, Controller restart, and leader transition. Without explicit locking
rules, a reconnecting switch can receive a new name and break every consumer that
stored the old one.

User stories:

- As a scheduler operator, I want one read-only resource that lists performance
  domains and Node paths, so I can see the tree that the Node labels express.
- As a platform operator, I want domain names to stay stable across restarts and
  outages, so dashboards and placement policies do not churn.
- As a cluster administrator, I want to combine NVIDIA Topograph labels with
  manually created `Switch` resources, so one API covers mixed discovery sources.

### Goals

- Represent scale-out, scale-up, and storage topology through one read-only API.
- Use Node labels as the sole source for `domains` and `nodes`. Use the fixed Switch
  `unifabric.io/domain` label only to enrich `switchMember`.
- Represent the domain hierarchy, the switches in each domain, and the complete domain
  path of each Node.
- Support built-in switch discovery, NVIDIA Topograph, and administrator-managed labels.
- Keep assigned domain names stable across ordinary reconciliation, device
  disconnection, Controller restart, and leader transition.

### Non-Goals

- `Topology` defines no resource-matching criteria.
- `Topology` status never creates or modifies Node, Switch, or FabricNode labels in
  the reverse direction. The explicit reset triggered by deleting a `Topology` is the
  only exception.
- `Topology` does not store raw LLDP, port, link, bandwidth, latency, or routing data.
- The design defines no scheduler plugins, workload placement, or topology constraints.
- Users cannot directly maintain `status.domains` or `status.nodes`.

## Proposal

The design splits the work between two controllers and a label contract.

`AutoDiscoveredTopologyLabelController` watches `Switch` and `FabricNode` resources,
runs `pkg/topology.DiscoverTopology()`, and writes two kinds of labels. Each Node
receives one tier label per level of its path, rendered from a configurable Go
template such as `scale-out.unifabric.io/tier-{{ .Tier }}`. Each Switch receives one
fixed `unifabric.io/domain` label.

`TopologyStatusController` watches those labels, rebuilds the domain tree, and writes
the status of the fixed `Topology` objects. It is the sole writer of `Topology`
status. NVIDIA Topograph and administrators also write Node labels, which this
controller reads the same way. There is therefore no intermediate labels-to-CRD
controller, and no loop from status back to labels.

The data flow is:

```text
Switch + FabricNode -> label controller -> Node tier labels + Switch domain label
NVIDIA Topograph  ----------------------^
administrator     ----------------------^
Node + Switch labels -> status controller -> Topology status -> UI, kubectl, schedulers
```

## Detailed Design

### API sketch

`Topology` is cluster-scoped, has no `spec`, and its core status fields are:

| Field | Meaning |
| --- | --- |
| `status.domains[*].name` | Domain name, valid as a Kubernetes label value. |
| `status.domains[*].tier` | Tier parsed from the label key. Tier 1 is nearest to a Node. |
| `status.domains[*].parent` | Optional direct parent domain. Absent on a root. |
| `status.domains[*].switchMember` | Names of the `Switch` resources in this domain. |
| `status.nodes[*].name` | Stable group identifier in the form `node-group<N>`. |
| `status.nodes[*].nodes` | Node names that share one path, sorted lexically. |
| `status.nodes[*].switchDomainPath` | Domain names ordered from the highest tier down to tier 1. |

```yaml
apiVersion: unifabric.io/v1beta1
kind: Topology
metadata:
  name: scaleout
status:
  domains:
    - name: tier2-group1
      tier: 2
      switchMember:
        - spine1
    - name: tier1-group1
      tier: 1
      parent: tier2-group1
      switchMember:
        - leaf1
        - leaf2
  nodes:
    - name: node-group1
      nodes: [node1, node2]
      switchDomainPath:
        - tier2-group1
        - tier1-group1
```

The structure fixes no depth. Two-tier, three-tier, and deeper fabrics use the same
fields. Built-in discovery names domains `tier${N}-group${M}`, where `M` counts groups
within one topology and tier.

### Label contract

`topoDiscovery` in the Helm values selects one writer and one label-key template per
topology. A template is a Go `text/template` with exactly one `{{ .Tier }}` action and
otherwise fixed text. Startup validation walks the parse tree to enforce this and
renders sample tiers to make sure that the output is a valid label key. The Controller
derives an anchored matcher from the same tree to parse tiers back out of label keys.

Tiers on one Node are contiguous from tier 1. Different Nodes can end at different
highest tiers. A Switch carries only the fixed `unifabric.io/domain` label and selects
its fabric through `spec.role`. The Controller infers the Switch tier by matching the
label value to a Node-derived domain.

### Name stability and locking

Once a Node or Switch carries a managed label, that key and value is a locked
assignment. Ordinary reconciliation only fills missing labels. It never overwrites,
migrates, renames, or removes an existing one. Device power-off, missing LLDP
observations, and input deletion do not clean up labels. The Controller stores no
naming counter. Each reconciliation scans existing labels, finds the maximum group
sequence per tier, and allocates new names from `M+1`.

Deleting a `Topology` owned by built-in discovery is the only full reset. A finalizer
owned by the label controller clears all managed labels, then allows the deletion.
Discovery recomputes, recreates the fixed object when the result is non-empty, and
names groups again from `group1`.

### Single-writer rules

Each topology label key has exactly one writer at a time. Built-in discovery owns the
labels it generates. An external writer, such as a separately deployed NVIDIA
Topograph, owns the configured keys only for a topology with `mode=external`. An
administrator manages a topology only when no automatic writer owns it. A conflict
with another field manager is reported, not forced.

### Controller interactions and upgrade compatibility

The label controller uses input fingerprints as update predicates. The `Switch`
fingerprint covers `spec.role`, `status.hostname`, `status.healthy`, and normalized
LLDP neighbors. The `FabricNode` fingerprint covers `status.nodeRole` and the NIC
fields that topology calculation consumes. Label-only updates do not retrigger
discovery. Labels are written with metadata-only Patches under a resource-version
precondition.

The status controller rebuilds a complete snapshot per reconciliation and replaces
status atomically. On conflicting input, it retains the last valid `domains` and
`nodes`, so consumers never observe a partial result. Identical input produces no API
write.

Fixed `Topology` objects are created lazily, only after a non-empty valid result.
Removing all relevant labels is a valid empty result: the status is cleared, but an
already-created object remains. This keeps upgrades safe. A new Controller version
reads the same labels and reproduces the same status without migration steps.

## Risks and Mitigations

- A label write can enqueue the discovery controller again and amplify API load.
  Mitigation: fingerprint-based update predicates and diff-based writes.
- Partial Patch failures can leave a group half-named. Mitigation: idempotent
  reconciliation that recalculates the maximum sequence and reuses the locked name.
- Conflicting locked names in one discovered group can corrupt the tree. Mitigation:
  report the conflict through Events, logs, and metrics, retain every existing label,
  and write nothing automatically.
- A deleted group frees its name for reuse, which can confuse consumers that stored
  it. Mitigation: documented behavior. Reuse happens only after every carrying object
  is actually deleted.

## Alternatives

- A writable `spec` with resource-matching criteria. Rejected because two writers for
  one topology create conflicts, and because every consumer needs the computed result,
  not the matching rules. The read-only status model keeps one authoritative writer.
- Per-tier label keys on Switches, mirroring the Node tier labels. Rejected because a
  Switch sits at exactly one tier, and the tier is already derivable from the matching
  Node domain. One fixed `unifabric.io/domain` key is simpler and role-independent.
- A persistent naming counter stored in a ConfigMap or in status. Rejected because a
  counter can drift from the actual labels after partial failures. Recomputing the
  maximum sequence from live labels is always consistent with the written state.
- One combined controller for discovery and status aggregation. Rejected because
  NVIDIA Topograph and administrators write labels without discovery. A separate
  status controller consumes all label sources uniformly and prevents write loops.
- Eager creation of the three fixed `Topology` objects at startup. Rejected because an
  empty object misleads consumers and complicates the deletion-as-reset semantics.
  Lazy creation ties object existence to the presence of real topology data.

## Implementation History

- 2026-07-13: Topology CRD design document added.
- 2026-07-17: Topology status aggregation and automatic label discovery implemented.
- 2026-08-11: `Topology` status fields renamed, and neighbor link and count tracking
  added. Earlier revisions of this document used the pre-rename field names.
- 2026-08-24: Grafana topology visualization added on top of the Topology API.
