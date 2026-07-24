# Unifabric API Reference

中文版：[README.zh.md](./README.zh.md)

Unifabric provides three cluster-scoped custom resources in
`unifabric.io/v1beta1`. Each CRD has a dedicated API reference:

| Kind | Resource | Short name | Description |
| --- | --- | --- | --- |
| [`FabricNode`](./fabricnode.md) | `fabricnodes` | `fn` | RDMA NIC, LLDP neighbor, and RDMA Pod state reported by the node Agent |
| [`Switch`](./switch.md) | `switches` | `sw` | Switch declaration, switch-agent connectivity, and switch LLDP state |
| [`Topology`](./topology.md) | `topologies` | `topo` | Read-only aggregate for scale-out, scale-up, and storage topology |

List the APIs installed in the cluster:

```bash
kubectl api-resources --api-group=unifabric.io
kubectl get fn
kubectl get sw
kubectl get topo
```

`spec` represents desired state declared by a user or component. `status`
represents observed state maintained by Unifabric components. Do not edit
`status` manually unless a document explicitly says otherwise.

The generated manifests in [`chart/crds`](../../chart/crds/) are the source of
truth for the CRD OpenAPI schemas.
