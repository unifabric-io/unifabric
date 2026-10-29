# API Reference

中文版：[README.zh.md](./README.zh.md)

Unifabric provides three cluster-scoped custom resources and one namespaced custom
resource in `unifabric.io/v1beta1`.

| Resource | Description | Reference |
| --- | --- | --- |
| `FabricNode` | RDMA NIC, LLDP neighbor, and RDMA Pod state reported by the node Agent | [fabricnode.md](./fabricnode.md) |
| `Switch` | Switch declaration, switch-agent connection state, and switch LLDP neighbors | [switch.md](./switch.md) |
| `Topology` | Read-only aggregate topology for the scale-out, scale-up, and storage networks | [topology.md](./topology.md) |
| `RDMAEndpoint` | RDMA devices, addresses, and queue pairs of one Pod, published by the Agent when `agent.ebpfFlow` is enabled | [rdmaendpoint.md](./rdmaendpoint.md) |

The generated manifests in [`chart/crds`](../../chart/crds/) are the source of truth
for the CRD OpenAPI schemas.

## Metrics

| Metric set | Reference |
| --- | --- |
| Host and Pod RDMA NIC metrics, RDMA tracing metrics, and RDMA send event fields | [metrics.md](./metrics.md) |
