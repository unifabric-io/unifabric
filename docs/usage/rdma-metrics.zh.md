# RDMA 指标

English version: [rdma-metrics.md](./rdma-metrics.md)

## 简介

RDMA 指标功能把每个节点的 RDMA 状态以 Prometheus 指标和 Grafana 仪表盘的形式暴露
出来。Unifabric Agent 在每个节点上采集指标,并通过 `8082` 端口的 `/metrics` 端点
提供服务。本功能覆盖节点级 RDMA 设备与端口计数器、网络接口属性、ethtool 优先级
计数器和 Pod 归属指标。非 host RDMA Pod 的归属仅支持 containerd 运行时;其他运行时
会被跳过,节点级指标不受影响。

指标包括:

- RDMA 设备与端口计数器,如收发字节数、包数和错误计数。
- 网络接口属性,如速率、MTU、端口状态和 RDMA 设备 ToS。
- ethtool 优先级计数器,如 pause 和 discard 计数。
- Pod 归属指标,包括 namespace、Pod、顶层 workload owner 和 host RDMA 模式。
- 拓扑维度标签,如 `kind=scaleOut`、`kind=storage`、`is_root` 和 `parent_ifname`。

采集读取本地 Linux sysfs、containerd 运行时状态和 `FabricNode.status.rdmaPods`。
完整的指标模型参见 [RDMA 指标设计](../design/rdma-metrics.zh.md)。

## 前提条件

- 已按[基础安装](../getting-started.zh.md)安装 Unifabric,并完成对应网络场景的指南:
  [SONiC RoCE](../getting-started-sonic-roce.zh.md)、
  [Spectrum-X](../getting-started-spectrum-x.zh.md) 或
  [InfiniBand](../getting-started-infiniband.zh.md)。
- Helm values 中已启用 `nodeMetrics`。各场景指南会启用它。
- 已安装 Prometheus Operator。Chart 渲染的 `ServiceMonitor` 由 Prometheus Operator
  消费。
- 已安装 Grafana sidecar 或 Grafana Operator,用于导入 Chart 渲染的仪表盘。

## 配置与使用

1. 确认指标 Service 和 ServiceMonitor 存在:

   ```bash
   kubectl -n unifabric-system get service unifabric-agent-metrics
   kubectl -n unifabric-system get servicemonitor unifabric-agent-metrics
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

2. 确认 Agent 输出 Unifabric 指标。在能访问 Pod IP 的主机上执行:

   ```bash
   POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-agent -o jsonpath='{.items[0].status.podIP}')
   curl -s "http://${POD_IP}:8082/metrics" | grep '^unifabric_'
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

3. 确认 Agent 已完成 RDMA 网卡分类。每条指标上的 `kind` 标签表示该接口所服务的
   网络:

   ```bash
   curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="scaleOut"'
   curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="scaleUp"'
   curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="storage"'
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

   安装时配置的 `fabricNode.*InterfaceSelector` Helm values 决定每个接口的 `kind`
   取值。

   > **注意:** 如果未配置 scale-out 选择器,且 Agent 能识别该接口,Agent 把其
   > `kind` 置为 `scaleOut`。

4. 打开仪表盘。当 `grafanaDashboard.enabled=true` 时,Chart 按 `grafanaDashboard.kind`
   把四个 RDMA 仪表盘渲染为 `ConfigMap` 或 `GrafanaDashboard` 资源:

   - `rdma-cluster.json`:集群级 RDMA 流量与错误。
   - `rdma-node.json`:单节点设备与端口计数器。
   - `rdma-pod.json`:Pod 归属 RDMA 指标。
   - `rdma-workload.json`:按顶层 workload owner 聚合的指标。

   每个仪表盘在 `chart/files/dashboard-en/` 下有英文版,在 `chart/files/dashboard-zh/`
   下有中文版。`grafanaDashboard.language` 决定安装哪套:`en`(默认)、`zh`(仪表盘
   uid 和资源名带 `-zh` 后缀)或 `all`(两套都装)。

   确认仪表盘资源存在:

   ```bash
   kubectl -n unifabric-system get configmap -l app.kubernetes.io/component=unifabric
   kubectl -n unifabric-system get grafanadashboard -l app.kubernetes.io/component=unifabric
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

## 验证

在 Grafana 中打开 `rdma-node` 仪表盘,选择一个有 RDMA 网卡的节点。当面板展示该节点
的设备与端口计数数据时,验证通过。

![Unifabric RDMA 节点仪表盘](../images/unifabric-rdma-node-dashboard.png)

## 故障排查

### 没有任何 RDMA 指标

先确认前提条件成立:节点上有 RDMA 设备,且 Agent 在该节点上运行。

1. 查看 Agent 日志:

   ```bash
   kubectl -n unifabric-system logs ds/unifabric-agent -c agent
   ```

2. 确认节点上能看到 RDMA 设备:

   ```bash
   kubectl -n unifabric-system exec ds/unifabric-agent -c agent -- ls /sys/class/infiniband
   ```

3. 确认所在场景已识别 RDMA 网卡。SONiC RoCE 场景下,检查 `FabricNode` 资源:

   ```bash
   kubectl get fabricnodes
   kubectl get fabricnode <node-name> -o yaml
   ```

   Spectrum-X 和 InfiniBand 场景通过 NVIDIA Topograph 写入 Node 标签,不依赖
   `FabricNode` 和 `Switch` 资源。这两个场景直接进入下一步。

4. 确认 Agent 端点返回 Unifabric 指标:

   ```bash
   POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-agent -o jsonpath='{.items[0].status.podIP}')
   curl -s "http://${POD_IP}:8082/metrics" | grep '^unifabric_'
   ```

   如果端点没有返回任何 `unifabric_` 指标,回到 Agent 日志和 RDMA 设备检查。
   如果端点有指标但 Prometheus 中没有,检查 `unifabric-agent-metrics` Service、
   其 EndpointSlice 和 ServiceMonitor:

   ```bash
   kubectl -n unifabric-system get servicemonitor unifabric-agent-metrics -o yaml
   ```

### 没有 Pod 归属指标

1. 确认 `FabricNode.status.rdmaPods` 包含目标 Pod。
2. 确认该 Pod 使用 SR-IOV RDMA 设备,或运行在 host RDMA 模式。
3. 确认节点运行时是 containerd。非 host RDMA Pod 的归属读取 containerd 容器 ID,
   其他运行时会被跳过。

> **注意:** Pod 容器信息缺失或过期不会阻塞节点级 RDMA 指标导出。

### kind 标签为空

1. 检查 Helm values 中的 `fabricNode.scaleOutInterfaceSelector`、
   `fabricNode.storageInterfaceSelector` 和 `fabricNode.scaleUpInterfaceSelector`。
   选择器支持 `interface=eth*,!eth9` 和 `cidr=172.17.0.0/16` 等写法。
2. 如果未配置 scale-out 选择器,Agent 把可识别的接口置为 `scaleOut`。仍然没有
   `kind` 的接口未被识别,确认有选择器匹配它。

### 仪表盘没有数据

1. 确认 Prometheus 数据源能查询到 `unifabric_node_info`。
2. 确认仪表盘变量(如 `cluster`、`node`、`namespace`)与当前指标标签一致。

## 参考

- [RDMA 指标设计](../design/rdma-metrics.zh.md):完整的指标模型与标签集。
- [FabricNode 参考](../reference/fabricnode.zh.md):Pod 归属所读取的
  `status.rdmaPods` 字段。
