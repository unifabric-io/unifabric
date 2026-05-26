# RDMA 可观测性使用指南

本文说明如何使用 Unifabric RDMA 可观测性，包括验证 Prometheus 指标、检查网卡分类和查看
Grafana 仪表盘。

## 功能概览

Unifabric Agent 会在每个节点上采集 RDMA 相关指标，并通过 controller-runtime 的 Prometheus `/metrics` 端点暴露。指标包括：

- RDMA 设备和端口计数器，例如收发字节、包数和错误计数。
- 网络接口属性，例如速率、MTU、端口状态和 RDMA device ToS。
- ethtool 优先级计数器，例如 pause 和 discard。
- Pod 维度归因指标，包括命名空间、Pod、顶层 workload owner 和 host RDMA 模式。
- 拓扑维度标签，例如 `kind=scaleOut`、`kind=storage`、`is_root`、`parent_ifname`。

Agent 采集依赖本机 Linux sysfs、containerd 运行时状态和 `FabricNode.status.rdmaPods`。完整指标模型见 [RDMA metrics design](../design/rdma-metrics.md)。

## 安装入口

使用 RDMA metrics 前，先按 fabric 场景完成 Unifabric 安装，并在安装文档中开启
`nodeMetrics` 和 dashboard。安装步骤见 [文档索引](../README.zh.md) 中的 Get Started。

场景入口：

| 场景 | 安装文档 |
| --- | --- |
| 通用 SONiC RoCE | [通用 SONiC RoCE](../getting-started-sonic-roce.zh.md) |
| Spectrum-X fabric | [Spectrum-X fabric](../getting-started-spectrum-x.zh.md) |
| InfiniBand fabric | [InfiniBand fabric](../getting-started-infiniband.zh.md) |

Prometheus Operator 已安装时，才能使用 chart 渲染的 `ServiceMonitor`。Grafana sidecar 或
Grafana Operator 已安装时，才能自动导入 chart 渲染的 dashboard。

## 验证指标

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
curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="scaleUp"'
curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="storage"'
```

`kind` 标签由安装时配置的 `nodeTopologyDiscovery.*InterfaceSelector` 决定。未配置
scale-out selector 且接口可识别时，Agent 会默认将接口标记为 `scaleOut`。

## Dashboard

Chart 内置以下 RDMA dashboard 文件：

- `rdma-cluster.json`
- `rdma-node.json`
- `rdma-pod.json`
- `rdma-workload.json`

当 `grafanaDashboard.enabled=true` 时，chart 会根据 `grafanaDashboard.kind` 将这些文件渲染为 `ConfigMap` 或 `GrafanaDashboard`。

验证 dashboard 资源：

```bash
kubectl -n unifabric-system get configmap -l app.kubernetes.io/component=unifabric
kubectl -n unifabric-system get grafanadashboard -l app.kubernetes.io/component=unifabric
```

如果 dashboard 页面没有数据，先确认 Prometheus 数据源能查询到 `unifabric_node_info`，再确认 dashboard 的 `cluster`、`node`、`namespace` 等变量选择是否匹配当前指标标签。

## 排障

如果没有 RDMA 指标：

1. 确认 Agent 正常运行：

   ```bash
   kubectl -n unifabric-system logs ds/unifabric-agent -c agent
   ```

2. 确认节点上能看到 RDMA 设备：

   ```bash
   kubectl -n unifabric-system exec ds/unifabric-agent -c agent -- ls /sys/class/infiniband
   ```

3. 按场景确认 RDMA 网卡被识别：

   ```bash
   # SONiC RoCE / LLDP 场景
   kubectl get fabricnodes
   kubectl get fabricnode <node-name> -o yaml
   ```

   Spectrum-X 和 InfiniBand 场景使用 NVIDIA topograph 写回 Node label，不依赖
   Unifabric `FabricNode` 或 `Switch` 拓扑资源；这两个场景优先检查 Agent metrics
   端点和 `/sys/class/infiniband`。

4. 确认 Agent Pod metrics 端点能直接返回 Unifabric 指标：

   ```bash
   POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-agent -o jsonpath='{.items[0].status.podIP}')
   curl -s "http://${POD_IP}:8082/metrics" | grep '^unifabric_'
   ```

   如果这里没有 `unifabric_` 开头的指标，先排查 Agent 日志和 RDMA 设备可见性；如果这里有指标但 Prometheus 没有数据，再检查 `unifabric-agent-metrics` Service、EndpointSlice 和 ServiceMonitor。

5. 如果使用 `ServiceMonitor`，确认 Prometheus 能发现对应 target：

   ```bash
   kubectl -n unifabric-system get servicemonitor unifabric-agent-metrics -o yaml
   ```

如果没有 Pod 维度指标：

- 确认 `FabricNode.status.rdmaPods` 中存在目标 Pod。
- 确认 Pod 使用 SR-IOV RDMA 设备，或以 host RDMA 模式运行。
- 当前非 host RDMA Pod 归因依赖 containerd 容器 ID；非 containerd 运行时会被跳过。
- 缺失或过期的 Pod 容器信息不会阻止 host 级 RDMA 指标导出。

如果 `kind` 标签为空：

- 检查 `nodeTopologyDiscovery.scaleOutInterfaceSelector`、`nodeTopologyDiscovery.storageInterfaceSelector` 和 `nodeTopologyDiscovery.scaleUpInterfaceSelector`。
- selector 支持 `interface=eth*,!eth9` 和 `cidr=172.17.0.0/16` 两种形式。
- 当没有配置 scale-out selector 且接口可识别时，Agent 会默认将接口标记为 `scaleOut`。
