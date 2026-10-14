# FabricNode

中文版：[fabricnode.zh.md](./fabricnode.zh.md)

## Overview

A `FabricNode` records the RDMA NICs, LLDP neighbors, and RDMA Pods observed on one
Kubernetes Node. The Agent creates and updates it, and users read it. For its role in
topology discovery, see the [architecture overview](../concepts/architecture.md).

## Resource Information

| Item | Value |
| --- | --- |
| API group | `unifabric.io` |
| API version | `v1beta1` |
| Kind | `FabricNode` |
| Scope | Cluster |
| Plural | `fabricnodes` |
| Singular | `fabricnode` |
| Short name | `fn` |
| Status subresource | Yes |
| Printer columns | `TotalNics`, `Ready`, `Role`, `NodeIP`, `Topologies` |

## Example YAML

The Agent generates objects like this one. Users do not write the `status`.

```yaml
apiVersion: unifabric.io/v1beta1
kind: FabricNode
metadata:
  # The name matches the Kubernetes Node name. The Node owns this
  # resource, so deleting the Node also removes the FabricNode.
  name: node1
# spec has no fields in v1beta1.
spec: {}
status:
  conditions:
    # Ready summarizes all other conditions.
    - lastTransitionTime: "2026-07-24T01:00:00Z"
      message: All FabricNode conditions are ready
      reason: Ready
      status: "True"
      type: Ready
    # LLDPNeighborsReady reports whether every selected RDMA
    # interface has a valid LLDP neighbor.
    - lastTransitionTime: "2026-07-24T01:00:00Z"
      message: All selected RDMA interfaces have LLDP neighbors
      reason: LLDPNeighborsReady
      status: "True"
      type: LLDPNeighborsReady
  # Node IP and role as reported by the Agent.
  nodeIP: 10.0.0.11
  nodeRole: GPU
  # Count of RDMA NICs classified into the scale-out or storage network.
  totalNics: 2
  # RDMA NICs in the scale-out network.
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
  # RDMA NICs in the storage network.
  storageNics:
    - name: eth5
      rdmaDeviceName: mlx5_4
      rdma: true
      ipv4: 192.168.5.11/24
      ipv6: ""
      state: up
      lldpNeighbor:
        hostname: storagesw
        mgmtIP: 10.0.5.1
        mac: 00:11:22:33:44:66
        port: Ethernet8
        description: storage switch
  # Pods on this Node that use RDMA devices.
  rdmaPods:
    - namespace: training
      name: llm-job-worker-0
      containerList:
        - containerd://3f9c2b
      hostRDMA: false
      topOwner:
        apiVersion: batch/v1
        kind: Job
        namespace: training
        name: llm-job
  # Fixed Topology names whose domain labels this Node currently carries.
  topologies:
    - scaleout
    - storage
```

## Field Definitions

### Metadata

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `metadata.name` | string | Yes | none | Matches the Kubernetes Node name. The Node is the owner, so the `FabricNode` is removed after the Node is deleted. |

The resource defines no required labels or annotations.

### Spec

`FabricNodeSpec` has no fields in `v1beta1`. Write `spec: {}`.

### Status

