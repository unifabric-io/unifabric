# Host and Pod RDMA NIC Metrics

> Maturity: Stable

中文版: [rdma-metrics.zh.md](./rdma-metrics.zh.md)

## Introduction

The Agent on each node collects the counters of the RDMA NICs and exports them as Prometheus metrics.
The metrics cover the RDMA NICs of the host and the RDMA devices that Pods use. They include throughput, congestion, errors and retransmissions, and pause and discards of each priority.
Each sample has a network type label. The samples of a Pod also have labels for the Pod and for the training or inference job that it belongs to.
This document tells you how to configure, verify, and view these metrics.

| Item | Description |
| --- | --- |
| Observed object | The RDMA NICs of the host and the RDMA devices that Pods use |
| Applicable networks | RDMA NICs in the Scale-out, Storage, and Scale-up networks |
| Data sources | RDMA counters and NIC attributes in sysfs, and ethtool priority counters |
| Dimensions | Node, NIC, Pod, workload |
| Output | `unifabric_*` metrics, and the RDMA Cluster, Node, Pod, and AI Workload dashboards in Grafana |
| Node side requirements | Linux nodes. RDMA devices are visible in `/sys/class/infiniband`. |
| Default state | Enabled by default |

For the definitions of all metrics and labels, see the [metrics reference](../../reference/metrics.md#host-and-pod-rdma-nic-metrics).

Limits:

- The metrics record how much traffic each NIC sends and receives. They do not record where the traffic goes. To see the peers, use [eBPF-based RDMA tracing](./rdma-flow.md).
- The metrics are the counter values at scrape time. The Agent does not keep history. Prometheus keeps the history.
- Only Pods that are identified as RDMA users have Pod samples. See [Identify the Pods that use RDMA](#identify-the-pods-that-use-rdma).
- A Pod in host mode uses the host NIC directly, so its samples are copies of the host NIC samples. When many Pods in host mode share one NIC, their samples are the same.

This item is part of AI high-speed network traffic observability. For the full scope, see [AI high-speed network traffic observability](./README.md#traffic-observability-scope).

## Prerequisites

- You meet the prerequisites in [AI high-speed network traffic observability](./README.md#prerequisites).
- The RDMA devices are visible on the node:

  ```bash
  kubectl -n unifabric-system exec ds/unifabric-agent -c agent -- ls /sys/class/infiniband
  ```

## Configuration

The metrics are enabled by default.
After the basic installation, the Agent already exports the metrics, and the chart already created the `ServiceMonitor` and the Grafana dashboards.
Change the configuration only in the scenarios below.

### Separate the network types

By default, the Agent classifies all RDMA NICs into the Scale-out network.
If the node has storage NICs or Scale-up NICs, set the related NIC selector at deployment.
For example, to classify `eth5` and `eth6` into the Storage network:

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set fabricNode.storageInterfaceSelector="interface=eth5\,eth6" \
  --wait
```

The `kind` label of the metrics records the result.
A selector can be `interface=eth*,!eth9` or `cidr=172.17.0.0/16`. For more, see `fabricNode.*` in the [Helm values reference](../../../chart/README.md).

> Note: These selectors also set the NIC classification for topology discovery.

### Configure Prometheus scraping

The chart creates the `ServiceMonitor` by default.
If Prometheus selects only `ServiceMonitor` objects with specific labels, add those labels with `nodeMetrics.serviceMonitor.labels` at deployment.
For the scrape interval and the other parameters, see `nodeMetrics.*` in the [Helm values reference](../../../chart/README.md).

### Configure the dashboards

By default, the chart installs the English dashboards, and Grafana Operator imports them.
To install the Chinese dashboards, to use the Grafana sidecar, or to select the target Grafana instance, see `grafanaDashboard.*` in the [Helm values reference](../../../chart/README.md).

## Associate with Pods and jobs

The Agent adds the Pod name, the namespace, and the training or inference job of the Pod to each Pod sample.
Then you can select a Pod or a job in Grafana and view its NIC traffic.
Most scenarios need no configuration.

### Identify the Pods that use RDMA

The Agent identifies these two types of Pods as RDMA users, and records them in `FabricNode.status.rdmaPods`:

- Pods that get RDMA devices through a CNI: the `k8s.v1.cni.cncf.io/network-status` annotation of the Pod contains `rdma-device`. For example, Pods that get a VF through Multus and the SR-IOV CNI.
- Pods in host mode: the Pod sets `hostNetwork: true`, and at least one container is privileged and mounts `/dev/infiniband`.

To see the Pods that the Agent found on a node:

```bash
kubectl get fabricnode node1 -o jsonpath='{range .status.rdmaPods[*]}{.namespace}{"\t"}{.name}{"\t"}{.hostRDMA}{"\t"}{.topOwner.kind}/{.topOwner.name}{"\n"}{end}'
```

<!-- TODO: capture output from a real environment -->

> Note: Pods that get RDMA devices through DRA, for example with DraNet, are not identified yet. See [#47](https://github.com/unifabric-io/unifabric/issues/47).

> Note: Pods that get RDMA devices through a CNI need containerd on the node. Pods on other container runtimes have no Pod samples. The host NIC samples are not affected.

### Identify the job of a Pod

The Pods of training jobs and inference jobs are usually not created directly. Higher-level objects create them step by step.
For example, a `PyTorchJob` creates Pods. A `Deployment` creates a `ReplicaSet`, and the `ReplicaSet` creates Pods.
The `ownerReferences` of each object records the object that created it.
The Agent follows this chain up, and writes the top object to the `topowner_*` labels as the job of the Pod.

The Agent reads each object in the chain, so it needs read permission on these resource types.
By default, the chart already gives read permission on these resource types. Jobs that they create need no configuration:

- Kubernetes built-in resources: `Deployment`, `ReplicaSet`, `StatefulSet`, `DaemonSet`, `Job`, `CronJob`.
- Common AI job controllers: the resources of Kubeflow Training Operator, Kubeflow Trainer, JobSet, LeaderWorkerSet, KubeRay, Spark Operator, and Volcano.

If another controller creates the job, for example Argo Workflows, the Agent cannot read its objects, and the `topowner_*` labels are empty.
Then give read permission with the Helm parameter `agent.workloadOwnerRules` at deployment.
The key is the API group of the controller. The value is the plural names of the resource types.
The values that you add are merged with the default values.
For example, to give read permission on the `Workflow` resources of Argo Workflows:

```yaml
agent:
  workloadOwnerRules:
    argoproj.io: [workflows]
```

## Verification

1. Make sure that the Service and the `ServiceMonitor` exist:

   ```bash
   kubectl -n unifabric-system get service unifabric-agent-metrics
   kubectl -n unifabric-system get servicemonitor unifabric-agent-metrics
   ```

2. In one terminal, start a port forward to an Agent Pod:

   ```bash
   AGENT_POD=$(kubectl -n unifabric-system get pod \
     -l app.kubernetes.io/component=unifabric-agent \
     --field-selector=status.phase=Running \
     -o jsonpath='{.items[0].metadata.name}')
   kubectl -n unifabric-system port-forward "pod/$AGENT_POD" 8082:8082
   ```

3. In a second terminal, make sure that the NIC classification is correct:

   ```bash
   curl -s http://127.0.0.1:8082/metrics | grep '^unifabric_port_xmit_data' | grep 'scope="host"'
   ```

   <!-- TODO: capture output from a real environment -->

   The `kind` of each NIC must agree with the selector configuration.

4. If the node has RDMA Pods, make sure that Pod-level samples exist:

   ```bash
   curl -s http://127.0.0.1:8082/metrics | grep '^unifabric_port_xmit_data' | grep 'scope="pod"'
   ```

   <!-- TODO: capture output from a real environment -->

5. Make sure that the dashboard resources exist:

   ```bash
   kubectl -n unifabric-system get grafanadashboard -l app.kubernetes.io/component=unifabric
   ```

   If `grafanaDashboard.kind=ConfigMap`, run:

   ```bash
   kubectl -n unifabric-system get configmap -l app.kubernetes.io/component=unifabric
   ```

## View the dashboards

Open Grafana. Start from the Unifabric RDMA Cluster dashboard, then go down to a node, a Pod, or a job.

### Unifabric RDMA Cluster

The overview of the cluster. It has these rows:

- Summary: the number of nodes, workloads, and Pods that use RDMA, and the read and write throughput and throughput usage of the cluster and of each node.
- Hotspot: the 10 nodes, Pods, and workloads with the highest read and write throughput.
- Node Status: pause, discards, ECN marks, CNP, and out-of-sequence packets of the host NICs on each node.
- Pod Status: the 10 Pods with the most out-of-sequence packets and discards.

Use the Hotspot row to find the jobs that use the most bandwidth. Use the Node Status row to find the nodes with congestion.

![Unifabric RDMA Cluster dashboard](../../images/cluster.png)

### Unifabric RDMA Node

The details of each NIC on one node. It has these rows:

- Throughput: throughput usage of each device, and read and write throughput of host devices and Pod devices.
- Basic (ECN/CNP/SEQ): pause and discards of each priority, and ECN marks, CNP, and out-of-sequence packets of each device.
- Detail: received Write, Read, and Atomic requests, and the error counters.
- RoCE: RoCE adaptive retransmission and slow restart.
- QoS: the traffic class of each device.
- Interface: the list of network devices.

Use it to find out if a NIC has congestion or errors.

![Unifabric RDMA Node dashboard](../../images/nodes.png)

### Unifabric RDMA Pod

Select a namespace and a Pod to view the data of the RDMA devices of this Pod:

- Summary: read and write throughput, throughput usage, and unicast and multicast packets.
- RDMA basic statistics: received requests, out-of-sequence packets, duplicate requests, and retries.
- RDMA error statistics: the errors on the requester side and the responder side.

Use it to find out if congestion or errors affect the communication of a Pod.
The "RDMA send details (CPU initiated)" row on the same dashboard comes from [eBPF-based RDMA tracing](./rdma-flow.md#view-the-results).

![Unifabric RDMA Pod dashboard](../../images/pods.png)

You can also query by Pod in Prometheus. For example, the send throughput of one Pod:

```promql
sum(rate(unifabric_port_xmit_data{pod_namespace="tenant-a", pod_name="vllm-decode-0"}[5m]))
```

### Unifabric RDMA AI Workload

Select the kind, namespace, and name of a job, for example a `PyTorchJob`, to view the summary of all Pods of this job:

- Summary: the read and write throughput of the job.
- Pods: the throughput, packets, out-of-sequence packets, and sequence errors of each Pod.

Use it to find the Pod with the most traffic or the most errors in a job.
The "RDMA send details (CPU initiated)" row on the same dashboard comes from [eBPF-based RDMA tracing](./rdma-flow.md#view-the-results).

![Unifabric RDMA AI Workload dashboard](../../images/workloads.png)

## Troubleshooting

### There are no RDMA metrics

1. Make sure that the Agent runs correctly:

   ```bash
   kubectl -n unifabric-system get daemonset unifabric-agent
   kubectl -n unifabric-system logs ds/unifabric-agent -c agent --tail 200
   ```

2. Make sure that the RDMA devices are visible on the node:

   ```bash
   kubectl -n unifabric-system exec ds/unifabric-agent -c agent -- ls /sys/class/infiniband
   ```

   If the directory is empty, check the NIC driver and the RDMA kernel modules.

3. Make sure that the `FabricNode` of the node exists:

   ```bash
   kubectl get fabricnodes
   ```

   If the `FabricNode` does not exist, the Agent returns empty metrics.

4. If the Agent has metrics locally but Prometheus has no data, see [Prometheus has no metrics](#prometheus-has-no-metrics).

### Prometheus has no metrics

1. Make sure that the Agent returns metrics locally. See [Verification](#verification).

2. Make sure that the Service and the `ServiceMonitor` exist:

   ```bash
   kubectl -n unifabric-system get service,servicemonitor \
     -l app.kubernetes.io/component=unifabric-agent
   ```

3. On the Targets page of Prometheus, make sure that the scrape target of `unifabric-agent-metrics` is `UP`.
   If Prometheus selects `ServiceMonitor` objects by label, add those labels to `nodeMetrics.serviceMonitor.labels`.

### There are no Pod-level metrics

1. Make sure that the Pod is in `FabricNode.status.rdmaPods`. See [Identify the Pods that use RDMA](#identify-the-pods-that-use-rdma).
2. If the Pod is not in the list, make sure that it meets the conditions.
3. If the Pod is in the list but has no Pod-level samples, make sure that the node uses containerd.

### Workload labels are empty or stop at an intermediate resource

- The `topowner_*` labels are empty: give read permission on the controller of the job. See [Identify the job of a Pod](#identify-the-job-of-a-pod).
- The `*_top_owner_kind` of `rdma_flow_*` stops at a Kubernetes built-in resource, for example `ReplicaSet`: make sure that the Agent can read the resource at the next level.
- The `*_top_owner_kind` of `rdma_flow_*` is a third-party resource, but it is not the top level: RDMA tracing follows only the resource types in [owners.go](../../../pkg/agent/ebpfflow/owners.go). Add the type to this file.

### The kind label is empty or wrong

- Check `fabricNode.scaleOutInterfaceSelector`, `fabricNode.storageInterfaceSelector`, and `fabricNode.scaleUpInterfaceSelector`.
- If `scaleOutInterfaceSelector` is set, a NIC that matches no selector has an empty `kind`.
- The selectors match `parent_ifname` first. For a VF, use the PF name in the selector.

### The dashboards have no data

1. In Grafana Explore, query `unifabric_node_info` to make sure that the Prometheus data source has Unifabric metrics.
2. Make sure that the `cluster`, `node`, `namespace`, and other variables of the dashboard select objects that make RDMA traffic.
3. Make sure that there is RDMA traffic in the time range of the dashboard.

## References

- [AI high-speed network traffic observability](./README.md): the full scope.
- [eBPF-based RDMA tracing](./rdma-flow.md): see the peers of RDMA traffic.
- [Metrics reference](../../reference/metrics.md): all metrics and labels.
- [RDMA metric model and Pod attribution design](../../design/rdma-metrics.md): the data sources, the collection flow, and the metric model.
- [FabricNode reference](../../reference/fabricnode.md): the `status.rdmaPods` field.
- [Helm values reference](../../../chart/README.md): all `nodeMetrics.*`, `fabricNode.*`, `agent.workloadOwnerRules`, and `grafanaDashboard.*` parameters.
