# Basic Installation

中文版：[getting-started.zh.md](./getting-started.zh.md)

## Prerequisites

- Access to the target Kubernetes cluster.
- `kubectl` and Helm installed.

## Install

The following command installs the latest Unifabric release. You can also select a specific version
from the [releases](https://github.com/unifabric-io/unifabric/releases) page.

```bash
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unifabric-io/unifabric/releases/latest | grep '"tag_name":' | cut -d '"' -f4)
CHART_VERSION="${LATEST_TAG#v}"

helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --version "${CHART_VERSION}" \
  --namespace unifabric-system \
  --create-namespace \
  --wait
```

By default, all RDMA NICs on a node are classified as scale-out. If a node has RDMA NICs for
different purposes, configure the corresponding selector during installation with `--set`.

For example, suppose a node has five RDMA NICs named `eth1` through `eth5`. If `eth1` through
`eth4` are used for scale-out and `eth5` is used for storage, add the following option to the
installation command:

```bash
--set fabricNode.storageInterfaceSelector="interface=eth5"
```

`eth5` is classified as storage, while the other four RDMA NICs remain classified as scale-out by
the default rule. See the [Helm values reference](../chart/README.md) for the complete selector
syntax.

If the cluster does not have Prometheus Operator or Grafana Operator, add the corresponding options
to the installation command:

```bash
--set nodeMetrics.serviceMonitor.enabled=false \
--set grafanaDashboard.enabled=false
```

In mainland China, add the following options to accelerate image pulls:

```bash
--set global.registry=m.daocloud.io \
--set controller.image.repository=ghcr.io/unifabric-io/unifabric-controller \
--set agent.image.repository=ghcr.io/unifabric-io/unifabric-agent \
--set nvidiaTopograph.image.repository=ghcr.io/nvidia/topograph
```

## Verify the Base Installation

### 1. Check FabricNode NIC Information

Confirm that the Controller and Agent Pods are running and that a `FabricNode` has been created for
every node:

```bash
kubectl -n unifabric-system get pods
kubectl get fabricnodes
kubectl get fabricnode <node-name> -o yaml
```

Check that `status.totalNics`, `status.scaleOutNics`, and `status.storageNics` match the node's
actual NIC configuration. For the five-NIC example above, the result should look similar to:

```yaml
apiVersion: unifabric.io/v1beta1
kind: FabricNode
metadata:
  name: node1
status:
  totalNics: 5
  scaleOutNics:
    - name: eth1
      rdmaDeviceName: mlx5_0
      rdma: true
      state: up
    - name: eth2
      rdmaDeviceName: mlx5_1
      rdma: true
      state: up
    - name: eth3
      rdmaDeviceName: mlx5_2
      rdma: true
      state: up
    - name: eth4
      rdmaDeviceName: mlx5_3
      rdma: true
      state: up
  storageNics:
    - name: eth5
      rdmaDeviceName: mlx5_4
      rdma: true
      state: up
```

### 2. View Node RDMA Metrics

Open the `Unifabric RDMA Node` dashboard in Grafana and select the cluster and node at the top of the
page. Expand groups such as `Throughput`, `Basic`, `RoCE`, `QoS`, or `Interface`, and confirm that
the dashboard lists the node's RDMA devices, network interfaces, and metrics.

![Unifabric RDMA Node dashboard](images/unifabric-rdma-node-dashboard.png)

If the dashboard shows the target node's devices, `ifname` values, and metric data, the Agent metrics
have been collected by Prometheus and can be queried successfully.

## Configure Topology Discovery

After the base installation succeeds, continue with the guide for the physical network:

- For a switch network running SONiC, see [General SONiC RoCE](./getting-started-sonic-roce.md).
- For a cluster using Spectrum-X switches and NetQ network management, see
  [Spectrum-X fabric](./getting-started-spectrum-x.md).
- For a cluster using an NVIDIA InfiniBand network, see
  [InfiniBand fabric](./getting-started-infiniband.md).

These scenario guides use `helm upgrade --reuse-values` to change only the Helm values relevant to
the selected network. Other base-installation settings remain unchanged.

## Troubleshooting

### FabricNode Does Not Discover the Expected RDMA NICs

- Confirm that the Agent Pods are running:

  ```bash
  kubectl -n unifabric-system get daemonset unifabric-agent
  kubectl -n unifabric-system logs daemonset/unifabric-agent -c agent
  ```

- Confirm that the node's RDMA devices are visible to the Agent:

  ```bash
  kubectl -n unifabric-system exec daemonset/unifabric-agent -c agent -- \
    ls /sys/class/infiniband
  ```

- Check whether `fabricNode.scaleOutInterfaceSelector`, `storageInterfaceSelector`, and
  `scaleUpInterfaceSelector` match the actual interface names or CIDRs.
- Check whether any NIC is misclassified. NICs matched by the storage or scale-up selector do not
  appear in `status.scaleOutNics`.

### Agent Metrics or the Grafana Dashboard Has No Data

Confirm that Agent metrics are enabled, then check the Service and ServiceMonitor:

```bash
helm get values unifabric -n unifabric-system
kubectl -n unifabric-system get service unifabric-agent-metrics
kubectl -n unifabric-system get servicemonitor unifabric-agent-metrics
```

If `nodeMetrics.serviceMonitor.enabled` was disabled during installation, the absence of a
`ServiceMonitor` is expected. Access the Agent metrics endpoint directly to confirm whether the
collector is producing metrics:

```bash
POD_IP=$(kubectl -n unifabric-system get pod \
  -l app.kubernetes.io/component=unifabric-agent \
  -o jsonpath='{.items[0].status.podIP}')

curl -s "http://${POD_IP}:8082/metrics" | grep '^unifabric_'
```

- If no metrics are returned, confirm that `nodeMetrics.enabled=true`, then check the Agent logs and
  the RDMA devices under `/sys/class/infiniband`.
- If the endpoint returns metrics but the Grafana dashboard has no data, check whether Prometheus
  has discovered the `unifabric-agent-metrics` target and whether the correct Grafana data source,
  cluster, and node variables are selected.
