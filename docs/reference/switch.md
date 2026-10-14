# Switch

中文版：[switch.zh.md](./switch.zh.md)

## Overview

A `Switch` declares one physical switch and holds the switch-agent connection state
and LLDP neighbors that the Controller observes. For its role in topology discovery,
see the [architecture overview](../concepts/architecture.md).

## Resource Information

| Item | Value |
| --- | --- |
| API group | `unifabric.io` |
| API version | `v1beta1` |
| Kind | `Switch` |
| Scope | Cluster |
| Plural | `switches` |
| Singular | `switch` |
| Short name | `sw` |
| Status subresource | Yes |
| Printer columns | `MgmtIP`, `Role`, `Healthy`, `Neighbors` |

## Example YAML

Users write `metadata` and `spec`. The Controller writes `status`.

```yaml
apiVersion: unifabric.io/v1beta1
kind: Switch
metadata:
  name: leaf01
  labels:
    # Performance domain that contains this switch. Discovery writes it,
    # or an administrator sets it manually.
    unifabric.io/domain: tier1-group1
  annotations:
    # Semi-automatic mode only: directly connected switch names,
    # as a JSON string array. Node names are not allowed.
    unifabric.io/switch-neighbors: '["spine01"]'
spec:
  # Dial target for the switch-agent subscription. Omit it to declare
  # the switch without a subscription.
  mgmtIP: 10.0.0.1
  # Fabric domain of this switch.
  role: ScaleOut
  # Per-switch override of the switch-agent gRPC port.
  grpcPort: 8090
status:
  conditions:
    # Connected reports the gRPC stream state.
    - lastTransitionTime: "2026-07-24T01:00:00Z"
      message: controller is receiving LLDP snapshots from switch agent
      reason: StreamReady
      status: "True"
      type: Connected
    # Ready reports the freshness of the stored LLDP data.
    - lastTransitionTime: "2026-07-24T01:00:00Z"
      message: accepted LLDP snapshot generation 7
      reason: SnapshotAccepted
      status: "True"
      type: Ready
  # Hostname from the latest accepted snapshot.
  hostname: leaf01
  healthy: true
  # Unique LLDP neighbors, deduplicated by remote system.
  lldpNeighborCount: 2
  lldpNeighbors:
    - remoteSystemType: KubernetesNode
      remoteSystemName: node1
      linkCount: 1
    - remoteSystemType: Switch
      remoteSystemName: spine01
      linkCount: 2
```

## Field Definitions

### Metadata

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `metadata.name` | string | Yes | none | Switch name. Topology discovery matches it against LLDP system names. |
| `metadata.labels["unifabric.io/domain"]` | label | No | none | Performance domain that contains this switch, for example `tier1-group1`. Discovery writes it, and a manual value must name an existing domain. |
| `metadata.annotations["unifabric.io/switch-neighbors"]` | annotation | No | none | Directly connected switch names in semi-automatic mode, as a JSON string array. Kubernetes Node names are not allowed. |

For switches with the same `role`, the presence of the
`unifabric.io/switch-neighbors` key selects the discovery mode. When every switch has
the key, discovery runs in semi-automatic mode. When any switch lacks the key,
discovery runs in fully automatic mode and ignores every such annotation. An empty
value and `[]` both count as the key being present.

### Spec

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `mgmtIP` | string | No | none | Dial target for the switch-agent subscription. When omitted, the Controller starts no subscription, and the switch still contributes topology membership and manual adjacency through metadata. |
| `role` | string | No | `ScaleOut` | Fabric domain of this switch. Allowed values are `ScaleOut`, `ScaleUp`, and `Storage`. |
| `grpcPort` | integer | No | none | Per-switch override of the switch-agent gRPC port, from `1` to `65535`. When omitted, the Controller uses its configured `defaultGrpcPort` (`8090` in the chart). |

### Status

The Controller maintains `status`. Do not edit it manually. When `spec.mgmtIP` is
removed, the Controller stops the subscription and clears the whole `status`.

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `hostname` | string | No | none | Switch-reported system name from the latest accepted snapshot. |
| `healthy` | boolean | No | none | `true` after an accepted snapshot, `false` after a subscription failure. |
| `conditions` | `[]metav1.Condition` | No | none | Stream and data freshness conditions, unique by `type`. See [Conditions](#conditions). |
| `lldpNeighborCount` | integer | No | none | Number of stored LLDP neighbor entries. |
| `lldpNeighbors` | `[]SwitchNeighbor` | No | none | Unique remote neighbors observed for this switch. See [SwitchNeighbor](#switchneighbor). |

#### SwitchNeighbor

Element type of `lldpNeighbors`.

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `remoteSystemType` | string | No | none | Whether the remote system resolves to a Kubernetes Node or another switch. Allowed values are `KubernetesNode` and `Switch`. |
| `remoteSystemName` | string | Yes | none | Name of the remote system for this LLDP entry. |
| `linkCount` | integer | No | none | Number of physical links (local ports) observed to this remote system. Multiple links to the same remote system are deduplicated into this single entry. |

#### Conditions

The Controller sets two condition types. The tables enumerate every status and reason
with its trigger.

`Connected`:

| Status | Reason | Trigger |
| --- | --- | --- |
| `True` | `StreamReady` | The Controller is receiving LLDP snapshots from switch-agent. |
| `False` | `DialFailed` | The gRPC dial or stream failed for a reason other than the ones below. The message contains the error. |
| `False` | `AuthenticationFailed` | The subscription failed with a gRPC `FailedPrecondition`, `Unauthenticated`, or `PermissionDenied` code, or with a TLS or certificate error. |
| `False` | `SnapshotRejected` | switch-agent rejected the request with a gRPC `InvalidArgument` code. |

`Ready`:

| Status | Reason | Trigger |
| --- | --- | --- |
| `True` | `SnapshotAccepted` | The Controller accepted and stored the latest LLDP snapshot. The message contains the snapshot generation. |
| `False` | `DataStale` | The subscription failed, so the stored LLDP data is no longer fresh. Set together with `Connected=False`. |

The Controller also emits a `Normal` Kubernetes Event with reason `NeighborsChanged`
when the neighbor set changes between two accepted snapshots. A neighbor is identified
by `remoteSystemType` plus `remoteSystemName`, so a `linkCount` change alone does not
trigger the event.

For how to deploy switch-agent and connect a switch, see the
[switch-agent deployment guide](../usage/switch-agent-deployment.md).
