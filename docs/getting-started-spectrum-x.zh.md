# Spectrum-X Fabric

本文说明如何在 Spectrum-X 交换机集群中部署 Unifabric。该场景通过 NetQ API 获取 fabric 拓扑。

## 部署目标

完成部署后，集群中应达成两个目标：

- Node 被写入可供调度系统消费的拓扑 label，包括：
  `unifabric.io/scale-up`、`unifabric.io/scale-out-leaf`、
  `unifabric.io/scale-out-spine`、`unifabric.io/scale-out-core`。
- 节点 RDMA 状态可观测，能够通过 Unifabric Agent metrics 查看 RDMA device、port、
  priority 和 Pod 归属等指标。

> 该场景不会创建 `FabricNode` CR，也不会创建或更新 `ScaleOutLeafGroup` CR。

## 前置条件

- Kubernetes 集群，包含目标 GPU 节点。
- 已安装 `kubectl` 和 Helm 3。
- 节点上存在 RDMA 设备，并能在 `/sys/class/infiniband` 下看到。
- Spectrum-X fabric 已经由 NetQ 管理，并且 NetQ 中已经有对应 fabric 拓扑数据。
  - 如果您没有部署 NetQ，但是网络是承载的 IB 网络，那么可以参考[InfiniBand fabric](./getting-started-spectrum-x.zh.md)。
  - 如果你承载的是 RoCE 网络，那么可以参考[General SONiC RoCE](./getting-started-sonic-roce.md)。
- 集群可以访问 NetQ API。
- 已准备 NetQ 用户名、密码和 API URL。
- 集群需要安装 Prometheus Operator 和 Grafana Operator，如果未安装，请在安装 Unifabric 时取消下发 ServiceMonitor
  和 GrafanaDashboard，避免下发 CRD 失败。

确认当前集群连接：

```bash
kubectl cluster-info
kubectl get nodes -o wide
```

## 准备 NetQ 凭据

先创建包含 `credentials.yaml` 的 Secret：

```bash
kubectl create namespace unifabric-system --dry-run=client -o yaml | kubectl apply -f -
kubectl -n unifabric-system create secret generic netq-credentials \
  --from-file=credentials.yaml=./netq-credentials.yaml
```

`netq-credentials.yaml` 内容：

```yaml
username: <netq-user>
password: <netq-password>
```

Secret 默认挂载为 `/etc/topograph/credentials/credentials.yaml`。只有需要使用非默认文件名
或路径时，才额外设置 `nvidiaTopograph.topograph.config.credentialsPath`。

## 安装 Unifabric

以下命令使用最新的 release 版本。示例将 RDMA interface selector 留空，因此所有 RDMA
网卡都会被 metrics 观测；同时关闭 Unifabric 自身的 leaf group 路径，避免
Unifabric Agent / Controller 通过 LLDP/FabricNode 写回拓扑 label。Spectrum-X
拓扑 label 由 NVIDIA topograph 通过 NetQ provider 写回。

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
  --set scaleOutDiscovery.leafGroups.enabled=false \
  --set nodeMetrics.enabled=true \
  --set nodeMetrics.serviceMonitor.enabled=true \
  --set grafanaDashboard.enabled=true \
  --wait
