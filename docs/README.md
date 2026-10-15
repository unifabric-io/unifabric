# Unifabric Documentation

中文版：[README.zh.md](./README.zh.md)

Unifabric discovers the RDMA network topology of Kubernetes AI clusters and makes it
observable. It writes topology labels to Nodes for schedulers, exposes the network
state through the `FabricNode`, `Switch`, and `Topology` resources, and attributes
RDMA metrics to Pods and workloads. For a full description of the components and data
flows, see the [architecture overview](./concepts/architecture.md).

Recommended reading path: install with the getting started guides, use each feature
with the usage guides, look up fields in the reference, and read the concepts and
design documents to understand how the system works.

## Concepts

- [Architecture overview](./concepts/architecture.md): components, core concepts,
  data flows, and use cases.
- [Topology discovery](./concepts/topology-discovery.md): logical topologies,
  discovery modes, and discovery results.

## Getting Started

- [Basic installation](./getting-started.md): deploy the Controller, the Agent, and
  the observability components shared by every scenario.
- [General SONiC RoCE](./getting-started-sonic-roce.md): LLDP-based discovery on
  SONiC RoCE fabrics.
- [NVIDIA Spectrum-X](./getting-started-spectrum-x.md): import topology from NVIDIA
  Topograph with NetQ.
- [InfiniBand](./getting-started-infiniband.md): import topology from NVIDIA
  Topograph.

Scenario guides contain only the Helm values and onboarding steps that the network
requires. The basic installation guide owns the shared version, image, and monitoring
configuration. For other chart parameters, see the
[Helm values reference](../chart/README.md).

## Usage

- [Topology visualization](./usage/topology-visualization.md): render the discovered
  fabric in Grafana.
- [RDMA metrics](./usage/rdma-metrics.md): RDMA traffic, congestion, and error
  metrics with Pod and workload attribution.
- [RDMA flow attribution](./usage/rdma-flow.md): attribute RDMA sends to Pod-to-Pod
  flows with eBPF, publish `RDMAEndpoint`, and store per-send events.
- [Workload TAS](./usage/workload-tas.md): topology-aware scheduling with Kueue TAS
  and similar schedulers.
- [Switch Agent deployment](./usage/switch-agent-deployment.md): deploy the Switch
  Agent on SONiC switches.

## Reference

- [API reference index](./reference/README.md)
- [FabricNode](./reference/fabricnode.md)
- [Switch](./reference/switch.md)
- [Topology](./reference/topology.md)
- [RDMAEndpoint](./reference/rdmaendpoint.md)

## Design

- [FabricNode design](./design/fabricnode.md)
- [RDMA metric model and Pod attribution design](./design/rdma-metrics.md)
- [eBPF RDMA flow attribution design](./design/rdma-flow.md)
- [Scale-out topology discovery design](./design/scaleout-topology.md)
- [Topology CRD design](./design/topology-crd.md)
- [Topology visualization design](./design/topology-visualization.md)

## Development

- [NVAIR development guide](./development/dev-with-nvair.md): build a local topology
  and end-to-end development environment.

## Roadmap

- [Roadmap](./roadmap.md): planned features and future direction.
