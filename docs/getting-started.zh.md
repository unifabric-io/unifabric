# 基础安装

English version: [getting-started.md](./getting-started.md)

## 简介与部署目标

本文使用 Helm 安装 Unifabric 基础组件:Controller、Agent 和观测组件。所有环境
指南都建立在这次安装之上。

完成本文后,集群将呈现以下结果:

- 每个节点都有对应的 `FabricNode` 资源,按用途列出其 RDMA 网卡:scale-out、
  storage 或 scale-up。
- 节点 RDMA 指标可以在 Grafana 的 `Unifabric RDMA Node` 看板中查看。

拓扑发现与具体环境相关。完成本文后,按[下一步](#下一步)选择对应物理网络的指南
继续。

## 前提条件

继续之前,确认以下各项均成立:

- [ ] 可以访问目标 Kubernetes 集群。
- [ ] 执行安装的主机上已安装 `kubectl` 和 Helm。

## 安装

下面的命令安装最新版本的 Unifabric。要选择特定版本,查看
[releases](https://github.com/unifabric-io/unifabric/releases) 页面。在可以用
`kubectl` 和 Helm 访问集群的主机上执行:

```bash
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unifabric-io/unifabric/releases/latest | grep '"tag_name":' | cut -d '"' -f4)
CHART_VERSION="${LATEST_TAG#v}"

helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --version "${CHART_VERSION}" \
  --namespace unifabric-system \
  --create-namespace \
  --wait
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

默认情况下,Unifabric 把节点上的所有 RDMA 网卡归类为 scale-out。如果节点上存在
不同用途的 RDMA 网卡,在安装命令中用 `--set` 追加对应的 selector。

例如,一个节点有 `eth1` 到 `eth6` 六张 RDMA 网卡,其中 `eth1` 到 `eth4` 承载
scale-out 流量,`eth5` 和 `eth6` 承载 storage 流量。标准 GPU 服务器
(如 DGX H100/B200、GB200/GB300 NVL72)的参考架构通常配备 2 张专用存储网卡。
在安装命令中追加:

```bash
--set fabricNode.storageInterfaceSelector="interface=eth5\,eth6"
```

Unifabric 会把 `eth5` 和 `eth6` 归类为 storage,其余四张 RDMA 网卡仍按默认规则
归类为 scale-out。选择器还支持通配符和 CIDR,例如 `interface=eth*,!eth9` 或
`cidr=172.17.0.0/16`。完整语法参见 [Helm values 参考](../chart/README.md)。

如果集群没有 Prometheus Operator 或 Grafana Operator,在安装命令中追加:

```bash
--set nodeMetrics.serviceMonitor.enabled=false \
--set grafanaDashboard.enabled=false
```

如果位于中国大陆,追加以下参数加速镜像拉取:

```bash
--set global.registry=m.daocloud.io \
--set controller.image.repository=ghcr.io/unifabric-io/unifabric-controller \
--set agent.image.repository=ghcr.io/unifabric-io/unifabric-agent \
--set nvidiaTopograph.image.repository=ghcr.io/nvidia/topograph
```

## 验证

### 1. 检查 FabricNode 网卡信息

确认 Controller 和 Agent Pod 正常运行,并且每个节点都存在对应的 `FabricNode`:

```bash
kubectl -n unifabric-system get pods
kubectl get fabricnodes
kubectl get fabricnode <node-name> -o yaml
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

确认 `status.totalNics`、`status.scaleOutNics` 和 `status.storageNics` 符合
节点的真实网卡配置。以前面的六张 RDMA 网卡为例,结果形如:

```yaml
apiVersion: unifabric.io/v1beta1
kind: FabricNode
metadata:
  name: node1
status:
  totalNics: 6
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
    - name: eth6
      rdmaDeviceName: mlx5_5
      rdma: true
      state: up
```

### 2. 查看节点 RDMA 指标

打开 Grafana 的 `Unifabric RDMA Node` 看板,在页面顶部选择集群和节点。展开
`Throughput`、`Basic`、`RoCE`、`QoS` 或 `Interface` 等分组。确认看板列出了该
节点的 RDMA device、网络接口及对应指标。

![Unifabric RDMA Node 看板](images/unifabric-rdma-node-dashboard.png)

看板中能看到目标节点的 device、`ifname` 和指标数据,说明 Prometheus 已正确采集
Agent 指标。

## 故障排查

### FabricNode 没有发现预期的 RDMA 网卡

先确认前提条件成立:

1. 确认 Agent Pod 正常运行:

   ```bash
   kubectl -n unifabric-system get daemonset unifabric-agent
   kubectl -n unifabric-system logs daemonset/unifabric-agent -c agent
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

2. 确认节点上的 RDMA 设备对 Agent 可见:

   ```bash
   kubectl -n unifabric-system exec daemonset/unifabric-agent -c agent -- \
     ls /sys/class/infiniband
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

3. 确认 `fabricNode.scaleOutInterfaceSelector`、`storageInterfaceSelector` 和
   `scaleUpInterfaceSelector` 匹配真实的接口名或 CIDR。
4. 确认网卡没有被错误归类。被 storage 或 scale-up selector 匹配的网卡不会出现在
   `status.scaleOutNics` 中。

### Agent 指标或 Grafana 看板没有数据

确认 Agent metrics 已启用,然后检查 Service 和 ServiceMonitor:

```bash
helm get values unifabric -n unifabric-system
kubectl -n unifabric-system get service unifabric-agent-metrics
kubectl -n unifabric-system get servicemonitor unifabric-agent-metrics
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

如果安装时关闭了 `nodeMetrics.serviceMonitor.enabled`,没有 `ServiceMonitor`
属于正常情况。直接访问 Agent metrics 端点,确认采集器已经产生指标:

```bash
POD_IP=$(kubectl -n unifabric-system get pod \
  -l app.kubernetes.io/component=unifabric-agent \
  -o jsonpath='{.items[0].status.podIP}')

curl -s "http://${POD_IP}:8082/metrics" | grep '^unifabric_'
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

- 如果端点没有返回指标,确认 `nodeMetrics.enabled=true`,然后查看 Agent 日志和
  `/sys/class/infiniband` 下的 RDMA 设备。
- 如果端点有指标,但 Grafana 看板没有数据,确认 Prometheus 已发现
  `unifabric-agent-metrics` target,且 Grafana 数据源、集群和节点变量选择正确。

## 卸载

删除 Helm release:

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

如果不再需要 CRD,删除它们。此操作会同时删除所有 `FabricNode`、`Switch` 和
`Topology` 资源:

```bash
kubectl delete crd fabricnodes.unifabric.io switches.unifabric.io topologies.unifabric.io
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

## 下一步

按物理网络选择对应指南继续。每篇指南都使用 `helm upgrade --reuse-values`,只修改
所选网络相关的 Helm values,其他基础安装配置保持不变。

- 使用运行 SONiC 的交换机组网,阅读
  [在通用 SONiC RoCE 网络部署 Unifabric](./getting-started-sonic-roce.zh.md)。
- 使用 Spectrum-X 交换机组网并由 NetQ 管理网络,阅读
  [在 Spectrum-X 网络部署 Unifabric](./getting-started-spectrum-x.zh.md)。
- 使用 NVIDIA InfiniBand 组网,阅读
  [在 InfiniBand 网络部署 Unifabric](./getting-started-infiniband.zh.md)。

拓扑发现正常工作后,继续阅读功能使用文档:

- 在调度器中使用这些 label:参见 [Kueue TAS 工作负载示例](./usage/workload-tas.zh.md)。
- 渲染拓扑:参见[拓扑可视化指南](./usage/topology-visualization.zh.md)。
- 观测 RDMA 指标:参见 [RDMA 指标指南](./usage/rdma-metrics.zh.md)。
- 调整安装配置:参见 [Helm values 参考](../chart/README.md)。
