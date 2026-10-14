# 拓扑发现

English version: [topology-discovery.md](./topology-discovery.md)

## 概述

拓扑发现把 RDMA 网络中的物理邻接关系转换成分层性能域。性能域是在同一网络层级内通信的
一组 Node 及服务它们的交换机。Controller 把每个 Node 的域路径写成 Node label，
并把全部结果汇总到只读的 `Topology` 资源。拓扑感知调度器消费这些 label，
把通信密集型工作负载放到网络距离更近的节点上。

本页说明发现结果是什么，以及每种发现方式如何工作。组件职责见
[架构总览](./architecture.zh.md)。安装步骤见[基础安装](../getting-started.zh.md)
和各场景文档。

## 逻辑拓扑

Unifabric 建模三类逻辑拓扑：

- Scale-out 网络：跨 leaf、spine、core 交换机层级的节点间 RDMA 网络。发现结果写入
  `scale-out.unifabric.io/tier-1`（距节点最近的层级）、`scale-out.unifabric.io/tier-2`、
  `scale-out.unifabric.io/tier-3` 等 Node label。
- Scale-up 网络：NVLink、NVSwitch 等高带宽 GPU 互联域。同一域内的 GPU 通信不经过
  scale-out 网络。
- Storage 网络：计算节点与存储服务之间的独立 RDMA 网络。Unifabric 单独建模它，
  使存储网卡不干扰计算拓扑。

## 发现方式

发现方式取决于集群的物理网络：

| 场景 | 主要拓扑 | 发现方式 | 配置文档 |
| --- | --- | --- | --- |
| 通用 SONiC RoCE | Scale-out、RoCE storage | 节点与交换机 LLDP 采集 | [通用 SONiC RoCE](../getting-started-sonic-roce.zh.md) |
| NVIDIA Spectrum-X | Scale-out | 从 NVIDIA Topograph + NetQ 导入 | [Spectrum-X fabric](../getting-started-spectrum-x.zh.md) |
| InfiniBand | Scale-out、Scale-up | 从 NVIDIA Topograph 导入 | [InfiniBand fabric](../getting-started-infiniband.zh.md) |

### 基于 LLDP 的发现

LLDP 是一种设备向链路对端通告自身信息的协议。在通用 SONiC RoCE 场景中，
Agent 在每个节点上读取每张 RDMA 网卡的 LLDP 邻居，写入 `FabricNode` 资源。
可选的 Switch Agent 把交换机侧的 LLDP 邻居快照上报到 `Switch` 资源。
Controller 把节点到交换机链路与交换机间链路合并，计算出 leaf、spine、core 域。
在没有部署 Switch Agent 的网络中，管理员在 `Switch` 资源上声明的邻居 annotation
替代交换机侧快照。

### 基于 Topograph 的发现

NVIDIA Topograph 是一个从厂商数据源发现网络拓扑的外部服务。Spectrum-X 场景中，
Topograph 从 NetQ 读取网络拓扑。InfiniBand 场景中，Topograph 使用 `infiniband-k8s`
provider，通过 `ibnetdiscover` 读取网络拓扑。这两个场景中，Controller 导入
Topograph 的结果，不再基于 LLDP 数据计算域。

## 发现结果

每种发现方式产出相同的结果：

- Node label：每个 Node 的分层域路径，以 label 形式写入，供调度器消费。参见
  [拓扑感知调度](../usage/workload-tas.zh.md)。
- `Topology` 资源：集群级只读聚合，呈现性能域与节点路径。参见
  [API 参考](../reference/README.zh.md)。
- 可视化：Grafana 渲染发现的网络拓扑。参见
  [拓扑可视化](../usage/topology-visualization.zh.md)。

## 延伸阅读

- [Topology CRD 设计](../design/topology-crd.zh.md)：性能域、Node 路径与拓扑数据模型。
- [Scale-out 拓扑发现设计](../design/scaleout-topology.zh.md)：Controller 构建
  Scale-out 拓扑的过程。
