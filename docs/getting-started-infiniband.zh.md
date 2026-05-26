# InfiniBand Fabric

本文说明如何在 InfiniBand 网卡集群中部署 Unifabric。该场景适用于 IB 网络，例如 Mellanox
网卡运行在 IB 模式并连接 IB 交换机。

## 部署目标

完成部署后，集群中应达成两个目标：

- Node 被写入可供调度系统消费的拓扑 label，包括：
  `unifabric.io/scale-up`、`unifabric.io/scale-out-leaf`、
  `unifabric.io/scale-out-spine`、`unifabric.io/scale-out-core`。
- 节点 RDMA 状态可观测，能够通过 Unifabric Agent metrics 查看 RDMA device、port、
  priority 和 Pod 归属等指标。

> 该场景不会为 Unifabric switch-driven discovery 创建 `FabricNode` 或 `Switch` CR。

## 前置条件

- Kubernetes 集群，包含目标 GPU 节点。
- 已安装 `kubectl` 和 Helm 3。
- 节点上存在 InfiniBand / RDMA 设备，并能在 `/sys/class/infiniband` 下看到。
- GPU Operator 和 NVIDIA device plugin 已部署。
- node-data-broker 能在目标节点运行，并具备所需 `pods/exec` 权限。
- 集群需要安装 Prometheus Operator 和 Grafana Operator，如果未安装，请在安装 Unifabric 时取消下发 ServiceMonitor
  和 GrafanaDashboard，避免下发 CRD 失败。

确认当前集群连接：

```bash
kubectl cluster-info
kubectl get nodes -o wide
```

## 安装 Unifabric

以下命令使用最新的 release 版本。示例将 RDMA interface selector 留空，因此所有 RDMA
网卡都会被 metrics 观测。InfiniBand 拓扑 label 由 NVIDIA topograph 写回。

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
  --set nodeMetrics.enabled=true \
  --set nodeMetrics.serviceMonitor.enabled=true \
  --set grafanaDashboard.enabled=true \
  --wait
```

参数说明：

| Helm value | 用途 |
| --- | --- |
| `nvidiaTopograph.enable` | 启用 NVIDIA topograph。InfiniBand IB 网络场景必须为 `true`。 |
| `nvidiaTopograph.provider.name` | 设置为 `infiniband-k8s`，表示使用 `ibnetdiscover` 命令发现拓扑。 |

| `nodeMetrics.enabled` | 开启 Agent Metrics 用于节点 RDMA 可观测。 |
| `nodeTopologyDiscovery.scaleUpInterfaceSelector` | 选择特定 RDMA 网卡用于观测，并在 RDMA 指标中打上 `kind=scaleOut` 标签，支持 `interface=ib*,mlx*` 或 `cidr=172.17.0.0/16`，默认为全部 RDMA 网卡。 |
| `nodeTopologyDiscovery.storageInterfaceSelector` | 选择一组 RDMA 存储网卡观测，并在 RDMA 指标中打上 `kind=storage` 标签。支持 `interface=ib*,mlx*` 或 `cidr=172.17.0.0/16`。默认为空。|
| `nodeMetrics.serviceMonitor.enabled` | 创建 Prometheus Operator 使用的 `ServiceMonitor`。 |
| `grafanaDashboard.enabled` | 渲染内置 RDMA Dashboard。 |

更多 Helm 参数见 [chart/README.md](../chart/README.md)。

如果您位于中国地区，可以额外增加下面的参数，加速下载

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

重点检查 topograph 组件、node-data-broker DaemonSet、Node annotation 和 Node label：

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

## 常见问题

### Node label 没有写入

- 确认 NVIDIA topograph、node-observer 和 node-data-broker 组件正常运行，并且有权限更新 Node。
- 确认 node-data-broker Pod 已运行在目标 GPU 节点。
- 确认对应节点有 `topograph.nvidia.com/cluster-id` annotation。
- 确认 `ibnetdiscover` 可用。
- 如果自定义了 Helm values 中的 `topologyLabels.*`，调度器配置中的 label key 也必须同步更新。

### RDMA metrics 没有采集到 IB 网卡

- 确认 Agent Pod 正常运行，并且节点上能在 `/sys/class/infiniband` 下看到 IB 设备。
- 确认 `nodeTopologyDiscovery.scaleUpInterfaceSelector` 匹配节点上的 IB 网卡名称或 CIDR。
- 确认 `nodeMetrics.enabled=true`。
- 如果使用 Prometheus Operator，确认 `nodeMetrics.serviceMonitor.enabled=true` 且
  `ServiceMonitor` 能被 Prometheus selector 选中。

## 卸载

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

## 下一步

- 返回 [文档索引](./README.zh.md)。
- 阅读 [Kueue TAS 工作负载示例](./usage/workload-tas.zh.md)。
- 查看 [Helm values 参考](../chart/README.md)。