```

参数说明：

| Helm value | 用途 |
| --- | --- |
| `nvidiaTopograph.enable` | 启用 NVIDIA topograph。Spectrum-X 场景必须为 `true`。 |
| `nvidiaTopograph.provider.name` | 设置为 `netq`，表示通过 NetQ API 发现拓扑。 |
| `nvidiaTopograph.provider.params.apiUrl` | NetQ API URL。 |
| `nvidiaTopograph.topograph.config.credentialsSecret` | 包含 `credentials.yaml` 的 Secret 名称。 |
| `nvidiaTopograph.topograph.config.credentialsPath` | 可选：非默认凭据路径。 |

| `scaleOutDiscovery.leafGroups.enabled` | 关闭 Unifabric 自身的 `ScaleOutLeafGroup` 和 leaf Node label 写回。 |
| `nodeMetrics.enabled` | 开启 Agent Metrics 用于节点 RDMA 可观测。 |
| `nodeTopologyDiscovery.scaleUpInterfaceSelector` | 选择特定 RDMA 网卡用于观测，并在 RDMA 指标中打上 `kind=scaleOut` 标签，支持 `interface=eth*,mlx*` 或 `cidr=172.17.0.0/16`，默认为全部 RDMA 网卡。 |
| `nodeTopologyDiscovery.storageInterfaceSelector` | 选择一组 RDMA 存储网卡观测，并在 RDMA 指标中打上 `kind=storage` 标签。支持 `interface=eth*,mlx*` 或 `cidr=172.17.0.0/16`。默认为空。|
| `nodeMetrics.serviceMonitor.enabled` | 创建 Prometheus Operator 使用的 `ServiceMonitor`。 |
| `grafanaDashboard.enabled` | 渲染内置 RDMA Dashboard。 |

更多 Helm 参数见 [chart/README.md](../chart/README.md)。

如果您位于中国地区，可以额外增加下面的参数，加速下载：

```bash
--set global.registry=m.daocloud.io \
--set controller.image.repository=ghcr.io/unifabric-io/unifabric-controller \
--set agent.image.repository=ghcr.io/unifabric-io/unifabric-agent \
--set agent.lldp.image.repository=ghcr.io/unifabric-io/unifabric-agent \
--set nvidiaTopograph.topograph.image.repository=ghcr.io/nvidia/topograph \
--set nvidiaTopograph.nodeObserver.image.repository=ghcr.io/nvidia/topograph \
--set nvidiaTopograph.nodeDataBroker.image.repository=ghcr.io/nvidia/topograph/ib \
--set nvidiaTopograph.nodeDataBroker.initContainer.image.repository=ghcr.io/nvidia/topograph \
```

## 验证部署

重点检查 topograph 组件、NetQ provider 配置和 Node label：

```bash
kubectl -n unifabric-system get pods
kubectl get pods -n unifabric-system -o wide
kubectl get nodes -L unifabric.io/scale-up,unifabric.io/scale-out-core,unifabric.io/scale-out-spine,unifabric.io/scale-out-leaf,kubernetes.io/hostname
```

配置 Kueue、Volcano 或 KAI Scheduler 时，应只使用上述命令中已经真实写到 Node 上的 label。

验证 RDMA metrics 资源：

```bash
kubectl -n unifabric-system get service unifabric-agent-metrics
kubectl -n unifabric-system get servicemonitor unifabric-agent-metrics
```

直接检查 Agent metrics 端点：

```bash
POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-agent -o jsonpath='{.items[0].status.podIP}')
curl -s "http://${POD_IP}:8082/metrics" | grep '^unifabric_'
```

检查 RDMA metrics 中的网卡分类：

```bash
curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="scaleUp"'
```

## 常见问题

### Node label 没有写入

- 确认 NVIDIA topograph 和 node-observer 组件正常运行，并且有权限更新 Node。
- 确认 `nvidiaTopograph.provider.name=netq`。
- 确认 `nvidiaTopograph.provider.params.apiUrl=<netq-url>` 配置正确。
- 确认 NetQ 账号可访问目标 premises，并且 NetQ 中已经有对应 fabric 拓扑数据。
- 确认 `nvidiaTopograph.topograph.config.credentialsSecret` 指向的 Secret 存在，并包含
  `credentials.yaml`。
- 如果自定义了 Helm values 中的 `topologyLabels.*`，调度器配置中的 label key 也必须同步更新。

### RDMA metrics 没有采集到 RDMA 网卡

- 确认 Agent Pod 正常运行，并且节点上能在 `/sys/class/infiniband` 下看到 RDMA 设备。
- 确认 `nodeTopologyDiscovery.scaleUpInterfaceSelector` 匹配节点上的 RDMA 网卡名称或 CIDR。
- 确认 `nodeMetrics.enabled=true`。
- 如果使用 Prometheus Operator，确认 `nodeMetrics.serviceMonitor.enabled=true` 且
  `ServiceMonitor` 能被 Prometheus selector 选中。

### topograph 无法访问 NetQ

- 检查 NetQ API URL、证书和网络连通性。
- 查看 topograph / node-observer 日志。
- 确认 topograph Service 可达。

## 卸载

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

## 下一步

- 返回 [文档索引](./README.zh.md)。
- 阅读 [Kueue TAS 工作负载示例](./usage/workload-tas.zh.md)。
- 查看 [Helm values 参考](../chart/README.md)。
