# InfiniBand Fabric

中文版: [getting-started-infiniband.zh.md](./getting-started-infiniband.zh.md)

This guide explains how to deploy Unifabric in an InfiniBand NIC cluster. This scenario is for IB networking, such as Mellanox NICs in IB mode with IB switches.

## Deployment Goals

After deployment, the cluster should achieve two goals:

- Nodes are labeled with topology labels consumed by schedulers, including `unifabric.io/scale-up`, `unifabric.io/scale-out-leaf`, `unifabric.io/scale-out-spine`, and `unifabric.io/scale-out-core`.
- Node RDMA state is observable through Unifabric Agent metrics, including RDMA device, port, priority, and Pod attribution metrics.

> This scenario does not create `FabricNode` CRs and does not create or update `ScaleOutLeafGroup` CRs.

## Prerequisites

- A Kubernetes cluster with target GPU nodes.
- `kubectl` and Helm 3 installed.
- InfiniBand / RDMA devices visible under `/sys/class/infiniband` on the nodes.
- GPU Operator and NVIDIA device plugin are deployed.
- node-data-broker can run on target nodes and has the required `pods/exec` permission.
- Prometheus Operator and Grafana Operator are installed in the cluster. If they are not installed, disable
  `ServiceMonitor` and `GrafanaDashboard` when installing Unifabric to avoid CRD creation failures.

Verify cluster access:

```bash
kubectl cluster-info
kubectl get nodes -o wide
```

## Install Unifabric

The following command uses the latest release. The example leaves RDMA interface selectors empty, so all RDMA NICs are observed by metrics; it also disables Unifabric's own leaf group path so Unifabric Agent / Controller do not write topology labels through LLDP/FabricNode. InfiniBand topology labels are written by NVIDIA topograph.

```bash
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unifabric-io/unifabric/releases/latest | grep '"tag_name":' | cut -d '"' -f4)
CHART_VERSION="${LATEST_TAG#v}"

helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --version "${CHART_VERSION}" \
  --namespace unifabric-system \
  --create-namespace \
  --set nvidiaTopograph.enable=true \
  --set nvidiaTopograph.provider.name=infiniband-k8s \
  --set-string nodeTopologyDiscovery.scaleOutInterfaceSelector="" \
  --set-string nodeTopologyDiscovery.storageInterfaceSelector="" \
  --set scaleOutDiscovery.leafGroups.enabled=false \
  --set nodeMetrics.enabled=true \
  --set nodeMetrics.serviceMonitor.enabled=true \
  --set grafanaDashboard.enabled=true \
  --wait
```

Parameters:

| Helm value | Purpose |
| --- | --- |
| `nvidiaTopograph.enable` | Enables NVIDIA topograph. Must be `true` for InfiniBand IB networking. |
| `nvidiaTopograph.provider.name` | Set to `infiniband-k8s`, which discovers topology with `ibnetdiscover`. |

| `scaleOutDiscovery.leafGroups.enabled` | Disables Unifabric `ScaleOutLeafGroup` and leaf Node label write-back. |
| `nodeMetrics.enabled` | Enables Agent metrics for node RDMA observability. |
| `nodeTopologyDiscovery.scaleUpInterfaceSelector` | Selects specific RDMA NICs for observation and labels them with `kind=scaleOut` in RDMA metrics. Supports `interface=ib*,mlx*` or `cidr=172.17.0.0/16`. Defaults to all RDMA NICs. |
| `nodeTopologyDiscovery.storageInterfaceSelector` | Selects storage RDMA NICs and labels them with `kind=storage` in RDMA metrics. Supports `interface=ib*,mlx*` or `cidr=172.17.0.0/16`. Defaults to empty. |
| `nodeMetrics.serviceMonitor.enabled` | Creates the Prometheus Operator `ServiceMonitor`. |
| `grafanaDashboard.enabled` | Renders the built-in RDMA dashboards. |

For more Helm parameters, see [chart/README.md](../chart/README.md).

## Verify the Deployment

Check topograph components, node-data-broker DaemonSet, Node annotation, and Node labels:

```bash
kubectl -n unifabric-system get pods
kubectl get pods -n unifabric-system -o wide
kubectl get nodes -L unifabric.io/scale-up,unifabric.io/scale-out-core,unifabric.io/scale-out-spine,unifabric.io/scale-out-leaf,kubernetes.io/hostname
```

When configuring Kueue, Volcano, or KAI Scheduler, use only labels that are actually written to Nodes.

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

## Troubleshooting

### Node Labels Are Not Written

- Confirm that NVIDIA topograph, node-observer, and node-data-broker are running and have permission to update Nodes.
- Confirm that node-data-broker Pods run on target GPU nodes.
- Confirm that corresponding nodes have the `topograph.nvidia.com/cluster-id` annotation.
- Confirm that `ibnetdiscover` is available.
- If `topologyLabels.*` Helm values are customized, scheduler label keys must be updated accordingly.

### RDMA Metrics Do Not Include IB NICs

- Confirm that the Agent Pod is running and IB devices are visible under `/sys/class/infiniband` on the node.
- Confirm that `nodeTopologyDiscovery.scaleUpInterfaceSelector` matches the IB NIC name or CIDR.
- Confirm that `nodeMetrics.enabled=true`.
- If using Prometheus Operator, confirm that `nodeMetrics.serviceMonitor.enabled=true` and the `ServiceMonitor` is selected by Prometheus.

## Uninstall

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

## Next Steps

- Return to the [documentation index](./README.md).
- Read the [Kueue TAS workload example](./usage/workload-tas.md).
- See the [Helm values reference](../chart/README.md).