The Agent maintains `status`. Do not edit it manually.

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `conditions` | `[]metav1.Condition` | No | none | Current conditions, unique by `type`. See [Conditions](#conditions). |
| `totalNics` | integer | Yes | none | Total RDMA NICs classified into the scale-out or storage network. |
| `rdmaPods` | `[]RdmaPod` | No | none | RDMA Pods currently running on the Node. See [RdmaPod](#rdmapod). |
| `scaleOutNics` | `[]NicInfo` | No | none | RDMA NICs in the scale-out network. See [NicInfo](#nicinfo). |
| `storageNics` | `[]NicInfo` | No | none | RDMA NICs in the storage network. See [NicInfo](#nicinfo). |
| `nodeRole` | string | No | none | Node role reported by the Agent. Allowed values are `GPU` and `Storage`. |
| `nodeIP` | string | No | none | Node IP address. |
| `topologies` | `[]string` (set) | No | none | Fixed `Topology` names whose domain labels this Node currently carries, for example `["scaleout", "storage"]`. Derived from the Node labels, not reported by the Agent. |

#### NicInfo

Element type of `scaleOutNics` and `storageNics`.

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `name` | string | Yes | none | Linux interface name, for example `eth0` or `ib0`. |
| `rdmaDeviceName` | string | Yes | none | RDMA device name, for example `mlx5_0`. |
| `rdma` | boolean | Yes | none | Whether the interface supports RDMA. |
| `ipv4` | string | Yes | none | IPv4 address in CIDR notation, for example `172.17.0.1/32`. Empty when no address exists. |
| `ipv6` | string | Yes | none | IPv6 address in CIDR notation, for example `fe80::1/64`. Empty when no address exists. |
| `state` | string | Yes | none | Current interface state, for example `up`, `down`, or `unknown`. |
| `lldpNeighbor` | `LLDPNeighbor` | No | none | The neighbor device connected to this NIC. See [LLDPNeighbor](#lldpneighbor). |

#### LLDPNeighbor

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `hostname` | string | Yes | none | Hostname of the neighbor device. |
| `mgmtIP` | string | Yes | none | Management IP address of the neighbor device. |
| `mac` | string | Yes | none | MAC address of the neighbor device. |
| `port` | string | Yes | none | Port name on the neighbor device, for example `Ethernet32`. |
| `description` | string | Yes | none | Description of the neighbor device. |

The schema requires these fields when an `lldpNeighbor` object is present. A value
that the Agent cannot collect is an empty string.

#### RdmaPod

Element type of `rdmaPods`.

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `namespace` | string | Yes | none | Namespace of the RDMA Pod. |
| `name` | string | Yes | none | Name of the RDMA Pod. |
| `containerList` | `[]string` | No | none | IDs of the containers in the RDMA Pod. |
| `hostRDMA` | boolean | No | `false` | Whether the Pod runs in host RDMA mode. That mode typically mounts `/dev/infiniband`, enables privileged mode, and sets `hostNetwork: true`. |
| `topOwner` | `OwnerRef` | No | none | Top-level owner of the RDMA Pod. See [OwnerRef](#ownerref). |

#### OwnerRef

| Field | Type | Required | Default | Description |
| --- | --- | --- | --- | --- |
| `apiVersion` | string | No | none | API version of the owner. |
| `kind` | string | No | none | Kind of the owner, for example `Job`. |
| `namespace` | string | No | none | Namespace of the owner. |
| `name` | string | No | none | Name of the owner. |

#### Conditions

The Agent sets two condition types. The tables enumerate every status and reason with
its trigger.

`LLDPNeighborsReady`:

| Status | Reason | Trigger |
| --- | --- | --- |
| `True` | `LLDPNeighborsReady` | Every selected RDMA interface has an LLDP neighbor, or no selected interface requires LLDP. |
| `False` | `LLDPNeighborMissing` | At least one selected interface is `up` but reports no LLDP neighbor hostname. The message lists the interfaces. |
| `Unknown` | `DiscoveryFailed` | The Agent failed to list the network interfaces or failed to run LLDP discovery. The message contains the error. |

`Ready`:

| Status | Reason | Trigger |
| --- | --- | --- |
| `True` | `Ready` | All other conditions are `True`. |
| `False` | `ConditionNotReady` | At least one other condition is `False`. The message names that condition. |
| `Unknown` | `ConditionUnknown` | At least one other condition is `Unknown`, and none is `False`. |

The Agent also emits Kubernetes Events on `LLDPNeighborsReady` transitions. A failed
discovery uses the `LLDPDiscoveryFailed` event reason instead of `DiscoveryFailed`.

For how to act on these fields, see the
[SONiC RoCE guide](../getting-started-sonic-roce.md).
