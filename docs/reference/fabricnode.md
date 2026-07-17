# FabricNode

中文版：[fabricnode.zh.md](./fabricnode.zh.md)

A `FabricNode` records RDMA NICs, LLDP neighbors, and RDMA Pods observed on a
Kubernetes Node. The node Agent creates and updates this resource. Users
normally read it and should not maintain its `status` manually.

## Example

The Agent generates the following object:

```yaml
apiVersion: unifabric.io/v1beta1
kind: FabricNode
metadata:
  name: node1
spec: {}
status:
  conditions:
    - lastTransitionTime: "2026-07-24T01:00:00Z"
      message: All FabricNode conditions are ready
      reason: Ready
      status: "True"
      type: Ready
    - lastTransitionTime: "2026-07-24T01:00:00Z"
      message: LLDP neighbors are ready
      reason: LLDPNeighborsReady
      status: "True"
      type: LLDPNeighborsReady
  nodeIP: 10.0.0.11
  nodeRole: GPU
  totalNics: 1
  scaleOutNics:
    - name: eth1
      rdmaDeviceName: mlx5_0
      rdma: true
      ipv4: 192.168.1.11/24
      ipv6: ""
      state: up
      lldpNeighbor:
        hostname: leaf01
        mgmtIP: 10.0.0.1
        mac: 00:11:22:33:44:55
        port: Ethernet1
        description: leaf switch
```

## Resource information

| Field | Value |
| --- | --- |
| API group | `unifabric.io` |
| API version | `v1beta1` |
| Kind | `FabricNode` |
| Scope | Cluster |
| Resource | `fabricnodes` |
| Singular name | `fabricnode` |
| Short name | `fn` |
| Status subresource | Yes |

The resource name matches its Kubernetes Node name, and that Node is its owner.
The corresponding `FabricNode` is cleaned up after the Node is deleted.

## FabricNode fields

| Field | Type | Description |
| --- | --- | --- |
| `apiVersion` | string | `unifabric.io/v1beta1` |
| `kind` | string | `FabricNode` |
| `metadata` | `metav1.ObjectMeta` | Standard Kubernetes metadata |
| `spec` | `FabricNodeSpec` | Desired state; currently empty |
| `status` | `FabricNodeStatus` | Read-only observed state maintained by the Agent |

## FabricNodeSpec

`FabricNodeSpec` currently has no configurable fields.

## FabricNodeStatus

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `conditions` | `[]metav1.Condition` | No | Current resource conditions, unique by `type` |
| `totalNics` | integer | Yes | Total RDMA NICs classified into the scale-out or storage network |
| `rdmaPods` | `[]RdmaPod` | No | RDMA Pods currently running on the Node |
| `scaleOutNics` | `[]NicInfo` | No | RDMA NICs in the scale-out network |
| `storageNics` | `[]NicInfo` | No | RDMA NICs in the storage network |
| `nodeRole` | string | No | Node role: `GPU` or `Storage` |
| `nodeIP` | string | No | Node IP address |

Known condition types:

| Type | Description |
| --- | --- |
| `Ready` | Summarizes the other conditions and is `True` when all conditions are ready |
| `LLDPNeighborsReady` | Whether every interface that requires LLDP has a valid neighbor |

A common reason for `LLDPNeighborsReady=False` is `LLDPNeighborMissing`.
Collection failures use the `DiscoveryFailed` reason.

## NicInfo

Element type used by `scaleOutNics` and `storageNics`.

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `name` | string | Yes | Linux interface name, such as `eth1` or `ib0` |
| `rdmaDeviceName` | string | Yes | RDMA device name, such as `mlx5_0` |
| `rdma` | boolean | Yes | Whether the interface supports RDMA |
| `ipv4` | string | Yes | IPv4 address in CIDR notation; empty when no address exists |
| `ipv6` | string | Yes | IPv6 address in CIDR notation; empty when no address exists |
| `state` | string | Yes | Link state, such as `up`, `down`, or `unknown` |
| `lldpNeighbor` | `LLDPNeighbor` | No | LLDP neighbor observed on this interface |

Topology discovery uses only NICs that match the current fabric role, are
`up`, and have both `hostname` and `port` in their LLDP neighbor.

## LLDPNeighbor

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `hostname` | string | Yes | Neighbor device hostname |
| `mgmtIP` | string | Yes | Neighbor management IP |
| `mac` | string | Yes | Neighbor MAC address |
| `port` | string | Yes | Neighbor port, such as `Ethernet32` |
| `description` | string | Yes | Neighbor device description |

The schema requires these fields when an `lldpNeighbor` object is present. A
value that cannot be collected may be an empty string.

## RdmaPod

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `namespace` | string | Yes | Pod namespace |
| `name` | string | Yes | Pod name |
| `containerList` | `[]string` | No | IDs of containers using RDMA |
| `hostRDMA` | boolean | No | Whether the Pod uses host RDMA mode |
| `topOwner` | `OwnerRef` | No | Top-level workload owner of the Pod |

`OwnerRef` may contain `apiVersion`, `kind`, `namespace`, and `name`.

## Common commands

```bash
kubectl get fn
kubectl get fn node1 -o yaml

kubectl get fn node1 \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{"\t"}{.reason}{"\n"}{end}'

kubectl get fn node1 \
  -o jsonpath='{range .status.scaleOutNics[*]}{.name}{"\t"}{.state}{"\t"}{.lldpNeighbor.hostname}{"\n"}{end}'
```

For details about how topology calculation uses these fields, see
[Scale-out topology discovery design](../design/scaleout-topology.md).
