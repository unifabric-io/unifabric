# Unifabric Architecture

中文版：[architecture.zh.md](./architecture.zh.md)

## Overview

Unifabric is a cloud-native observability system for the high-speed networks of
Kubernetes AI clusters. An AI cluster depends on three such networks:

- The scale-out network carries GPU traffic between nodes over RoCE or InfiniBand.
- The scale-up network is the GPU interconnect, such as NVIDIA NVLink. It connects
  the GPUs inside one node, or the GPUs of many nodes in one multi-node NVLink domain.
- The storage network carries RDMA traffic between the compute nodes and high-speed
  storage.

These networks contain switches and NICs from many vendors. The tools of each vendor
are not connected to Kubernetes, and the usual cloud-native observability stack is
built around TCP/IP. RDMA bypasses the kernel network stack, so that stack cannot see
it. Thus Kubernetes cannot attribute a network problem to a Pod or a job. Unifabric
fills this gap with three capabilities:

- Topology discovery: it finds the links between nodes and switches, and between
  switches, and records the network position of each node.
- AI high-speed network traffic observability: it collects RDMA traffic, congestion, and
  error data, and attributes the data to Pods, namespaces, and workloads.
- Network health checks: it reports the health of the LLDP links and of the switch
  connections continuously.

Unifabric presents the results in one Kubernetes resource model: custom resources,
Node labels, and Prometheus metrics. The model is the same for all vendors.

Design goals:

- Make the physical network topology available through the Kubernetes API, without
  manual records of the cabling.
- Give topology-aware schedulers accurate Node labels that follow topology changes
  automatically.
- Attribute network traffic to workloads without application changes.
- Support SONiC RoCE, NVIDIA Spectrum-X, and InfiniBand fabrics through one data model.
  For each network, select the discovery method that matches the network devices.

Non-goals:

- Unifabric is not a scheduler and does not decide Pod placement. It supplies topology
  data to schedulers such as Kueue, Volcano, and KAI Scheduler.
- Unifabric does not configure switches or NICs. It only observes them.
- Unifabric does not replace vendor network management platforms such as NVIDIA NetQ.
  It can import topology from them.

## Overall Architecture

![Unifabric architecture](../images/architecture.png)

The diagram has four layers. The infrastructure layer contains the switches and the
RDMA nodes. The topology source layer contains LLDP and, optionally, NVIDIA Topograph
with NetQ. Unifabric runs in Kubernetes and writes Node labels, custom resources, and
metrics. Schedulers, Prometheus, and Grafana consume these results.

Unifabric has the components below. The Helm chart deploys all of them except
switch-agent.

| Component | Form | Responsibility |
| --- | --- | --- |
| Controller | Deployment | It calculates the topology from `FabricNode` and `Switch` data. It writes tiered topology labels to Nodes and domain labels to `Switch` resources. It aggregates these labels into the `Topology` resources. It also serves the read-only Topology HTTP API for the Grafana topology plugins. |
| Agent | DaemonSet | On each node, it discovers the RDMA NICs and classifies them as scale-out or storage NICs. It reads the LLDP neighbor of each NIC and finds the Pods that use RDMA devices. It writes the result to the `FabricNode` resource of the node. It exposes RDMA metrics with Pod and workload attribution. When eBPF flow attribution is enabled, it also publishes `RDMAEndpoint` resources and Pod-to-Pod flow metrics. |
| switch-agent | Container or systemd service on a switch (optional) | It reads the LLDP neighbors of the switch from the `lldpd` daemon on the switch. It serves them over mTLS gRPC on port `8090`. The Controller subscribes to each switch-agent. It is necessary only for LLDP discovery in RoCE networks. |
| NVIDIA Topograph | Deployment (optional) | It reads the fabric topology from NVIDIA NetQ, or from `ibnetdiscover` on an InfiniBand fabric. It writes the tiered Node labels with the Unifabric label keys. The chart deploys it only when a network uses the `nv-topograph` discovery mode. |
| Flow event collector | OpenTelemetry Collector Deployment (optional) | It receives per-send RDMA flow events from the Agents over OTLP and writes them to ClickHouse or Elasticsearch. |
| Grafana dashboards and plugins | Grafana Operator resources (optional) | They provide the RDMA metric dashboards, the flow dashboards, and the topology data source and panel plugins. |

## Core Concepts

