# RDMA Observability Usage Guide

中文版: [rdma-metrics.zh.md](./rdma-metrics.zh.md)

This guide explains how to use Unifabric RDMA observability, including verifying
Prometheus metrics, checking NIC classification, and viewing Grafana dashboards.

## Feature Overview

Unifabric Agent collects RDMA-related metrics on each node and exposes them
through the controller-runtime Prometheus `/metrics` endpoint. Metrics include:

- RDMA device and port counters, such as transmitted and received bytes, packets, and error counters.
- Network interface attributes, such as speed, MTU, port state, and RDMA device ToS.
- ethtool priority counters, such as pause and discard counters.
- Pod-attributed metrics, including namespace, Pod, top-level workload owner, and host RDMA mode.
- Topology dimension labels, such as `kind=scaleOut`, `kind=storage`, `is_root`, and `parent_ifname`.

Agent collection depends on local Linux sysfs, containerd runtime state, and
`FabricNode.status.rdmaPods`. For the full metric model, see
[RDMA metrics design](../design/rdma-metrics.md).

## Installation Entry Points

Before using RDMA metrics, complete the Unifabric installation for your fabric
scenario and enable `nodeMetrics` and dashboards in the installation guide. See
Get Started in the [documentation index](../README.md) for installation steps.

Scenario entry points:

| Scenario | Installation guide |
| --- | --- |
| General SONiC RoCE | [General SONiC RoCE](../getting-started-sonic-roce.md) |
| Spectrum-X fabric | [Spectrum-X fabric](../getting-started-spectrum-x.md) |
| InfiniBand fabric | [InfiniBand fabric](../getting-started-infiniband.md) |

Prometheus Operator must be installed to use the chart-rendered
`ServiceMonitor`. Grafana sidecar or Grafana Operator must be installed to
automatically import chart-rendered dashboards.

## Verify Metrics

Verify RDMA metrics resources:

```bash
kubectl -n unifabric-system get service unifabric-agent-metrics
kubectl -n unifabric-system get servicemonitor unifabric-agent-metrics
```

Check the Agent metrics endpoint directly:

```bash
POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-agent -o jsonpath='{.items[0].status.podIP}')
curl -s "http://${POD_IP}:8082/metrics" | grep '^unifabric_'
```

Check RDMA metric NIC classification:

```bash
curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="scaleOut"'
curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="scaleUp"'
curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="storage"'
```

The `kind` label is determined by the
`nodeTopologyDiscovery.*InterfaceSelector` values configured during
installation. If no scale-out selector is configured and the interface can be
identified, the Agent defaults the interface to `scaleOut`.

## Dashboard

The chart includes these RDMA dashboard files:

- `rdma-cluster.json`
- `rdma-node.json`
- `rdma-pod.json`
- `rdma-workload.json`

When `grafanaDashboard.enabled=true`, the chart renders these files as
`ConfigMap` or `GrafanaDashboard` resources depending on
`grafanaDashboard.kind`.

Verify dashboard resources:

```bash
kubectl -n unifabric-system get configmap -l app.kubernetes.io/component=unifabric
kubectl -n unifabric-system get grafanadashboard -l app.kubernetes.io/component=unifabric
```

If dashboards show no data, first confirm that the Prometheus data source can
query `unifabric_node_info`, then check whether dashboard variables such as
`cluster`, `node`, and `namespace` match the current metric labels.

## Troubleshooting

If there are no RDMA metrics:

1. Confirm that the Agent is running:

   ```bash
   kubectl -n unifabric-system logs ds/unifabric-agent -c agent
   ```

2. Confirm that RDMA devices are visible on the node:

   ```bash
   kubectl -n unifabric-system exec ds/unifabric-agent -c agent -- ls /sys/class/infiniband
   ```

3. Confirm that RDMA NICs are identified for the scenario:

   ```bash
   # SONiC RoCE / LLDP scenario
   kubectl get fabricnodes
   kubectl get fabricnode <node-name> -o yaml
   ```

   Spectrum-X and InfiniBand scenarios use NVIDIA topograph to write Node
   labels and do not depend on `FabricNode` / `ScaleOutLeafGroup`. For these
   two scenarios, check the Agent metrics endpoint and `/sys/class/infiniband`
   first.

4. Confirm that the Agent Pod metrics endpoint returns Unifabric metrics directly:

   ```bash
   POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-agent -o jsonpath='{.items[0].status.podIP}')
   curl -s "http://${POD_IP}:8082/metrics" | grep '^unifabric_'
   ```

   If there are no `unifabric_` metrics here, debug Agent logs and RDMA device
   visibility first. If metrics exist here but not in Prometheus, check the
   `unifabric-agent-metrics` Service, EndpointSlice, and ServiceMonitor.

5. If using `ServiceMonitor`, confirm that Prometheus can discover the target:

   ```bash
   kubectl -n unifabric-system get servicemonitor unifabric-agent-metrics -o yaml
   ```

If there are no Pod-attributed metrics:

- Confirm that `FabricNode.status.rdmaPods` contains the target Pod.
- Confirm that the Pod uses SR-IOV RDMA devices or runs in host RDMA mode.
- Current non-host RDMA Pod attribution depends on containerd container IDs; non-containerd runtimes are skipped.
- Missing or stale Pod container information does not block host-level RDMA metric export.

If the `kind` label is empty:

- Check `nodeTopologyDiscovery.scaleOutInterfaceSelector`, `nodeTopologyDiscovery.storageInterfaceSelector`, and `nodeTopologyDiscovery.scaleUpInterfaceSelector`.
- Selectors support `interface=eth*,!eth9` and `cidr=172.17.0.0/16`.
- If no scale-out selector is configured and the interface is identifiable, the Agent defaults the interface to `scaleOut`.
