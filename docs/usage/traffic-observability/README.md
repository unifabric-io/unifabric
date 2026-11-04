# AI High-Speed Network Traffic Observability

中文版: [README.zh.md](./README.zh.md)

## Introduction

Unifabric AI high-speed network traffic observability collects the communication traffic of AI applications on the high-speed networks.
It associates the traffic with Pods, namespaces, and top-level workloads.
It covers NVLink traffic between GPUs in an NVLink domain, and traffic between nodes that goes from Pods through RDMA NICs and switches.
It gives the results as Prometheus metrics, `RDMAEndpoint` resources, and Grafana dashboards.

In this document you learn:

- Which locations on the communication path traffic observability covers, and whether Unifabric supports each location now.
- How the data of each location is associated with one Pod identity.
- Where to find the detailed document of each observation method.

## Background

AI training jobs and inference jobs are, at their core, communication between GPUs.
The Pods of one job run on many nodes.
GPUs in the same NVLink domain communicate through NVLink.
GPU traffic outside the NVLink domain uses RDMA and goes through the RDMA NIC of the Pod, the RDMA NIC of the host, and switches.
The traffic that Pods use to read and write models, datasets, and KV cache also goes through RDMA NICs and switches.
Congestion or errors at any location on this path can make the full job slow.

But this path is almost a black box to the cloud-native stack:

- Usual network observability tools do not work.

  RDMA bypasses the kernel network stack. The NIC sends and receives the data directly.
  Packet capture, connection tracking, and tools that use the kernel network stack see only TCP/IP traffic. They do not see RDMA traffic.
  Only device-level total counters stay on the NIC.

- The data at each location is not connected, and it has no workload identity.

  Switch counters are in the network management platform. Host NIC counters are in the sysfs of the host.
  The SR-IOV VF of a Pod is in the network namespace of the Pod, so monitoring tools on the host do not see it.
  Pods in host mode share the host NIC, so the counters cannot tell which Pod the traffic belongs to.
  This data records only devices, for example `mlx5_0` or a switch port. It does not record Pods.
  Kubernetes knows Pods, but it cannot see this hardware.
  This is the "last mile" between the cloud-native stack and the high-speed network.

- The communication relations are a black box.

  You can know how much traffic a Pod sends, but you do not know where the traffic goes.
  When a job becomes slow, operators cannot answer "whose traffic uses the bandwidth, and which jobs does it affect".
  During performance tuning, they cannot make sure that the peers, the timing, and the message sizes are as expected.

The AI high-speed network traffic observability feature of Unifabric tries to solve these problems.
It collects traffic at each location on the communication path, and associates the data of each location with one Pod identity.
Operators can start from a workload and find which peers its Pods talk to, which NICs the traffic goes through, and if these NICs have congestion or errors.
Then they can find out if the network makes a job slow, and whose traffic affects whom.

## Prerequisites

- You deployed Unifabric according to the [basic installation](../../getting-started.md).
- Prometheus Operator is installed in the cluster.
- Only for Grafana dashboards: Grafana Operator (v5 or later) or the Grafana sidecar is installed in the cluster.

For the other requirements of each observation method, see the related document.

## Traffic observability scope

The diagram shows the communication path of AI applications, and the observation locations on the path.
GPUs in the same NVLink domain communicate through NVLink. An NVLink domain can span many nodes.
Traffic through the RDMA NICs starts when the application process in a Pod submits an RDMA request. It goes through the RDMA NIC of the Pod, the RDMA NIC of the host, and switches, to the Pod on the peer node.

![Traffic observability scope](../../images/traffic-observability-scope.png)

| Observation location | Observed data | Support status | Document |
| --- | --- | --- | --- |
| ① NVLink | NVLink traffic between GPUs in an NVLink domain | Planned | None |
| ② RDMA submit path | RDMA flows between Pods: who talks to whom, how much traffic, and how large each send is | Supported (Alpha) | [eBPF-based RDMA tracing](./rdma-flow.md), [RDMA send event storage](./rdma-flow-events.md) |
| ③ Host and Pod RDMA NICs | Throughput, congestion, errors, and retransmissions of the NICs | Supported (Stable) | [Host and Pod RDMA NIC metrics](./rdma-metrics.md) |
| ④ Switches | RDMA traffic on switch ports | Planned | None |

The traffic of the Scale-out network and the Storage network goes through ② to ④.
The traffic of the Scale-up network goes through ①.

### NVLink traffic observability

| Item | Description |
| --- | --- |
| Observed object | Traffic between GPUs in an NVLink domain through NVLink and NVLink Switch |
| Support status | Planned |

The size of an NVLink domain depends on the server:

- In an 8-GPU server, for example DGX B200, the NVLink domain is one node. NVLink traffic stays in the node.
- In a multi-node NVLink system, for example GB200 NVL72 and GB300 NVL72, NVLink Switches connect the 18 compute nodes of one rack into one NVLink domain with 72 GPUs. NVLink traffic goes between nodes.

NVLink traffic does not go through the RDMA NICs, so the NIC counters and the eBPF probes do not see it.
This item will collect the NVLink traffic between GPUs, and associate it with the Pods that use these GPUs.

> Note: Topology discovery records the NVLink domain as tier 1 of the Scale-up network. See [Topology discovery concepts](../../concepts/topology-discovery.md).

