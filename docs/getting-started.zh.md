# 部署 Unifabric

English version: [getting-started.md](./getting-started.md)

## 简介与部署目标

本文使用 Helm 安装 Unifabric 基础组件:Controller、Agent 和观测组件。默认安装
面向 SONiC RoCE 网络;如果集群使用 InfiniBand 或 Spectrum-X,需要修改少量 Helm
参数。

完成本文后,集群将呈现以下结果:

- 每个节点都有对应的 `FabricNode` 资源,按用途列出其 RDMA 网卡:scale-out、
  storage 或 scale-up。
- 拓扑发现和 RDMA 指标采集组件已就绪。Node 拓扑 label 和 RDMA 指标属于功能
  结果,分别参见[拓扑发现指南](./usage/topology/README.zh.md)和
  [RDMA 指标指南](./usage/rdma-metrics.zh.md)。

## 前提条件

继续之前,确认以下各项均成立:

- [ ] 可以访问目标 Kubernetes 集群。
- [ ] 集群节点配备 RDMA 网卡,并接入 SONiC RoCE、InfiniBand 或 NVIDIA
      Spectrum-X 其中一种网络,节点上已安装 RDMA 驱动(如 NVIDIA DOCA-OFED)。
- [ ] 执行安装的主机上已安装 `kubectl` 和 Helm(v3.13 及以上)。
- [ ] 集群已安装 Prometheus Operator 和 Grafana Operator(v5 及以上,提供
      `grafana.integreatly.org/v1beta1` API),用于采集 RDMA 指标和加载
      Grafana 看板。拓扑可视化还要求 Grafana 12.1 及以上,参见
      [拓扑可视化指南](./usage/topology/topology-visualization.zh.md)。

## 安装

