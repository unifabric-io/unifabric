# Unifabric Architecture

中文版：[architecture.zh.md](./architecture.zh.md)

## Overview

Unifabric is an RDMA network topology discovery and observability system for Kubernetes
AI clusters. RDMA (direct memory access between servers, without CPU copies) runs on
RoCE or InfiniBand fabrics. Unifabric turns the hierarchy of this network, the
proximity of nodes, and the state of NICs into data that Kubernetes can query. It
produces Node labels for schedulers, custom resources for operators and tooling, and
Prometheus metrics with Grafana dashboards for monitoring.

Design goals:

- Make the physical RDMA network visible to Kubernetes without manual cabling records.
- Enable topology-aware scheduling by writing tiered topology labels to Nodes.
- Attribute RDMA traffic, congestion, and errors to the Pods and workloads that cause them.
- Support SONiC RoCE, NVIDIA Spectrum-X, and InfiniBand fabrics through pluggable discovery methods.

Unifabric does not implement a scheduler or a placement policy. It produces the
topology data that schedulers such as Kueue TAS, Volcano, and KAI Scheduler consume.
It also does not manage switch configuration. It only observes the network.

## Overall Architecture

![Unifabric architecture](../images/architecture.png)

Unifabric runs on Kubernetes between the physical RDMA network and the scheduling and
observability stack. It consists of three components.

The Controller is a Deployment. It computes the topology from `FabricNode` and
`Switch` data, or imports topology discovered by NVIDIA Topograph. It writes tiered
topology labels to Kubernetes Nodes and maintains the read-only `Topology` resource.
It also serves the Topology HTTP API that the visualization plugins consume.

The Agent is a DaemonSet. On each node, it collects the RDMA NICs, their LLDP
neighbors, and the Pods that use RDMA devices into a `FabricNode` resource. LLDP is a
protocol in which each device announces itself to its link peers. The Agent also
exposes the RDMA metrics of the node to Prometheus.

The Switch Agent is optional. It runs on SONiC switches as a container, a systemd
service, or a binary. It reports switch-side LLDP neighbor snapshots to the Controller
over gRPC. This enables switch-to-switch discovery when the fabric has more than one
switch tier.

## Core Concepts

| Term | Definition |
| --- | --- |
| `FabricNode` | A cluster-scoped resource, one per Kubernetes Node, that the Agent creates and updates. It records the RDMA NICs of the node, their LLDP neighbors, the node IP, the node role, and the Pods that use RDMA devices. It is the node-side input to topology discovery. |
| `Switch` | A cluster-scoped resource that describes one physical switch. It records the management endpoint of the switch, its network role, and the LLDP neighbor snapshot that the Switch Agent reports. It is the switch-side input to topology discovery. |
| `Topology` | A cluster-scoped, read-only resource that aggregates discovery results. Its status lists performance domains, their parent relationships, and the domain path of every Node. Three fixed names exist: `scaleout`, `scaleup`, and `storage`. |
| Performance domain | A group of Nodes, and the switches that serve them, that communicate at one network tier. Domains form a tree. Tier 1 is nearest to the Nodes, and higher tiers such as spine and core are parents of lower tiers. |
| Scale-out network | The node-to-node RDMA network that crosses leaf, spine, and core switch tiers. Cross-node GPU training traffic uses it. Discovery writes labels such as `scale-out.unifabric.io/tier-1`. |
| Scale-up network | A high-bandwidth GPU interconnect domain such as NVLink or NVSwitch. GPUs inside one domain communicate without crossing the scale-out fabric. |
| Storage network | A dedicated RDMA network between compute Nodes and storage services. Unifabric models it separately, so storage NICs do not distort the compute topology. |
| Node topology labels | Discovery results written to Kubernetes Nodes as tiered labels, for example `scale-out.unifabric.io/tier-1=tier1-group1`. Topology-aware schedulers consume these labels. |

The concepts relate as follows. Agents produce `FabricNode` resources, and the Switch
Agent enriches `Switch` resources. The Controller consumes both, computes performance
domains, writes them as Node topology labels, and aggregates the labels into the
`Topology` resource. Schedulers read the labels. Users and the visualization read the
`Topology` resource.

## Workflows

### Topology discovery and consumption

1. Collect. On every node, the Agent scans the RDMA NICs and classifies each one as
   scale-out, scale-up, or storage. It reads the LLDP neighbor of each NIC and writes
   the result to the `FabricNode` status. On fabrics with a Switch Agent, each switch
   reports its own LLDP neighbor snapshot to the Controller, which stores it in the
   matching `Switch` status.
2. Compute. The Controller combines node-to-switch links from `FabricNode` LLDP with
   switch-to-switch links from `Switch` snapshots or administrator-declared neighbor
   annotations. It groups switches that share downstream Nodes into tiered performance
   domains. In the Spectrum-X and InfiniBand scenarios, imported NVIDIA Topograph
   results replace this step.
3. Publish. The Controller writes the complete domain path of each Node as tiered Node
   labels. It aggregates all labels into the read-only `Topology` status.
4. Consume. Topology-aware schedulers read the Node labels to co-locate workloads.
   Operators and the visualization dashboard read the `Topology`, `FabricNode`, and
   `Switch` resources.

### RDMA observability

1. The Agent collects RDMA device, port, and priority counters from each node. The
   counters cover throughput, congestion, and errors.
2. The Agent maps each RDMA device to the Pods that use it and resolves the top-level
   workload owner of each Pod. Every metric therefore carries Pod, namespace, and
   workload attribution.
3. Prometheus scrapes the Agent metrics endpoint. Grafana dashboards present per-node
   and per-workload views.

## Use Cases

- Topology-aware scheduling. Kueue TAS, Volcano, or KAI Scheduler consumes the tiered
  Node labels to place all Pods of one training job under the same leaf or spine group.
  This reduces cross-tier traffic and improves GPU utilization.
- RDMA traffic and health monitoring. Operators watch per-node and per-workload RDMA
  throughput, congestion, and error metrics in Grafana. Degraded links and misbehaving
  workloads become visible before they slow down training jobs.
- Network troubleshooting. When a job slows down, operators inspect the `FabricNode`
  resources of the affected nodes and the topology graph. NIC state, LLDP neighbors,
  and domain membership locate the failing NIC, link, or switch.
- Fabric inventory. The `FabricNode`, `Switch`, and `Topology` resources give platform
  teams a current inventory of RDMA NICs, switch connectivity, and network hierarchy
  through the standard Kubernetes API.
- Multi-fabric clusters. Clusters that mix compute and storage RDMA networks model
  each network separately. Storage traffic and topology never distort compute
  scheduling decisions.
