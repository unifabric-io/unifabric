# Unifabric 文档

English version: [README.md](./README.md)

Unifabric 发现 Kubernetes AI 集群的 RDMA 网络拓扑并使其可观测。它把拓扑 label 写入
Node 供调度器消费，通过 `FabricNode`、`Switch` 和 `Topology` 资源暴露网络状态，
并把 RDMA 指标归因到 Pod 和 workload。组件与数据流的完整说明见
[架构总览](./concepts/architecture.zh.md)。

推荐阅读路径：按快速开始文档完成安装，按功能使用文档用好每个功能，在参考手册中查字段，
通过概念和设计文档理解系统如何运作。

## 概念

- [架构总览](./concepts/architecture.zh.md)：组件、核心概念、数据流与使用场景。
- [拓扑发现](./concepts/topology-discovery.zh.md)：逻辑拓扑、发现方式与发现结果。

## 快速开始

按照[部署 Unifabric](./getting-started.zh.md) 安装 Controller、Agent 和观测组件。默认
安装面向 SONiC RoCE 网络，通过 LLDP 发现拓扑；InfiniBand 或 Spectrum-X 网络只需按
指南修改少量 Helm 参数：

| 场景 | 主要拓扑 | 发现方式 |
| --- | --- | --- |
| SONiC RoCE（默认） | Scale-out、RoCE storage | 节点及交换机 LLDP 协议拓扑识别 |
| InfiniBand | Scale-out、Scale-up | NVIDIA Topograph |
| Spectrum-X | Scale-out | NVIDIA Topograph + NetQ |

SONiC 交换机接入方式、label 语义和发现链路排查，参见
[拓扑发现指南](./usage/topology/README.zh.md)。需要调整其他 Chart 参数时，可查看
[Helm values 参考](../chart/README.md)中的默认值和配置说明。

## 功能使用

- [拓扑可视化](./usage/topology/topology-visualization.zh.md)：在 Grafana 中渲染发现的网络拓扑。
- [RDMA 指标](./usage/rdma-metrics.zh.md)：RDMA 流量、拥塞和错误指标，归因到 Pod 和 workload。
- [RDMA 流归因](./usage/rdma-flow.zh.md)：用 eBPF 把 RDMA 发送归因为 Pod 间流，发布
  `RDMAEndpoint` 并存储逐次发送事件。
- [拓扑感知调度](./usage/topology/README.zh.md#在调度器中使用-node-label)：配合 Kueue TAS 等调度器做拓扑感知调度。
- [Switch Agent 部署](./usage/topology/roce-lldp-discovery.zh.md)：在 SONiC 交换机上部署 Switch Agent。

## 参考手册

- [API 参考索引](./reference/README.zh.md)
- [FabricNode](./reference/fabricnode.zh.md)
- [Switch](./reference/switch.zh.md)
- [Topology](./reference/topology.zh.md)
- [RDMAEndpoint](./reference/rdmaendpoint.zh.md)

## 设计文档

- [FabricNode 设计](./design/fabricnode.zh.md)
- [RDMA 指标模型与 Pod 归因设计](./design/rdma-metrics.zh.md)
- [eBPF RDMA 流归因设计](./design/rdma-flow.zh.md)
- [Scale-out 拓扑发现设计](./design/scaleout-topology.zh.md)
- [Topology CRD 设计](./design/topology-crd.zh.md)
- [拓扑可视化设计](./design/topology-visualization.zh.md)

## 开发

- [NVAIR 开发环境指南](./development/dev-with-nvair.md)：搭建本地拓扑与端到端开发环境。

## 路线图

- [路线图](./roadmap.zh.md)：规划中的功能与未来方向。
