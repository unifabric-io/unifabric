# RDMA Metrics

中文版: [rdma-metrics.zh.md](./rdma-metrics.zh.md)

## Introduction

RDMA metrics expose the RDMA state of every node as Prometheus metrics and Grafana
dashboards. The Unifabric Agent collects the metrics on each node and serves them on
its `/metrics` endpoint on port `8082`. The feature covers node-level RDMA device and
port counters, network interface attributes, ethtool priority counters, and
Pod-attributed metrics. Pod attribution for non-host RDMA Pods works only with the
containerd runtime. Other runtimes are skipped, and host-level metrics still work.

The metrics include:

- RDMA device and port counters, such as transmitted and received bytes, packets, and
  error counters.
- Network interface attributes, such as speed, MTU, port state, and RDMA device ToS.
- ethtool priority counters, such as pause and discard counters.
- Pod-attributed metrics, including namespace, Pod, top-level workload owner, and host
  RDMA mode.
- Topology dimension labels, such as `kind=scaleOut`, `kind=storage`, `is_root`, and
  `parent_ifname`.

Collection reads local Linux sysfs, containerd runtime state, and
`FabricNode.status.rdmaPods`. For the full metric model, see the
[RDMA metrics design](../design/rdma-metrics.md).

## Prerequisites

- Unifabric is installed through the [basic installation](../getting-started.md), and
  the scenario guide for your fabric is complete:
  [SONiC RoCE](../getting-started-sonic-roce.md),
  [Spectrum-X](../getting-started-spectrum-x.md), or
  [InfiniBand](../getting-started-infiniband.md).
- `nodeMetrics` is enabled in the Helm values. The scenario guides enable it.
- Prometheus Operator is installed. The chart renders a `ServiceMonitor` that
  Prometheus Operator consumes.
- A Grafana sidecar or Grafana Operator is installed. It imports the chart-rendered
  dashboards.

## Configuration and Usage

1. Make sure that the metrics Service and the ServiceMonitor exist:

   ```bash
   kubectl -n unifabric-system get service unifabric-agent-metrics
   kubectl -n unifabric-system get servicemonitor unifabric-agent-metrics
   ```

   ```text
   <!-- TODO: capture output from a real environment -->
   ```

2. Make sure that the Agent serves Unifabric metrics. Run this on a host that can
   reach Pod IPs:

   ```bash
   POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-agent -o jsonpath='{.items[0].status.podIP}')
   curl -s "http://${POD_IP}:8082/metrics" | grep '^unifabric_'
   ```

   ```text
   <!-- TODO: capture output from a real environment -->
   ```

3. Make sure that the Agent classified the RDMA NICs. The `kind` label on each metric
   names the network that the interface serves:

   ```bash
   curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="scaleOut"'
   curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="scaleUp"'
   curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="storage"'
   ```

   ```text
   <!-- TODO: capture output from a real environment -->
   ```

   The `fabricNode.*InterfaceSelector` Helm values set during installation decide the
   `kind` value of each interface.

   > **Note:** If no scale-out selector is configured and the Agent can identify the
   > interface, the Agent sets `kind` to `scaleOut`.

4. Open the dashboards. When `grafanaDashboard.enabled=true`, the chart renders four
   RDMA dashboards as `ConfigMap` or `GrafanaDashboard` resources, depending on
   `grafanaDashboard.kind`:

   - `rdma-cluster.json`: cluster-wide RDMA traffic and errors.
   - `rdma-node.json`: per-node device and port counters.
   - `rdma-pod.json`: Pod-attributed RDMA metrics.
   - `rdma-workload.json`: metrics grouped by the top-level workload owner.

   Each dashboard exists in English under `chart/files/dashboard-en/` and in
   Chinese under `chart/files/dashboard-zh/`. `grafanaDashboard.language` selects
   the installed set: `en` (default), `zh` (dashboard uid and resource name carry
   a `-zh` suffix), or `all` for both.

   Make sure that the dashboard resources exist:

   ```bash
   kubectl -n unifabric-system get configmap -l app.kubernetes.io/component=unifabric
   kubectl -n unifabric-system get grafanadashboard -l app.kubernetes.io/component=unifabric
   ```

   ```text
   <!-- TODO: capture output from a real environment -->
   ```

## Verification

Open the `rdma-node` dashboard in Grafana and select a node that has RDMA NICs. The
verification passes when the panels show device and port counter data for that node.

![Unifabric RDMA node dashboard](../images/unifabric-rdma-node-dashboard.png)

## Troubleshooting

### No RDMA Metrics

First make sure that the preconditions hold: the node has RDMA devices, and the Agent
runs on it.

1. Read the Agent logs:

   ```bash
   kubectl -n unifabric-system logs ds/unifabric-agent -c agent
   ```

2. Make sure that RDMA devices are visible on the node:

   ```bash
   kubectl -n unifabric-system exec ds/unifabric-agent -c agent -- ls /sys/class/infiniband
   ```

3. Make sure that the scenario identified the RDMA NICs. In the SONiC RoCE scenario,
   inspect the `FabricNode` resources:

   ```bash
   kubectl get fabricnodes
   kubectl get fabricnode <node-name> -o yaml
   ```

   The Spectrum-X and InfiniBand scenarios write Node labels through NVIDIA Topograph
   and do not depend on `FabricNode` or `Switch` resources. For these two scenarios,
   go directly to the next step.

4. Make sure that the Agent endpoint returns Unifabric metrics:

   ```bash
   POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-agent -o jsonpath='{.items[0].status.podIP}')
   curl -s "http://${POD_IP}:8082/metrics" | grep '^unifabric_'
   ```

   If the endpoint returns no `unifabric_` metrics, return to the Agent logs and the
   RDMA device check. If the endpoint returns metrics but Prometheus has none, inspect
   the `unifabric-agent-metrics` Service, its EndpointSlice, and the ServiceMonitor:

   ```bash
   kubectl -n unifabric-system get servicemonitor unifabric-agent-metrics -o yaml
   ```

### No Pod-Attributed Metrics

1. Make sure that `FabricNode.status.rdmaPods` contains the target Pod.
2. Make sure that the Pod uses SR-IOV RDMA devices or runs in host RDMA mode.
3. Make sure that the node runtime is containerd. Attribution for non-host RDMA Pods
   reads containerd container IDs. Other runtimes are skipped.

> **Note:** Missing or stale Pod container information does not block host-level RDMA
> metric export.

### The kind Label Is Empty

1. Inspect `fabricNode.scaleOutInterfaceSelector`,
   `fabricNode.storageInterfaceSelector`, and `fabricNode.scaleUpInterfaceSelector`
   in the Helm values. Selectors accept forms such as `interface=eth*,!eth9` and
   `cidr=172.17.0.0/16`.
2. If no scale-out selector is configured, the Agent sets identifiable interfaces to
   `scaleOut`. An interface that stays without `kind` was not identified. Make sure
   that a selector matches it.

### Dashboards Show No Data

1. Make sure that the Prometheus datasource can query `unifabric_node_info`.
2. Make sure that the dashboard variables, such as `cluster`, `node`, and
   `namespace`, match the current metric labels.

## References

- [RDMA metrics design](../design/rdma-metrics.md): the full metric model and label
  set.
- [FabricNode reference](../reference/fabricnode.md): the `status.rdmaPods` field that
  Pod attribution reads.
