# Topology Discovery

中文版：[topology-discovery.zh.md](./topology-discovery.zh.md)

## Overview

Topology discovery converts the physical adjacency of the RDMA network into tiered
performance domains. A performance domain is a group of Nodes, and the switches that
serve them, that communicate at one network tier. The Controller writes the domain
path of each Node as Node labels and aggregates all results into the read-only
`Topology` resource. Topology-aware schedulers consume the labels to place
communication-intensive workloads on nearby Nodes.

This page explains what discovery produces and how each discovery mode works. For
component responsibilities, see the [architecture overview](./architecture.md). For
installation steps, see the [basic installation](../getting-started.md) and the
scenario guides.

## Logical Topologies

Unifabric models three logical topologies:

- Scale-out network: the node-to-node RDMA network that crosses leaf, spine, and core
  switch tiers. Discovery writes Node labels such as `scale-out.unifabric.io/tier-1`
  (the tier nearest to a Node), `scale-out.unifabric.io/tier-2`, and
  `scale-out.unifabric.io/tier-3`.
- Scale-up network: a high-bandwidth GPU interconnect domain such as NVLink or
  NVSwitch. GPUs inside one domain communicate without crossing the scale-out fabric.
- Storage network: a dedicated RDMA network between compute Nodes and storage
  services. Unifabric models it separately, so storage NICs do not distort the
  compute topology.

## Discovery Modes

The discovery mode depends on the physical network:

| Scenario | Primary topology | Discovery mode | Guide |
| --- | --- | --- | --- |
| General SONiC RoCE | Scale-out and RoCE storage | LLDP collection on nodes and switches | [General SONiC RoCE](../getting-started-sonic-roce.md) |
| NVIDIA Spectrum-X | Scale-out | Import from NVIDIA Topograph with NetQ | [Spectrum-X fabric](../getting-started-spectrum-x.md) |
| InfiniBand | Scale-out and Scale-up | Import from NVIDIA Topograph | [InfiniBand fabric](../getting-started-infiniband.md) |

### LLDP-based discovery

LLDP is a protocol in which each device announces itself to its link peers. In the
General SONiC RoCE scenario, the Agent on each node reads the LLDP neighbor of each
RDMA NIC and writes it to the `FabricNode` resource. The optional Switch Agent reports
switch-side LLDP neighbor snapshots to the `Switch` resource. The Controller combines
the node-to-switch links with the switch-to-switch links and computes the leaf, spine,
and core domains. On fabrics without a Switch Agent, administrator-declared neighbor
annotations on `Switch` resources replace the switch-side snapshots.

### Topograph-based discovery

NVIDIA Topograph is an external service that discovers fabric topology from vendor
sources. In the Spectrum-X scenario, Topograph reads the fabric from NetQ. In the
InfiniBand scenario, Topograph uses the `infiniband-k8s` provider, which reads the
fabric through `ibnetdiscover`. In both scenarios, the Controller imports the
Topograph result instead of computing domains from LLDP data.

## Discovery Results

Each discovery mode produces the same outputs:

- Node labels: the tiered domain path of each Node, written as labels for schedulers.
  See [topology-aware scheduling](../usage/workload-tas.md).
- `Topology` resource: a cluster-scoped, read-only aggregation of performance domains
  and Node paths. See the [API reference](../reference/README.md).
- Visualization: Grafana renders the discovered fabric. See
  [topology visualization](../usage/topology-visualization.md).

## Further Reading

- [Topology CRD design](../design/topology-crd.md): performance domains, Node paths,
  and the topology data model.
- [Scale-out topology discovery design](../design/scaleout-topology.md): how the
  Controller constructs scale-out topologies.
