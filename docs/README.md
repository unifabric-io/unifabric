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

Follow [Deploy Unifabric](./getting-started.md) to install the Controller, the
Agent, and the observability components. The default installation works on a
SONiC RoCE fabric and discovers the topology over LLDP. For an InfiniBand or
Spectrum-X fabric, the guide lists the Helm options to change:

| Scenario | Primary topology | Discovery method |
| --- | --- | --- |
| SONiC RoCE (default) | Scale-out and RoCE storage | Node and switch topology discovery over LLDP |
| InfiniBand | Scale-out and Scale-up | NVIDIA Topograph |
| Spectrum-X | Scale-out | NVIDIA Topograph with NetQ |

For the SONiC switch onboarding options, the label semantics, and discovery
troubleshooting, see the [topology discovery guide](./usage/topology-discovery.md).
For other Chart parameters, see the defaults and descriptions in the
[Helm values reference](../chart/README.md).

## Usage

- [Topology visualization](./usage/topology-visualization.md): render the discovered
  fabric in Grafana.
- [AI high-speed network traffic observability](./usage/traffic-observability/README.md):
  the traffic observability scope, the observation method of each location, and the Pod identity association.
- [Host and Pod RDMA NIC metrics](./usage/traffic-observability/rdma-metrics.md): RDMA traffic, congestion, and error
  metrics of host NICs and Pod RDMA devices, associated with Pods and jobs.
- [eBPF-based RDMA tracing](./usage/traffic-observability/rdma-flow.md): attribute RDMA sends to Pod-to-Pod
  flows with eBPF and publish `RDMAEndpoint`.
- [RDMA send event storage](./usage/traffic-observability/rdma-flow-events.md): store per-send events
  in ClickHouse or Elasticsearch.
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
