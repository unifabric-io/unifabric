# Unifabric Documentation

中文版：[README.zh.md](./README.zh.md)

Unifabric turns RDMA network hierarchy, node proximity, and NIC state in Kubernetes AI clusters
into data that Kubernetes can query and consume:

- Agents collect node RDMA NICs, LLDP neighbors, and Pod network attribution.
- The Controller or NVIDIA Topograph discovers topology and writes the result to Node labels.
- `FabricNode`, `Switch`, and `Topology` CRs expose cluster-level network state.
- Prometheus metrics and Grafana dashboards expose RDMA traffic, congestion, errors, and Pod
  attribution.

Schedulers can use these results to place communication-intensive AI workloads on nearby Nodes,
while operators can diagnose RDMA network and workload traffic problems.

## Quick Start

1. Follow the [basic installation](./getting-started.md) to deploy the Controller, Agent, and
   observability components shared by every scenario.
2. Select the topology-discovery scenario for the physical network:

| Scenario | Primary topology | Discovery method | Guide |
| --- | --- | --- | --- |
| General SONiC RoCE | Scale-out and RoCE storage | Node and switch topology discovery over LLDP | [General SONiC RoCE](./getting-started-sonic-roce.md) |
| Spectrum-X | Scale-out | NVIDIA Topograph with NetQ | [Spectrum-X fabric](./getting-started-spectrum-x.md) |
| InfiniBand | Scale-out and Scale-up | NVIDIA Topograph | [InfiniBand fabric](./getting-started-infiniband.md) |

Scenario guides contain only the Helm values and onboarding steps required by that network. The
basic installation guide owns shared version, image, and monitoring configuration. For other Chart
parameters, see the defaults and descriptions in the
[Helm values reference](../chart/README.md).

## Core Features

### Topology Discovery

Topology discovery converts physical adjacency into hierarchical performance domains and writes
them to Kubernetes Node labels. Unifabric models three logical topologies:

- **Scale-out network:** Node-to-node communication through leaf, spine, and core tiers. See the
  [General SONiC RoCE](./getting-started-sonic-roce.md),
  [Spectrum-X](./getting-started-spectrum-x.md), or
  [InfiniBand](./getting-started-infiniband.md) guides for scenario-specific configuration.
- **Scale-up network:** High-bandwidth GPU interconnect domains such as NVLink and NVSwitch. See
  the [InfiniBand scenario](./getting-started-infiniband.md) for configuration.
- **Storage network:** A dedicated RDMA network between compute Nodes and storage services.
  Currently, only [General SONiC RoCE](./getting-started-sonic-roce.md) is supported.

Discovery results are exposed to schedulers as Node labels and aggregated into read-only
`Topology` CRs. Use the
[FabricNode, Switch, and Topology API reference](./reference/README.md) to query the results.

### Topology-Aware Scheduling

Kueue, Volcano, KAI Scheduler, and similar systems can consume the discovered Node labels and place
Pods from one workload in the same or nearby performance domains, reducing traffic across switch
tiers.

- [Topology-aware scheduling and Kueue TAS example](./usage/workload-tas.md)
- [Topology API reference](./reference/topology.md)

### RDMA Traffic and Health Observability

Unifabric Agents collect RDMA device, port, priority, throughput, congestion, and error metrics and
attribute traffic to Pods, namespaces, and top-level workloads. Grafana dashboards help operators
diagnose RDMA traffic and health by cluster, Node, and workload.

- [RDMA observability usage guide](./usage/rdma-metrics.md)
- [Application flow observability guide](./usage/sflow.md): Install switch sample ingestion,
  flow-record storage, schema initialization, and workload dashboards with Helm.

## Development

- [NVAIR development guide](./development/dev-with-nvair.md): Build a local topology and end-to-end
  development environment.

## Design

- [Unifabric API reference](./reference/README.md): Query `FabricNode`, `Switch`, and `Topology`.
- [Topology CRD design](./design/topology-crd.md): Understand performance domains, Node paths, and
  the topology data model.
- [Scale-out topology discovery design](./design/scaleout-topology.md): Understand how Scale-out
  topologies are discovered and constructed.
- [RDMA metric model and Pod attribution design](./design/rdma-metrics.md): Understand metric
  definitions and workload attribution.
- [Application flow observability design](./design/sflow.md): Understand the sFlow collector,
  Pod attribution, ClickHouse row model, and overload behavior.
