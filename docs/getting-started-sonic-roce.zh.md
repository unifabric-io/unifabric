# 通用 SONiC RoCE

本文说明如何在 SONiC 交换机承载 RoCE 网络的集群中部署 Unifabric。该场景通过节点 RDMA
网卡和 LLDP 邻居信息发现 scale-out leaf 拓扑。

## 部署目标

完成部署后，集群中应达成两个目标：

- Node 被写入可供调度系统消费的 leaf 拓扑 label，默认是
  `unifabric.io/scale-out-leaf=<group-name>`。
- 节点 RDMA 状态可观测，能够通过 Unifabric Agent metrics 查看 RDMA device、port、
  priority 和 Pod 归属等指标。
- 可以通过 `FabricNode` CR 和 `ScaleOutLeafGroup` CR 查询相应拓扑。

> 目前 Unifabric 仅支持识别 leaf label，spine 和 core 级别的拓扑识别工作正在进行。

## 前置条件

- Kubernetes 集群，包含 Linux worker 节点。
- 已安装 `kubectl` 和 Helm 3。
- 节点上存在 RDMA-capable network interfaces，并能在 `/sys/class/infiniband` 下看到。
- 交换机和节点侧 LLDP 可用，Agent 会读取 LLDP 邻居信息。
- Agent 需要 privileged 权限，用于访问主机网络、RDMA 设备、`/proc` 和 container runtime 状态。
- 集群需要安装 Prometheus Operator 和 Grafana Operator，如果未安装，请在安装 Unifabric 时候取消下发 ServiceMonitor
  和 GrafanaDashboard，避免下发 CRD 失败。

确认当前集群连接：

```bash
kubectl cluster-info
kubectl get nodes -o wide
```

## 安装 Unifabric

以下命令使用最新的 release 版本。示例把 `eth9` 作为存储 RDMA 网卡排除出 scale-out
leaf 分组；其余 RDMA 网卡默认作为 scale-out 网卡参与 LLDP 拓扑发现和 RDMA metrics
观测。

```bash
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unifabric-io/unifabric/releases/latest | grep '"tag_name":' | cut -d '"' -f4)
CHART_VERSION="${LATEST_TAG#v}"

helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --version "${CHART_VERSION}" \
  --namespace unifabric-system \
  --create-namespace \
  --set nvidiaTopograph.enable=false \
  --set scaleOutDiscovery.leafGroups.enabled=true \
  --set-string nodeTopologyDiscovery.storageInterfaceSelector="interface=eth9" \
  --set nodeMetrics.enabled=true \
  --set nodeMetrics.serviceMonitor.enabled=true \
  --set grafanaDashboard.enabled=true \
  --wait
```

参数说明：

| Helm value | 用途 |
| --- | --- |
| `nvidiaTopograph.enable` | SONiC RoCE / LLDP 场景保持 `false`，拓扑由 Unifabric Agent / Controller 发现。 |
| `scaleOutDiscovery.leafGroups.enabled` | 开启 Unifabric 自身的 `ScaleOutLeafGroup` 和 leaf Node label 写回。 |
| `nodeTopologyDiscovery.storageInterfaceSelector` | 可选：选择存储 RDMA 网卡，并从 scale-out leaf 分组中排除；指标中标记为 `kind=storage`。支持 `interface=eth9` 或 `cidr=172.20.0.0/16`。 |
| `nodeTopologyDiscovery.scaleUpInterfaceSelector` | 可选：选择 scale-up RDMA 网卡，并从 scale-out leaf 分组中排除；指标中标记为 `kind=scaleUp`。 |
| `nodeTopologyDiscovery.scaleOutInterfaceSelector` | 可选：限制参与 scale-out leaf 分组的 RDMA 网卡；为空时，所有未命中 storage / scale-up selector 的 RDMA 网卡都会参与。 |
| `nodeMetrics.enabled` | 开启 Agent Metrics 用于节点 RDMA 可观测。 |
| `nodeMetrics.serviceMonitor.enabled` | 创建 Prometheus Operator 使用的 `ServiceMonitor`。 |
| `grafanaDashboard.enabled` | 渲染内置 RDMA Dashboard。 |
| `topologyLabels.scaleOutLeaf` | leaf Node label key，默认 `unifabric.io/scale-out-leaf`。 |

