# Spectrum-X Fabric

中文版: [getting-started-spectrum-x.zh.md](./getting-started-spectrum-x.zh.md)

This guide explains how to deploy Unifabric in a Spectrum-X switch cluster. This scenario gets fabric topology through the NetQ API.

## Deployment Goals

After deployment, the cluster should achieve two goals:

- Nodes are labeled with topology labels consumed by schedulers, including `unifabric.io/scale-up`, `unifabric.io/scale-out-leaf`, `unifabric.io/scale-out-spine`, and `unifabric.io/scale-out-core`.
- Node RDMA state is observable through Unifabric Agent metrics, including RDMA device, port, priority, and Pod attribution metrics.

> This scenario does not create `FabricNode` or `Switch` CRs for Unifabric switch-driven discovery.

## Prerequisites

- A Kubernetes cluster with target GPU nodes.
- `kubectl` and Helm 3 installed.
- RDMA devices visible under `/sys/class/infiniband` on the nodes.
- The Spectrum-X fabric is already managed by NetQ, and NetQ already contains the corresponding fabric topology data.
  - If NetQ is not deployed but the network is an IB network, see [InfiniBand fabric](./getting-started-infiniband.md).
  - If the network is a RoCE network, see [General SONiC RoCE](./getting-started-sonic-roce.md).
- The cluster can reach the NetQ API.
- NetQ username, password, and API URL have been prepared.
- Prometheus Operator and Grafana Operator are installed in the cluster. If they are not installed, disable
  `ServiceMonitor` and `GrafanaDashboard` when installing Unifabric to avoid CRD creation failures.

Verify cluster access:

```bash
kubectl cluster-info
kubectl get nodes -o wide
```

## Prepare NetQ Credentials

Create a Secret that contains `credentials.yaml`:

```bash
kubectl create namespace unifabric-system --dry-run=client -o yaml | kubectl apply -f -
kubectl -n unifabric-system create secret generic netq-credentials \
  --from-file=credentials.yaml=./netq-credentials.yaml
```

`netq-credentials.yaml` content:

```yaml
username: <netq-user>
password: <netq-password>
```

The Secret is mounted as `/etc/topograph/credentials/credentials.yaml` by default. Set `nvidiaTopograph.topograph.config.credentialsPath` only when a non-default file name or path is needed.

## Install Unifabric

The following command uses the latest release. The example leaves RDMA interface selectors empty, so all RDMA NICs are observed by metrics. Spectrum-X topology labels are written by NVIDIA topograph through the NetQ provider.

```bash
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unifabric-io/unifabric/releases/latest | grep '"tag_name":' | cut -d '"' -f4)
CHART_VERSION="${LATEST_TAG#v}"

helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --version "${CHART_VERSION}" \
  --namespace unifabric-system \
  --create-namespace \
  --set nvidiaTopograph.enable=true \
  --set nvidiaTopograph.provider.name=netq \
  --set-string nvidiaTopograph.provider.params.apiUrl=https://netq.example.com \
  --set nvidiaTopograph.topograph.config.credentialsSecret=netq-credentials \
  --set-string nodeTopologyDiscovery.scaleUpInterfaceSelector="" \
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
| `nvidiaTopograph.enable` | Enables NVIDIA topograph. Must be `true` for Spectrum-X. |
| `nvidiaTopograph.provider.name` | Set to `netq` to discover topology through the NetQ API. |
| `nvidiaTopograph.provider.params.apiUrl` | NetQ API URL. |
| `nvidiaTopograph.topograph.config.credentialsSecret` | Secret name that contains `credentials.yaml`. |
| `nvidiaTopograph.topograph.config.credentialsPath` | Optional: non-default credentials path. |

| `nodeMetrics.enabled` | Enables Agent metrics for node RDMA observability. |
| `nodeTopologyDiscovery.scaleUpInterfaceSelector` | Selects specific RDMA NICs for observation and labels them with `kind=scaleOut` in RDMA metrics. Supports `interface=eth*,mlx*` or `cidr=172.17.0.0/16`. Defaults to all RDMA NICs. |
| `nodeTopologyDiscovery.storageInterfaceSelector` | Selects storage RDMA NICs and labels them with `kind=storage` in RDMA metrics. Supports `interface=eth*,mlx*` or `cidr=172.17.0.0/16`. Defaults to empty. |
| `nodeMetrics.serviceMonitor.enabled` | Creates the Prometheus Operator `ServiceMonitor`. |
| `grafanaDashboard.enabled` | Renders the built-in RDMA dashboards. |

For more Helm parameters, see [chart/README.md](../chart/README.md).

## Verify the Deployment

Check topograph components, NetQ provider configuration, and Node labels:

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

Check RDMA metric NIC classification:

```bash
curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="scaleUp"'
```

## Troubleshooting

### Node Labels Are Not Written

- Confirm that NVIDIA topograph and node-observer are running and have permission to update Nodes.
- Confirm that `nvidiaTopograph.provider.name=netq`.
- Confirm that `nvidiaTopograph.provider.params.apiUrl=<netq-url>` is correct.
- Confirm that the NetQ account can access the target premises and that NetQ has fabric topology data.
- Confirm that the Secret referenced by `nvidiaTopograph.topograph.config.credentialsSecret` exists and contains `credentials.yaml`.
- If `topologyLabels.*` Helm values are customized, scheduler label keys must be updated accordingly.

### RDMA Metrics Do Not Include RDMA NICs

- Confirm that the Agent Pod is running and RDMA devices are visible under `/sys/class/infiniband` on the node.
- Confirm that `nodeTopologyDiscovery.scaleUpInterfaceSelector` matches the RDMA NIC name or CIDR.
- Confirm that `nodeMetrics.enabled=true`.
- If using Prometheus Operator, confirm that `nodeMetrics.serviceMonitor.enabled=true` and the `ServiceMonitor` is selected by Prometheus.

### topograph Cannot Reach NetQ

- Check the NetQ API URL, certificates, and network connectivity.
- Check topograph / node-observer logs.
- Confirm that the topograph Service is reachable.

## Uninstall

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

## Next Steps

- Return to the [documentation index](./README.md).
- Read the [Kueue TAS workload example](./usage/workload-tas.md).
- See the [Helm values reference](../chart/README.md).
