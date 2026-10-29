# InfiniBand 网络发现

English version: [infiniband-discovery.md](./infiniband-discovery.md)

## 简介

InfiniBand 是独立于以太网的高速互联技术。Subnet Manager 维护整个 fabric 的拓扑。
Unifabric 使用 NVIDIA Topograph 的 `infiniband-k8s` provider。Topograph 在 GPU 节点
的 Pod 中运行 `ibnetdiscover`。它读取 IB 子网内的交换机、HCA 和链路。对于 Scale-up
网络,Topograph 读取 GPU Operator 写在每个节点上的 `nvidia.com/gpu.clique` label。
本文说明如何配置 InfiniBand 网络发现。

| 项目 | 说明 |
| --- | --- |
| 适用网络 | Scale-out、Storage |
| 拓扑来源 | Scale-out:`ibnetdiscover`。Scale-up:GPU Operator 的 clique label。 |
| 能发现的层级 | Scale-out:leaf、spine、core。Scale-up:tier 1,即 NVLink 域。 |
| 交换机侧要求 | Subnet Manager 在运行。 |
| 集群侧要求 | GPU 节点上能运行 `ibnetdiscover`。Scale-up 网络需要 GPU Operator。 |
| Helm 配置 | `topoDiscovery.scaleOut.mode=nv-topograph` 和 `topoDiscovery.scaleUp.mode=nv-topograph`。 |

对于 Scale-out 网络,label 值是由 InfiniBand 交换机 GUID 得出的名称,例如
`S-fc6a1c03006636c0`。

限制:

- 本方式不创建 `Switch` 资源。`Topology` 资源和 Node label 是完整的。但在拓扑可视化
  中,每个性能域的交换机成员卡片为空。参见
  [可选:补充 Switch 资源](#可选补充-switch-资源)。
- Unifabric 暂不支持在 InfiniBand 网络发现得到的拓扑中区分 Scale-out 网络和 Storage
  网络。发现结果只作为 Scale-out 网络。如果 Storage 网络是以太网,为它使用
  [RoCE 网络中 LLDP 发现](./roce-lldp-discovery.zh.md)或[自定义网络拓扑](./custom-network-topology.zh.md)。
  如果 Storage 网络是 InfiniBand,目前不支持为它做拓扑发现。
- Scale-up 网络的 NVLink 域需要 `ibnetdiscover` 成功运行。在没有 InfiniBand 的集群
  上,本方式不能单独提供 NVLink 域。

各网络在不同网络设备下应选择哪种发现方式,参见
[网络拓扑发现](./README.zh.md#按网络选择发现方式)。

## 前提条件

- 已按[部署 Unifabric](../../getting-started.zh.md)完成部署。
- 集群使用 NVIDIA InfiniBand 网络。例如,Mellanox 网卡运行在 IB 模式,并连接 IB
  交换机。节点上存在 IB 模式的 RDMA 设备。
- IB fabric 中运行 Subnet Manager。
- 如果要发现 Scale-up 网络,集群已安装 GPU Operator。节点上也有
  `nvidia.com/gpu.clique` label。

## 配置

为 Scale-out 网络和 Scale-up 网络选择 NVIDIA Topograph 作为拓扑写入器:

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set topoDiscovery.scaleUp.mode=nv-topograph \
  --set topoDiscovery.scaleOut.mode=nv-topograph \
  --wait
```

`nvidiaTopograph.provider.name` 的默认值是 `infiniband-k8s`。不需要设置它。如果不需要
发现 Scale-up 网络,去掉 `--set topoDiscovery.scaleUp.mode=nv-topograph`。

## 验证

确认 Node 上出现 scale-out label:

```bash
kubectl get nodes -L scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2
```

如果配置了 Scale-up 网络,确认 Node 上出现 scale-up label:

```bash
kubectl get nodes -L scale-up.unifabric.io/tier-1
```

label 的含义,以及 `Topology` 资源的查看方法,参见
[网络拓扑发现](./README.zh.md#查看-topology-与-node-label)。

## 可选:补充 Switch 资源

本方式不创建 `Switch` 资源。如果需要拓扑可视化显示交换机,参见
[创建 Switch 实例](./topograph-switch-resources.zh.md)。

## 故障排查

### Node 上没有拓扑 label

1. 确认 Topograph Deployment 在运行,并查看日志:

   ```bash
   kubectl -n unifabric-system get deployment unifabric-nvidia-topograph
   kubectl -n unifabric-system logs deployment/unifabric-nvidia-topograph --tail 100
   ```

2. 确认日志显示 `ibnetdiscover` 正常运行。

## 参考

- [网络拓扑发现](./README.zh.md):如何为各网络选择发现方式,以及如何查看结果。
- [RoCE 网络中 NVIDIA NetQ 发现](./roce-netq-discovery.zh.md):另一种同样使用 NVIDIA Topograph 的发现方式。
- [创建 Switch 实例](./topograph-switch-resources.zh.md):可选步骤。
- [Topograph InfiniBand provider](https://github.com/dsx-ai-factory/topograph/blob/main/docs/providers/infiniband.md):
  上游文档。