更多 Helm 参数见 [chart/README.md](../chart/README.md)。

如果您位于中国地区，可以额外增加下面的参数，加速下载：

```bash
--set global.registry=m.daocloud.io \
--set controller.image.repository=ghcr.io/unifabric-io/unifabric-controller \
--set agent.image.repository=ghcr.io/unifabric-io/unifabric-agent \
--set agent.lldp.image.repository=ghcr.io/unifabric-io/unifabric-agent \
```

## 验证部署

等待 controller 和 agent 就绪：

```bash
kubectl -n unifabric-system get pods
kubectl -n unifabric-system rollout status deployment/unifabric-controller
kubectl -n unifabric-system rollout status daemonset/unifabric-agent
```

查看 `FabricNode`：

```bash
kubectl get fabricnodes
kubectl get fabricnode <node-name> -o yaml
```

重点检查：

- `status.scaleOutNics` 是否包含预期的 scale-out RDMA 网卡。
- `status.storageNics` 是否只包含存储网络接口。
- `status.scaleOutNics[*].lldpNeighbor.hostname` 是否存在。
- `status.conditions` 中 `Ready` 和 `LLDPNeighborsReady` 是否为 `True`。

查看 leaf 分组和 Node label：

```bash
kubectl get scaleoutleafgroups -o wide
kubectl get scaleoutleafgroup <group-name> -o yaml
kubectl get nodes -L unifabric.io/scale-out-leaf,kubernetes.io/hostname
```

如果节点被正确分组，`ScaleOutLeafGroup.status.nodes` 会包含对应节点，且节点上会出现
`unifabric.io/scale-out-leaf=<group-name>`。

配置 Kueue、Volcano 或 KAI Scheduler 时，应只使用上述命令中已经真实写到 Node 上的 label。
当前 SONiC RoCE / LLDP 场景只会自动写入 leaf label，还未写入 spine/core 等更高层级。

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
curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="scaleOut"'
```

## 常见问题

### `FabricNode` 没有 scale-out NIC

- 确认节点上能在 `/sys/class/infiniband` 下看到 RDMA 设备。
- 确认 `nodeTopologyDiscovery.scaleOutInterfaceSelector` 匹配实际接口名或 CIDR。
- 确认没有把 scale-out 接口误匹配为 storage 或 scale-up；这些接口会从 scale-out 分组中排除。

### `LLDPNeighborsReady=False`

- 确认交换机已开启 LLDP。
- 确认节点侧 `lldpd` 正常工作，Agent Pod 内可以读取 LLDP 信息。
- 确认 `nodeTopologyDiscovery.initialScanDelay` 足够长，避免 Agent 首次扫描早于 LLDP 学习完成。

### 没有 `ScaleOutLeafGroup` 或 Node label

- 确认 `scaleOutDiscovery.leafGroups.enabled=true`。
- 确认 `FabricNode.status.nodeRole` 不是 `Storage`。
- 确认至少一个 `scaleOutNics` 同时满足 `state=up` 且有 `lldpNeighbor.hostname`。
- 查看 controller 日志：

  ```bash
  kubectl -n unifabric-system logs deployment/unifabric-controller
  ```

### RDMA metrics 没有采集到 RoCE 网卡

- 确认 Agent Pod 正常运行，并且节点上能在 `/sys/class/infiniband` 下看到 RDMA 设备。
- 确认 `nodeMetrics.enabled=true`。
- 如果使用 Prometheus Operator，确认 `nodeMetrics.serviceMonitor.enabled=true` 且
  `ServiceMonitor` 能被 Prometheus selector 选中。
- 先直接访问 Agent metrics 端点，确认 `unifabric_` 指标存在，再排查 Prometheus target discovery。

## 卸载

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

如不再需要 CRD，可手动删除：

```bash
kubectl delete crd fabricnodes.unifabric.io scaleoutleafgroups.unifabric.io
```

## 下一步

- 返回 [文档索引](./README.zh.md)。
- 阅读 [Kueue TAS 工作负载示例](./usage/workload-tas.zh.md)。
- 查看 [Helm values 参考](../chart/README.md)。