下面的命令安装最新版本的 Unifabric,Helm 会自动解析最新的 chart 版本。要安装
特定版本,追加 `--version <x.y.z>`,可用版本见
[releases](https://github.com/unifabric-io/unifabric/releases) 页面。

在可以用
`kubectl` 和 Helm 访问集群的主机上执行:

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --create-namespace \
  --wait
```

1. Chart 默认把 `topoDiscovery.scaleOut.mode` 设置为 `unifabric-roce`,适用于
SONiC 交换机承载 RoCE 网络的集群。RoCE 是在融合以太网上运行的 RDMA。Unifabric
通过 LLDP 邻居发现 scale-out 拓扑。LLDP 是一种设备向链路对端通告自身信息的
协议,确认每台交换机都启用了 LLDP。如果集群使用 InfiniBand 或 Spectrum-X
网络,按[其他网络环境](#其他网络环境)修改拓扑发现参数。

> 只依靠节点侧 LLDP 时,发现结果只包含 tier 1(节点直连的 leaf 交换机)。要发现
leaf 以上的 spine、core 等层级,在安装完成后按
[拓扑发现指南](./usage/topology/README.zh.md)声明上层交换机,或在交换机上
运行 switch-agent 自动发现。

2. 默认情况下,Unifabric 把节点上的所有 RDMA 网卡归类为 scale-out。如果节点上存在
不同用途的 RDMA 网卡,完成基础安装后,用 `helm upgrade --install` 追加对应的
selector。

> 例如,一个节点有 `eth1` 到 `eth6` 六张 RDMA 网卡,其中 `eth1` 到 `eth4` 承载
scale-out 流量,`eth5` 和 `eth6` 承载 storage 流量。标准 GPU 服务器
(如 DGX H100/B200、GB200/GB300 NVL72)的参考架构通常配备 2 张专用存储网卡。
执行以下命令,同时打开 Storage 网络的拓扑发现:

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set fabricNode.storageInterfaceSelector="interface=eth5\,eth6" \
  --set topoDiscovery.storage.mode=unifabric-roce \
  --wait
```

> Unifabric 会把 `eth5` 和 `eth6` 归类为 storage,其余四张 RDMA 网卡仍按默认规则
归类为 scale-out。选择器还支持通配符和 CIDR,例如 `interface=eth*,!eth9` 或
`cidr=172.17.0.0/16`。完整语法参见 [Helm values 参考](../chart/README.md)。

3. 如果位于中国大陆,执行以下命令加速镜像拉取:

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set global.registry=m.daocloud.io \
  --set controller.image.repository=ghcr.io/unifabric-io/unifabric-controller \
  --set agent.image.repository=ghcr.io/unifabric-io/unifabric-agent \
  --set nvidiaTopograph.image.repository=ghcr.io/nvidia/topograph \
  --wait
```

## 其他网络环境

默认安装面向 SONiC RoCE 网络,并使用
[RoCE 网络中 LLDP 发现](./usage/topology/roce-lldp-discovery.zh.md)。如果集群使用其他网络设备,
保持相同的安装流程。只修改对应发现方式的拓扑发现参数:

- InfiniBand 网络:参见 [InfiniBand 网络发现](./usage/topology/infiniband-discovery.zh.md)。
- 由 NetQ 管理的 Spectrum-X 网络:参见 [RoCE 网络中 NVIDIA NetQ 发现](./usage/topology/roce-netq-discovery.zh.md)。

各网络在不同网络设备下应选择哪种发现方式,参见
[网络拓扑发现](./usage/topology/README.zh.md#按网络选择发现方式)。

## 验证

### 检查 FabricNode 网卡信息

确认 Controller 和 Agent Pod 正常运行,并且每个节点都存在对应的 `FabricNode`:

```bash
kubectl -n unifabric-system get pods
kubectl get fabricnodes
kubectl get fabricnode <node-name> -o yaml
```

```text
NAME                                    READY   STATUS    RESTARTS   AGE
unifabric-agent-7tkfm                   2/2     Running   0          3m12s
unifabric-agent-x5d9z                   2/2     Running   0          3m12s
unifabric-controller-6f9c7b5d44-l2wqv   1/1     Running   0          3m12s

NAME    TOTALNICS   READY   ROLE   NODEIP        TOPOLOGIES
node1   6           True    GPU    10.0.10.11
node2   6           True    GPU    10.0.10.12
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

Pod 正常运行且 `FabricNode` 状态符合预期,说明部署已经完成。

## 下一步

部署完成后,按功能使用文档继续:

- 深入拓扑发现:声明 SONiC 上层交换机、部署 switch-agent、理解 label 语义与
  `Topology` 资源,参见[拓扑发现指南](./usage/topology/README.zh.md)。
- 在调度器中使用这些 label:参见[在调度器中使用 Node label](./usage/topology/README.zh.md#在调度器中使用-node-label)。
- 渲染拓扑:参见[拓扑可视化指南](./usage/topology/topology-visualization.zh.md)。
- 观测 RDMA 指标:参见 [RDMA 指标指南](./usage/rdma-metrics.zh.md)。
- 调整安装配置:参见 [Helm values 参考](../chart/README.md)。

## 故障排查

### FabricNode 没有发现预期的 RDMA 网卡

先确认前提条件成立:

1. 确认 Agent Pod 正常运行:

   ```bash
   kubectl -n unifabric-system get daemonset unifabric-agent
   kubectl -n unifabric-system logs daemonset/unifabric-agent -c agent
   ```

   ```text
   NAME              DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
   unifabric-agent   2         2         2       2            2            <none>          12m

   time=2026-10-20T09:52:31.104Z level=INFO msg="starting agent" node=node1
   time=2026-10-20T09:52:31.388Z level=INFO msg="discovered RDMA device" device=mlx5_0 ifname=eth1 kind=scaleOut
   time=2026-10-20T09:52:31.391Z level=INFO msg="discovered RDMA device" device=mlx5_4 ifname=eth5 kind=storage
   ```

2. 确认节点上的 RDMA 设备对 Agent 可见:

   ```bash
   kubectl -n unifabric-system exec daemonset/unifabric-agent -c agent -- \
     ls /sys/class/infiniband
   ```

   ```text
   mlx5_0  mlx5_1  mlx5_2  mlx5_3  mlx5_4  mlx5_5
   ```

3. 确认 `fabricNode.scaleOutInterfaceSelector`、`storageInterfaceSelector` 和
   `scaleUpInterfaceSelector` 匹配真实的接口名或 CIDR。
4. 确认网卡没有被错误归类。被 storage 或 scale-up selector 匹配的网卡不会出现在
   `status.scaleOutNics` 中。

## 卸载

删除 Helm release:

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

```text
release "unifabric" uninstalled
```

如果不再需要 CRD,删除它们。此操作会同时删除所有 `FabricNode`、`Switch` 和
`Topology` 资源:

```bash
kubectl delete crd fabricnodes.unifabric.io switches.unifabric.io topologies.unifabric.io
```

```text
customresourcedefinition.apiextensions.k8s.io "fabricnodes.unifabric.io" deleted
customresourcedefinition.apiextensions.k8s.io "switches.unifabric.io" deleted
customresourcedefinition.apiextensions.k8s.io "topologies.unifabric.io" deleted
```
