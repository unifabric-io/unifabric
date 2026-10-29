# InfiniBand Network Discovery

中文版: [infiniband-discovery.zh.md](./infiniband-discovery.zh.md)

## Introduction

InfiniBand is a high-speed interconnect technology that is separate from
Ethernet. The Subnet Manager keeps the topology of the full fabric. Unifabric
uses the `infiniband-k8s` provider of NVIDIA Topograph. Topograph runs
`ibnetdiscover` in a Pod on the GPU nodes. It reads the switches, the HCAs, and
the links in the IB subnet. For the Scale-up network, Topograph reads the
`nvidia.com/gpu.clique` label that the GPU Operator writes on each node. This
document tells you how to configure InfiniBand network discovery.

| Item | Description |
| --- | --- |
| Applicable networks | Scale-out, Storage |
| Topology source | Scale-out: `ibnetdiscover`. Scale-up: the clique label of the GPU Operator. |
| Discovered tiers | Scale-out: leaf, spine, core. Scale-up: tier 1, that is, the NVLink domain. |
| Switch side requirements | The Subnet Manager runs. |
| Cluster side requirements | `ibnetdiscover` can run on the GPU nodes. The Scale-up network needs the GPU Operator. |
| Helm configuration | `topoDiscovery.scaleOut.mode=nv-topograph` and `topoDiscovery.scaleUp.mode=nv-topograph`. |

For the Scale-out network, the label value is a name that comes from the
InfiniBand switch GUID, for example `S-fc6a1c03006636c0`.

Limits:

- This method does not create `Switch` resources. The `Topology` resources and
  the Node labels are complete. But in topology visualization, the switch member
  card of each performance domain is empty. See
  [Optional: add Switch resources](#optional-add-switch-resources).
- Unifabric does not yet separate the Scale-out network and the Storage network
  in the topology that InfiniBand network discovery finds. The discovery result is used
  only as the Scale-out network. If the Storage network is Ethernet, use
  [LLDP discovery in RoCE networks](./roce-lldp-discovery.md) or [Custom network topology](./custom-network-topology.md)
  for it. If the Storage network is InfiniBand, topology discovery is not
  supported for it now.
- The NVLink domains of the Scale-up network need a successful `ibnetdiscover`
  run. On a cluster without InfiniBand, this method cannot supply NVLink domains
  alone.

For the discovery method to use for each network on each network device, see
[Network topology discovery](./README.md#choose-a-discovery-method-by-network).

## Prerequisites

- You completed the deployment as described in
  [Deploy Unifabric](../../getting-started.md).
- The cluster uses an NVIDIA InfiniBand network. For example, Mellanox NICs run
  in IB mode and connect to IB switches. RDMA devices in IB mode exist on the
  nodes.
- The Subnet Manager runs in the IB fabric.
- If you want to discover the Scale-up network, the GPU Operator is installed in
  the cluster. The nodes also have the `nvidia.com/gpu.clique` label.

## Configuration

Select NVIDIA Topograph as the topology writer for the Scale-out network and the
Scale-up network:

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set topoDiscovery.scaleUp.mode=nv-topograph \
  --set topoDiscovery.scaleOut.mode=nv-topograph \
  --wait
```

The default value of `nvidiaTopograph.provider.name` is `infiniband-k8s`. You do
not have to set it. If you do not want to discover the Scale-up network, remove
`--set topoDiscovery.scaleUp.mode=nv-topograph`.

## Verification

Make sure that the scale-out labels are on the Nodes:

```bash
kubectl get nodes -L scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2
```

If you configured the Scale-up network, make sure that the scale-up labels are
on the Nodes:

```bash
kubectl get nodes -L scale-up.unifabric.io/tier-1
```

For the meaning of the labels and for how to view the `Topology` resources, see
[Network topology discovery](./README.md#view-topology-and-node-labels).

## Optional: add Switch resources

This method does not create `Switch` resources. If you want topology
visualization to show the switches, see
[Create Switch instances](./topograph-switch-resources.md).

## Troubleshooting

### Nodes have no topology labels

1. Make sure that the Topograph Deployment runs, and read its logs:

   ```bash
   kubectl -n unifabric-system get deployment unifabric-nvidia-topograph
   kubectl -n unifabric-system logs deployment/unifabric-nvidia-topograph --tail 100
   ```

2. Make sure that the logs show that `ibnetdiscover` runs correctly.

## References

- [Network topology discovery](./README.md): how to choose a discovery method
  for each network, and how to view the results.
- [NVIDIA NetQ discovery in RoCE networks](./roce-netq-discovery.md): a different discovery method that also uses
  NVIDIA Topograph.
- [Create Switch instances](./topograph-switch-resources.md): an
  optional step.
- [Topograph InfiniBand provider](https://github.com/dsx-ai-factory/topograph/blob/main/docs/providers/infiniband.md):
  the upstream document.
