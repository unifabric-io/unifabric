# Spectrum-X Fabric

中文版: [getting-started-spectrum-x.zh.md](./getting-started-spectrum-x.zh.md)

This guide explains how to deploy Unifabric in a Spectrum-X switch cluster. This scenario gets fabric topology through the NetQ API.

## Deployment Goals

After deployment, the cluster should achieve two goals:

- Nodes receive the `scale-up.unifabric.io/tier-N` and
  `scale-out.unifabric.io/tier-N` topology labels used by schedulers.
- Node RDMA state is observable through Unifabric Agent metrics, including RDMA device, port, priority, and Pod attribution metrics.

## Prerequisites

- A Kubernetes cluster with target GPU nodes.
- `kubectl` and Helm 3 installed.
- RDMA devices visible under `/sys/class/infiniband` on the nodes.
- The Spectrum-X fabric is already managed by NetQ, and NetQ already contains the corresponding fabric topology data.
  - If NetQ is not deployed but the network is an IB network, see [InfiniBand fabric](./getting-started-infiniband.md).
  - If the network is a RoCE network, see [General SONiC RoCE](./getting-started-sonic-roce.md).
- The cluster can reach the NetQ API.
- The NetQ API URL, username, and password have been prepared.
- Prometheus Operator and Grafana Operator are installed in the cluster. If they are not installed, disable
  `ServiceMonitor` and `GrafanaDashboard` when installing Unifabric to avoid CRD creation failures.

Verify cluster access:

```bash
kubectl cluster-info
kubectl get nodes -o wide
```

## Install Unifabric

The following command uses the latest release. The example leaves RDMA interface selectors empty, so all RDMA NICs are observed by metrics. Spectrum-X topology labels are written by NVIDIA topograph through the NetQ provider.

Create the namespace and NetQ credentials Secret first. The following commands
do not write credentials to a local file, Helm values, or a ConfigMap. The
Secret must contain a key named `credentials.yaml`:

```bash
kubectl create namespace unifabric-system --dry-run=client -o yaml | kubectl apply -f -

read -r -p "NetQ username: " NETQ_USERNAME
read -r -s -p "NetQ password: " NETQ_PASSWORD
printf '\n'
printf 'username: %s\npassword: %s\n' "${NETQ_USERNAME}" "${NETQ_PASSWORD}" |
  kubectl -n unifabric-system create secret generic netq-credentials \
    --from-file=credentials.yaml=/dev/stdin
unset NETQ_USERNAME NETQ_PASSWORD
```

Production clusters should also enable encryption at rest for Kubernetes
Secrets and restrict RBAC access to this Secret.

```bash
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unifabric-io/unifabric/releases/latest | grep '"tag_name":' | cut -d '"' -f4)
CHART_VERSION="${LATEST_TAG#v}"

helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --version "${CHART_VERSION}" \
  --namespace unifabric-system \
  --create-namespace \
  --set topoDiscovery.scaleUp.mode=nv-topograph \
  --set topoDiscovery.scaleOut.mode=nv-topograph \
  --set topoDiscovery.storage.mode=unifabric-roce \
  --set nvidiaTopograph.provider.name=netq \
  --set-string nvidiaTopograph.provider.params.apiUrl=https://netq.example.com \
  --set-string nvidiaTopograph.credentialsSecretName=netq-credentials \
  --set-string fabricNode.scaleUpInterfaceSelector="" \
  --set-string fabricNode.scaleOutInterfaceSelector="" \
  --set-string fabricNode.storageInterfaceSelector="" \
  --set nodeMetrics.enabled=true \
  --set nodeMetrics.serviceMonitor.enabled=true \
  --set grafanaDashboard.enabled=true \
  --wait --debug
```

Parameters:

| Helm value | Purpose |
| --- | --- |
| `topoDiscovery.scaleUp.mode` | Set to `nv-topograph` to select NVIDIA Topograph as the scale-up label writer. This setting is independent of scale-out mode. |
| `topoDiscovery.scaleOut.mode` | Set to `nv-topograph` to select NVIDIA Topograph as the scale-out label writer. |
| `topoDiscovery.storage.mode` | Set to `unifabric-roce` for built-in RoCE storage discovery. |
| `nvidiaTopograph.provider.name` | Set to `netq` to discover topology through the NetQ API. |
| `nvidiaTopograph.provider.params.apiUrl` | NetQ API URL. |
| `nvidiaTopograph.credentialsSecretName` | Name of an existing NetQ credentials Secret containing a `credentials.yaml` key. The credentials are mounted read-only only into the Topograph Pod. |
| `nodeMetrics.enabled` | Enables Agent metrics for node RDMA observability. |
| `fabricNode.scaleUpInterfaceSelector` | Selects specific RDMA NICs for observation and labels them with `kind=scaleUp` in RDMA metrics. Supports `interface=eth*,mlx*` or `cidr=172.17.0.0/16`. Defaults to all RDMA NICs. |
| `fabricNode.storageInterfaceSelector` | Selects storage RDMA NICs and labels them with `kind=storage` in RDMA metrics. Supports `interface=eth*,mlx*` or `cidr=172.17.0.0/16`. Defaults to empty. |
| `nodeMetrics.serviceMonitor.enabled` | Creates the Prometheus Operator `ServiceMonitor`. |
| `grafanaDashboard.enabled` | Renders the built-in RDMA dashboards. |

For more Helm parameters, see [chart/README.md](../chart/README.md).

If you are in mainland China, add the following parameters to speed up image pulls:

```bash
--set global.registry=m.daocloud.io \
--set controller.image.repository=ghcr.io/unifabric-io/unifabric-controller \
--set agent.image.repository=ghcr.io/unifabric-io/unifabric-agent \
--set nvidiaTopograph.image.repository=ghcr.io/nvidia/topograph
```

## Verify the Deployment

Check topograph components, NetQ provider configuration, and Node labels:

```bash
kubectl -n unifabric-system get pods
kubectl get pods -n unifabric-system -o wide
kubectl -n unifabric-system describe secret netq-credentials
kubectl get fabricnodes.unifabric.io
kubectl get nodes -L scale-up.unifabric.io/tier-1,scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2,scale-out.unifabric.io/tier-3,kubernetes.io/hostname
kubectl get topo
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
- Confirm that `nvidiaTopograph.credentialsSecretName` refers to a Secret in
  the installation namespace and that the Secret contains a
  `credentials.yaml` key.
- Confirm that the NetQ account can access the target premises and that NetQ has fabric topology data.
- If `topoDiscovery.*.label.keyTemplate` values are customized, scheduler label keys must be updated accordingly.

### RDMA Metrics Do Not Include RDMA NICs

- Confirm that the Agent Pod is running and RDMA devices are visible under `/sys/class/infiniband` on the node.
- Confirm that `fabricNode.scaleUpInterfaceSelector` matches the RDMA NIC name or CIDR.
- Confirm that `nodeMetrics.enabled=true`.
- If using Prometheus Operator, confirm that `nodeMetrics.serviceMonitor.enabled=true` and the `ServiceMonitor` is selected by Prometheus.

### topograph Cannot Reach NetQ

- Check the NetQ API URL, username and password in the Secret, certificates,
  and network connectivity.
- Confirm that the Topograph Pod mounts the credentials at
  `/etc/topograph/credentials/credentials.yaml`.
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