| Term | Definition |
| --- | --- |
| Scale-out network | The node-to-node GPU network, in a leaf-spine or a deeper Clos design. Its Node labels have the prefix `scale-out.unifabric.io`, and its `Topology` resource is `scaleout`. |
| Scale-up network | The GPU interconnect, such as NVLink. Discovery is necessary only for a multi-node NVLink system, such as GB200 NVL72. Its Node label is `scale-up.unifabric.io/tier-1`, and its `Topology` resource is `scaleup`. |
| Storage network | The RDMA network between the compute nodes and storage. Unifabric models it separately from the scale-out network, so storage NICs do not change the compute topology. Its Node labels have the prefix `storage.unifabric.io`, and its `Topology` resource is `storage`. |
| Performance domain | A group of nodes, and of the switches that serve them, at one tier of one network. Traffic inside a domain does not go to a higher tier. Domains form a tree: tier 1 is the leaf tier, tier 2 is the spine tier, and tier 3 is the core tier. In the scale-up network, tier 1 is the NVLink domain. |
| Node topology labels | The domain path of a node, written as one label for each tier, for example `scale-out.unifabric.io/tier-1=tier1-group1`. Two nodes with the same value for a label are in the same domain at that tier. |
| Discovery mode | The topology source of one network, set by the Helm value `topoDiscovery.<network>.mode`. `unifabric-roce` calculates the topology from LLDP data. `nv-topograph` imports it from NVIDIA Topograph. `manual` means that an administrator writes the labels. |
| `FabricNode` | A cluster-scoped resource for each Kubernetes Node. The Agent writes it. It records the RDMA NICs, their LLDP neighbors, the node IP and role, the Pods that use RDMA devices, and the health conditions of the node. See the [FabricNode reference](../reference/fabricnode.md). |
| `Switch` | A cluster-scoped resource for one physical switch. A user creates it with the management IP and the network role of the switch. The Controller writes the switch-agent connection state and the LLDP neighbors to its status. See the [Switch reference](../reference/switch.md). |
| `Topology` | A cluster-scoped, read-only resource. The Controller aggregates it from the Node and `Switch` labels. It lists the performance domains of one network, their parent domains, and the domain path of each node. The three resources are `scaleout`, `scaleup`, and `storage`. See the [Topology reference](../reference/topology.md). |
| `RDMAEndpoint` | A namespaced, status-only resource for each Pod that has an RDMA device open. The Agent publishes it when eBPF flow attribution is enabled. It records the GIDs, LIDs, and queue pairs of the Pod, so that the Agents on other nodes can find the destination Pod of the traffic. See the [RDMAEndpoint reference](../reference/rdmaendpoint.md). |
| Workload attribution | The mapping from an RDMA device, or a queue pair, to the Pod that uses it, and from the Pod to its top-level owner, such as a Job or a LeaderWorkerSet. RDMA metrics and flow data carry these labels. |

The concepts relate as follows. Each network has a discovery mode. In the
`unifabric-roce` mode, the Agents supply the node-side links through `FabricNode`, and
`Switch` resources supply the switch-side links. The Controller calculates the
performance domains and writes them as Node topology labels. In the `nv-topograph`
mode, NVIDIA Topograph writes the same labels. In both modes, the Controller then
aggregates the labels into the `Topology` resource of the network.

## Workflows

### Topology discovery

1. Collect. The Agent on each node writes the RDMA NICs and their LLDP neighbors to
   the `FabricNode` resource. For each `Switch` resource, the Controller subscribes to
   the switch-agent and stores the switch-side LLDP neighbors in the `Switch` status.
2. Calculate. In the `unifabric-roce` mode, the Controller combines the node-to-leaf
   links from `FabricNode` with the switch-to-switch links. The switch-to-switch links
   come from the switch-agent, or from the neighbor annotations on the `Switch`
   resources when no switch-agent runs. The Controller groups the switches and the
   nodes into tiered performance domains. In the `nv-topograph` mode, NVIDIA Topograph
   does this step from NetQ or `ibnetdiscover` data.
3. Publish. The domain path of each node goes to the Node labels. The Controller
   aggregates the Node and `Switch` labels into the `Topology` resource.
4. Consume. Topology-aware schedulers read the Node labels. The Grafana topology
   plugins read the `Topology` resources through the Topology HTTP API.

For the discovery methods and the label model, see
[Topology discovery](./topology-discovery.md).

### RDMA metrics

1. The Agent reads the device, port, and priority counters of each RDMA NIC on the
   node. The counters cover throughput, congestion, and errors.
2. The Agent maps each RDMA device to the Pods that use it, and each Pod to its
   top-level workload owner. The metrics thus carry the Pod, namespace, and workload.
3. Prometheus scrapes the Agent metrics endpoint. Grafana dashboards show the
   metrics for each node and for each workload.

See the [RDMA metric model and Pod attribution design](../design/rdma-metrics.md).

### RDMA flow attribution

1. When `agent.ebpfFlow.enabled` is true, the Agent attaches eBPF uprobes to the
   `libibverbs` library on the node. The probes record the destination of each queue
   pair and count the bytes and work requests that the CPU submits.
2. The Agent finds the source Pod of each queue pair from its process. It finds the
   destination Pod from the Pod IPs, or from the `RDMAEndpoint` resources that the
   Agents on other nodes publish.
3. The Agent exposes Pod-to-Pod flow metrics. When per-send events are enabled, it
   also sends one event for each send to the flow event collector over OTLP.

Unifabric observes only RDMA that the CPU submits. RDMA that GPU kernels submit
directly to the NIC queues, such as GDAKI or IBGDA, bypasses the probes. See the
[eBPF RDMA flow attribution design](../design/rdma-flow.md).

### Network health status

1. The Agent sets the `LLDPNeighborsReady` and `Ready` conditions on each
   `FabricNode`. A NIC without an LLDP neighbor makes the node not ready, and the
   Agent records a Kubernetes Event.
2. The Controller sets `status.healthy` on each `Switch` after it receives a valid
   snapshot from the switch-agent. It sets the value to false when the subscription
   fails.

Active probing of network paths is planned. See the [roadmap](../roadmap.md).

## Use Cases

- Topology-aware scheduling. Kueue, Volcano, or KAI Scheduler reads the tiered Node
  labels and places the Pods of one training or inference job in the same leaf or
  spine domain. This makes the communication paths shorter and reduces interference
  from other jobs on the upper links.
- Diagnosis of a slow job. When a training or inference job slows down, operators
  correlate the RDMA metrics of its Pods and nodes in Grafana. They find the
  congested links and the workloads that use the bandwidth.
- Analysis of communication patterns. With eBPF flow attribution, operators see which
  Pods communicate, how much data they send, and the message sizes. They then confirm
  that the communication pattern of a job is as expected.
- Location of faults and of their scope. When a link reports errors, the topology
  graph and the `FabricNode` resources show the affected NIC, leaf switch, nodes, and
  jobs. Operators do not have to log in to each switch.
- Routine inspection of the fabric. The `FabricNode` conditions and the `Switch`
  health status give a continuous view of the LLDP links and the switch connections
  in a large cluster.
