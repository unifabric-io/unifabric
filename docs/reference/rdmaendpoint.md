# RDMAEndpoint

中文版：[rdmaendpoint.zh.md](./rdmaendpoint.zh.md)

An `RDMAEndpoint` is a namespaced, status-only resource published by the
Agent for every Pod on its node that holds an RDMA device open. It records the
devices, ports, GIDs, LID, subnet prefix and the queue pairs that reached
RTR, so Agents on other nodes can attribute traffic addressed to a node NIC or
to an InfiniBand LID back to the receiving Pod. It has no `spec`.

The resource only exists when `agent.ebpfFlow.enabled` is true, see the
[eBPF RDMA flow design](../design/rdma-flow.md).

## Example

```yaml
apiVersion: unifabric.io/v1beta1
kind: RDMAEndpoint
metadata:
  name: vllm-decode-0
  namespace: tenant-a
  labels:
    app.kubernetes.io/managed-by: unifabric-agent
    unifabric.io/node: gpu-node-03
  ownerReferences:
    - apiVersion: v1
      kind: Pod
      name: vllm-decode-0
      uid: 64e05f6d-91a3-45e8-8c55-e988b2d16271
      controller: true
      blockOwnerDeletion: true
status:
  nodeName: gpu-node-03
  hostNetwork: true
  lastUpdateTime: "2026-09-23T03:12:41Z"
  interfaces:
    - name: ibs576
      rdmaDevice: mlx5_0
      ports:
        - port: 1
          linkLayer: InfiniBand
          lid: 17
          subnetPrefix: fe80000000000000
          gids:
            - index: 0
              gid: fe80:0000:0000:0000:a088:c203:0088:6a44
          queuePairs:
            - qpn: 1234
              pid: 48213
    - name: eth1
      rdmaDevice: mlx5_2
      ports:
        - port: 1
          linkLayer: Ethernet
          lid: 0
          gids:
            - index: 3
              gid: 0000:0000:0000:0000:0000:ffff:ac11:0367
```

## Resource information

| Field | Value |
| --- | --- |
| API group | `unifabric.io` |
| API version | `v1beta1` |
| Kind | `RDMAEndpoint` |
| Plural | `rdmaendpoints` |
| Short name | `rep` |
| Scope | Namespaced, same namespace as the Pod |
| Subresources | `status` |

Printed columns: `Node`, `HostNetwork`, `Updated`.

## Metadata

| Field | Meaning |
| --- | --- |
| `metadata.name` | The Pod name. |
| `metadata.labels["unifabric.io/node"]` | The node whose Agent publishes the object. A restarted Agent adopts the objects carrying its node name. |
| `metadata.ownerReferences` | A single controller reference to the Pod, so the object is garbage collected with it. |

## Status

| Field | Type | Meaning |
| --- | --- | --- |
| `nodeName` | string | Node running the Pod and publishing the object. |
| `hostNetwork` | bool | Whether the Pod shares the node network namespace. Its RDMA addresses then belong to the node NIC and can only be attributed through this object. |
| `lastUpdateTime` | time | When the Agent last observed the Pod. Readers treat objects not refreshed for 15 minutes as stale. |
| `interfaces[]` | list | RDMA devices the Pod holds open. |
| `interfaces[].name` | string | Netdev bound to the device, empty when none is visible from the Pod. |
| `interfaces[].rdmaDevice` | string | ibdev name such as `mlx5_0`. |
| `interfaces[].ports[]` | list | Ports of the device. |
| `interfaces[].ports[].port` | int | 1-based port number. |
| `interfaces[].ports[].linkLayer` | enum | `InfiniBand` or `Ethernet`. |
| `interfaces[].ports[].lid` | int | LID assigned by the subnet manager, 0 on Ethernet and on InfiniBand without an active SM. |
| `interfaces[].ports[].subnetPrefix` | string | Upper 64 bits of GID index 0 as 16 hex digits. LIDs are only unique inside a subnet and must be read together with this field. |
| `interfaces[].ports[].gids[]` | list | Non-zero GIDs of the port with their sysfs index. Prefix-only entries such as `fe80::` are omitted. |
| `interfaces[].ports[].queuePairs[]` | list | Queue pairs of the Pod that reached RTR on this port. QPNs are unique per HCA port, which is why they hang under the port. |
| `interfaces[].ports[].queuePairs[].qpn` | int | Queue pair number. |
| `interfaces[].ports[].queuePairs[].pid` | int | Host tgid of the process that created the queue pair. |

## Lifecycle

- Created within `endpoint.minInterval`, default 3 s, of a Pod opening an
  RDMA device or of one of its queue pairs reaching RTR.
- Rewritten when its content changes, otherwise at most every 5 minutes to
  refresh `lastUpdateTime`. `endpoint.interval`, default 30 s, is the
  fallback publish period.
- `queuePairs` shrink when the owning process exits and the Agent removes
  its BPF map entries.
- Deleted when the Pod no longer holds a device and has no queue pairs,
  logged as `rdma_endpoint_deleted reason=no_rdma_holder`, and garbage
  collected with the Pod.
- By default only hostNetwork Pods and Pods not yet in the Agent's index are
  published. `endpoint.allPods` publishes every holder and is required on
  InfiniBand fabrics.

## Queries

```bash
kubectl get rep -A
kubectl get rep -A -l unifabric.io/node=gpu-node-03
kubectl get rep -n tenant-a vllm-decode-0 -o yaml
```

## RBAC

The Agent ClusterRole has `create`, `delete`, `get`, `list`, `watch` and
`patch` on `rdmaendpoints` and `patch` on `rdmaendpoints/status`. Writes use
Server-Side Apply with field manager `unifabric-agent`.
