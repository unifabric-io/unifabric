# InfiniBand Fabric

中文版: [getting-started-infiniband.zh.md](./getting-started-infiniband.zh.md)

This guide explains how to deploy Unifabric in an InfiniBand NIC cluster. This scenario is for IB networking, such as Mellanox NICs in IB mode with IB switches.

## Deployment Goals

After deployment, the cluster should achieve two goals:

- Nodes are labeled with topology labels consumed by schedulers, including `unifabric.io/scale-up`, `unifabric.io/scale-out-leaf`, `unifabric.io/scale-out-spine`, and `unifabric.io/scale-out-core`.
- Node RDMA state is observable through Unifabric Agent metrics and the built-in RDMA Grafana dashboards, with throughput, utilization, QoS, congestion, and error metrics grouped by cluster, node, Pod, and workload.

> This scenario does not create `FabricNode` or `Switch` CRs for Unifabric switch-driven discovery.

## Prerequisites

- A Kubernetes cluster with target GPU nodes.
- `kubectl` and Helm 3 installed.
- InfiniBand / RDMA devices visible under `/sys/class/infiniband` on the nodes.
- GPU Operator and NVIDIA device plugin are deployed.
- node-data-broker can run on target GPU nodes. By default it uses the GPU Operator clique label;
  `pods/exec` permission is only needed when `nvidiaTopograph.provider.params.useGpuCliqueLabel` is disabled.
- Prometheus Operator and Grafana Operator are installed in the cluster. If they are not installed, disable
  `ServiceMonitor` and `GrafanaDashboard` when installing Unifabric to avoid CRD creation failures.

Verify cluster access:

```bash
kubectl cluster-info
kubectl get nodes -o wide
```

## Install Unifabric

The following command uses the latest release. The example leaves RDMA interface selectors empty, so all RDMA NICs are observed by metrics. InfiniBand topology labels are written by NVIDIA topograph.

```bash
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unifabric-io/unifabric/releases/latest | grep '"tag_name":' | cut -d '"' -f4)
CHART_VERSION="${LATEST_TAG#v}"

helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --version "${CHART_VERSION}" \
  --namespace unifabric-system \
  --create-namespace \
  --set nvidiaTopograph.enable=true \
  --set nvidiaTopograph.provider.name=infiniband-k8s \
  --set internalTopologyLabelWriter.enabled=false \
  --set-string nodeTopologyDiscovery.scaleOutInterfaceSelector="" \
  --set-string nodeTopologyDiscovery.storageInterfaceSelector="" \
  --set nodeMetrics.enabled=true \
  --set nodeMetrics.serviceMonitor.enabled=true \
  --set grafanaDashboard.enabled=true \
  --wait
```

Parameters:

| Helm value | Purpose |
| --- | --- |
| `nvidiaTopograph.enable` | Enables NVIDIA topograph. Must be `true` for InfiniBand networking. |
| `nvidiaTopograph.provider.name` | Set to `infiniband-k8s` to use the InfiniBand Kubernetes provider. |
| `nvidiaTopograph.provider.params.useGpuCliqueLabel` | Defaults to `true` and uses the GPU Operator clique label as the accelerator topology source. Set it to `false` only when node-data-broker should discover topology through `pods/exec`. |
| `internalTopologyLabelWriter.enabled` | Set to `false` to prevent the built-in Unifabric writer and NVIDIA topograph from both writing topology Node labels. |
| `nodeMetrics.enabled` | Enables Agent metrics for node RDMA observability. |
| `nodeTopologyDiscovery.scaleOutInterfaceSelector` | Selects RDMA NICs included in scale-out topology and RDMA metrics, and labels them with `kind=scaleOut` in RDMA metrics. Supports `interface=ib*,mlx*` or `cidr=172.17.0.0/16`. Defaults to all RDMA NICs. |
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
- If `nvidiaTopograph.provider.params.useGpuCliqueLabel` is disabled, confirm that `ibnetdiscover` is available.
- If `topologyLabels.*` Helm values are customized, scheduler label keys must be updated accordingly.

### RDMA Metrics Do Not Include IB NICs

- Confirm that the Agent Pod is running and IB devices are visible under `/sys/class/infiniband` on the node.
- Confirm that `nodeTopologyDiscovery.scaleOutInterfaceSelector` matches the IB NIC name or CIDR.
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
