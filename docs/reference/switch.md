# Switch

中文版：[switch.zh.md](./switch.zh.md)

A `Switch` represents a physical switch. The user declares its `metadata` and
`spec`. After a switch-agent connection is configured, the Controller writes
connection state and LLDP snapshots to `status`. switch-agent does not access
the Kubernetes API directly.

## Examples

### Fully automatic LLDP discovery

In this mode, each Switch CR represents a real switch and provides its
switch-agent address:

```yaml
apiVersion: unifabric.io/v1beta1
kind: Switch
metadata:
  name: leaf01
spec:
  mgmtIP: 10.0.0.1
  role: ScaleOut
  grpcPort: 8090
```

### Semi-automatic discovery

When Node LLDP has discovered `leaf01` and `leaf02`, create a CR only for the
upstream switch:

```yaml
apiVersion: unifabric.io/v1beta1
kind: Switch
metadata:
  name: spine01
  annotations:
    unifabric.io/neighbors: '["leaf01", "leaf02"]'
spec:
  role: ScaleOut
```

## Resource information

| Field | Value |
| --- | --- |
| API group | `unifabric.io` |
| API version | `v1beta1` |
| Kind | `Switch` |
| Scope | Cluster |
| Resource | `switches` |
| Singular name | `switch` |
| Short name | `sw` |
| Status subresource | Yes |

## Switch fields

| Field | Type | Description |
| --- | --- | --- |
| `apiVersion` | string | `unifabric.io/v1beta1` |
| `kind` | string | `Switch` |
| `metadata` | `metav1.ObjectMeta` | Standard Kubernetes metadata with Unifabric labels and annotations |
| `spec` | `SwitchSpec` | User-declared switch connection and fabric role |
| `status` | `SwitchStatus` | Read-only observed state maintained by the Controller |

## SwitchSpec

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `mgmtIP` | string | No | None | Management address used by the Controller to connect to switch-agent; no subscription is started when omitted |
| `role` | string | No | `ScaleOut` | Fabric role: `ScaleOut`, `ScaleUp`, or `Storage` |
| `grpcPort` | int32 | No | Global port | Overrides the switch-agent gRPC port for this switch; valid range is `1`–`65535` |

When `grpcPort` is omitted, `switchSubscription.defaultGrpcPort` is used. Its
chart default is `8090`.

A Switch without `mgmtIP` can still declare semi-automatic adjacency or
populate `Topology.status.domains[*].members` through metadata, but the
Controller does not connect to switch-agent.

## SwitchStatus

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `hostname` | string | No | Hostname reported by the latest switch-agent snapshot |
| `healthy` | boolean | No | Whether the current subscription and LLDP data are healthy |
| `conditions` | `[]metav1.Condition` | No | Connection and data freshness conditions, unique by `type` |
| `lldpNeighborCount` | int32 | No | Number of unique stored LLDP neighbors |
| `lldpNeighbors` | `[]SwitchNeighbor` | No | LLDP neighbors in the latest accepted snapshot |

Known condition types and reasons:

| Category | Values |
| --- | --- |
| Type | `Connected`, `Ready` |
| Healthy reasons | `StreamReady`, `SnapshotAccepted` |
| Failure reasons | `DialFailed`, `AuthenticationFailed`, `SnapshotRejected`, `DataStale` |

## SwitchNeighbor

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `remoteSystemType` | string | No | Neighbor type: `KubernetesNode` or `Switch` |
| `remoteSystemName` | string | Yes | Neighbor Node or switch name |

## Labels and annotations

| Key | Type | Description |
| --- | --- | --- |
| `unifabric.io/domain` | Label | Performance domain containing the Switch, for example `tier1-group1` |
| `unifabric.io/neighbors` | Annotation | Directly connected switch names in semi-automatic mode, encoded as a JSON string array |

Built-in discovery can write `unifabric.io/domain`. An administrator may set
the label manually to populate `Topology.status.domains[*].members`, but its
value must match an existing performance domain.

Example `unifabric.io/neighbors` value:

```yaml
metadata:
  annotations:
    unifabric.io/neighbors: '["leaf01", "leaf02"]'
```

For Switches with the same `role`, topology mode is selected by the presence of
the annotation **key**:

- If every Switch has the key, semi-automatic mode uses Node LLDP to create
  synthetic leaves and annotations to connect upstream switches.
- If any Switch lacks the key, fully automatic mode uses switch-reported LLDP
  and ignores every `unifabric.io/neighbors` annotation.
- An empty value and `[]` both count as the key being present.

## Common commands

```bash
kubectl get sw
kubectl get sw leaf01 -o yaml

kubectl get sw leaf01 \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{"\t"}{.reason}{"\n"}{end}'

kubectl get sw leaf01 \
  -o jsonpath='{range .status.lldpNeighbors[*]}{.remoteSystemType}{"\t"}{.remoteSystemName}{"\n"}{end}'
```

For discovery modes and name matching rules, see
[Scale-out topology discovery design](../design/scaleout-topology.md).