### Host and Pod RDMA NIC observability

| Item | Description |
| --- | --- |
| Observed object | The RDMA NICs of the host and the RDMA devices that Pods use |
| Data source | RDMA counters in sysfs, and ethtool priority counters |
| How it is associated with Pods | The Agent finds the Pods that use RDMA, and reads the counters of the RDMA devices in the Pods |
| Output | `unifabric_*` metrics, and the RDMA Cluster, Node, Pod, and AI Workload dashboards in Grafana |
| Support status | Supported (Stable) |
| Document | [Host and Pod RDMA NIC metrics](./rdma-metrics.md) |

This item reads the throughput, congestion (ECN, CNP), error and retransmission counters, and the pause and discard counters of each priority.
The `kind` label records the network of the NIC: `scaleOut`, `storage`, or `scaleUp`.
It tells you how much each NIC and each Pod sends and receives, and if there is congestion or errors. It does not tell you where the traffic goes.
[eBPF-based RDMA tracing](#ebpf-based-rdma-tracing) gives the peers.

### eBPF-based RDMA tracing

| Item | Description |
| --- | --- |
| Observed object | Each RDMA send that the CPU submits through `libibverbs` |
| Data source | eBPF uprobes on the submit path of `libibverbs` |
| How it is associated with Pods | Source Pod: the cgroup of the sending process. Destination Pod: the destination address of the QP. A RoCE network uses the GID. An InfiniBand network uses the LID. |
| Output | `rdma_flow_*` metrics and `RDMAEndpoint` resources. Optional: each send stored in ClickHouse or Elasticsearch. |
| Support status | Supported (Alpha) |
| Document | [eBPF-based RDMA tracing](./rdma-flow.md), [RDMA send event storage](./rdma-flow-events.md) |

RDMA traffic bypasses the kernel, and the NIC has only total counters.
This item attaches eBPF probes where the application submits RDMA requests. Without application changes, it rebuilds the RDMA flows between Pods: who talks to whom, when, and how large each send is.
It makes RDMA communication as observable as TCP connections.

Limits:

- It covers only CPU-initiated RDMA. GPU-initiated RDMA, for example GDAKI and IBGDA, and kernel-space RDMA storage protocols, for example NVMe over RDMA, do not use `libibverbs`, so the probes do not see them. The NIC counters still record this traffic.
- An InfiniBand network needs `agent.ebpfFlow.endpoint.allPods=true` to resolve the destination Pod.
- On the Storage network, the RDMA flows of user-space storage clients are observable, but this scenario is not verified yet.

### Switch RDMA traffic observability

| Item | Description |
| --- | --- |
| Observed object | RDMA traffic on switch ports in the Scale-out network and the Storage network |
| Support status | Planned. See [#49](https://github.com/unifabric-io/unifabric/issues/49) and [#3](https://github.com/unifabric-io/unifabric/pull/3). |

This item will collect traffic samples on switches, and associate the sampled RDMA traffic with Pods.
It does more than answer "which job does this traffic on the switch belong to".
Together with host and Pod RDMA NIC observability and eBPF-based RDMA tracing, it will connect the RDMA metrics of a Pod on the host NIC and on each switch along the path.
Then you can see the end-to-end path of the traffic: from the source Pod, through each switch and port, to the destination Pod. You can find on which segment of the path the congestion or errors occur.

> Note: Now you can use topology discovery to find the switch that a NIC connects to. Topology discovery records the switch domain of each node in Node labels. With LLDP discovery, `FabricNode` also records the switch and port that each NIC connects to. See [Topology discovery concepts](../../concepts/topology-discovery.md).

## Associate with the Pod identity

Each observation method above collects the data of one location on the communication path.
Unifabric adds the Pod identity to each record at collection time: the Pod, the namespace, and the training or inference job of the Pod.
So you can query the data of different locations with the same Pod or the same job.
In Grafana, select a Pod or a job to see its NIC traffic and its communication with other Pods together.

For how each observation method is associated with Pods, and what you can see in Grafana, see:

- [Host and Pod RDMA NIC metrics](./rdma-metrics.md#associate-with-pods-and-jobs)
- [eBPF-based RDMA tracing](./rdma-flow.md#view-the-results)

## References

- [Host and Pod RDMA NIC metrics](./rdma-metrics.md), [eBPF-based RDMA tracing](./rdma-flow.md), [RDMA send event storage](./rdma-flow-events.md): the principle, configuration, verification, and troubleshooting of each observation method.
- [Architecture](../../concepts/architecture.md): how the Agent, the Controller, and the other components relate.
- [Topology discovery concepts](../../concepts/topology-discovery.md): the switches that nodes and NICs connect to.
- [RDMA metric model and Pod attribution design](../../design/rdma-metrics.md), [eBPF RDMA flow attribution design](../../design/rdma-flow.md).
- [Metrics reference](../../reference/metrics.md), [RDMAEndpoint reference](../../reference/rdmaendpoint.md), [FabricNode reference](../../reference/fabricnode.md).
- [Helm values reference](../../../chart/README.md): all `nodeMetrics.*`, `agent.ebpfFlow.*`, `ebpfFlowEventCollector.*`, and `grafanaDashboard.*` parameters.
- [Roadmap](../../roadmap.md): planned features.
