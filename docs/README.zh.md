# Unifabric 文档

English version: [README.md](./README.md)

Unifabric 面向 Kubernetes AI 集群，把 RDMA 网络中的交换机层级、节点距离和网卡状态转换成
Kubernetes 可以查询和消费的数据：

- Agent 采集节点 RDMA 网卡、LLDP 邻居和 Pod 网络归属。
- Controller 或 NVIDIA Topograph 发现网络拓扑，并把结果写入 Node label。
- `FabricNode`、`Switch` 和 `Topology` CR 提供集群级的网络状态视图。
- Prometheus metrics 和 Grafana dashboard 提供 RDMA 流量、拥塞、错误及 Pod 归因观测。

这些能力让调度器能够尽量把通信密集型 AI 工作负载放到网络距离更近的节点，并帮助运维人员
定位 RDMA 网络和工作负载流量问题。

## 快速开始

1. 按照[基础安装](./getting-started.zh.md)部署所有场景共用的 Controller、Agent 和观测组件。
2. 根据集群的物理网络选择拓扑发现场景：

| 场景 | 主要拓扑 | 发现方式 | 配置文档 |
| --- | --- | --- | --- |
| 通用 SONiC RoCE | Scale-out、RoCE storage | 节点及交换机 LLDP 协议拓扑识别 | [通用 SONiC RoCE](./getting-started-sonic-roce.zh.md) |
| Spectrum-X | Scale-out | NVIDIA Topograph + NetQ | [Spectrum-X fabric](./getting-started-spectrum-x.zh.md) |
| InfiniBand | Scale-out、Scale-up | NVIDIA Topograph | [InfiniBand fabric](./getting-started-infiniband.zh.md) |

场景文档只覆盖该网络需要的 Helm values 和接入步骤；版本、镜像、监控等通用配置统一由
基础安装文档维护。需要调整其他 Chart 参数时，可查看
[Helm values 参考](../chart/README.md)中的默认值和配置说明。

## 核心功能

### 拓扑发现

拓扑发现把物理网络中的邻接关系整理为分层性能域，并写入 Kubernetes Node label。
Unifabric 使用三类逻辑拓扑：

- **Scale-out 网络**：表示节点通过 leaf、spine、core 等交换机进行横向通信的层级。
  可以参考[通用 SONiC RoCE](./getting-started-sonic-roce.zh.md)、
  [Spectrum-X](./getting-started-spectrum-x.zh.md)或
  [InfiniBand](./getting-started-infiniband.zh.md)等不同场景的设置。
- **Scale-up 网络**：表示 GPU 间的 NVLink、NVSwitch 等高速互联域。
  可以参考 [InfiniBand 场景](./getting-started-infiniband.zh.md)进行设置。
- **Storage 网络**：表示计算节点访问存储服务所经过的独立 RDMA 网络。当前仅支持
  [通用 SONiC RoCE](./getting-started-sonic-roce.zh.md)设置。

发现结果以 Node label 提供给调度器，并汇总到只读 `Topology` CR。可以通过
[FabricNode、Switch、Topology API 参考](./reference/README.zh.md)查询发现结果。

- [拓扑可视化使用指南](./usage/topology-visualization.zh.md)

### 拓扑感知调度

拓扑发现生成的 Node label 可以被 Kueue、Volcano、KAI Scheduler 等调度系统消费，
让同一工作负载的 Pod 优先落在相同或相近的性能域，减少跨交换机层级通信。

- [拓扑感知调度与 Kueue TAS 示例](./usage/workload-tas.zh.md)
- [Topology API 参考](./reference/topology.zh.md)

### RDMA 流量与健康观测

Unifabric Agent 采集 RDMA 设备、端口、优先级、吞吐、拥塞和错误指标，并把流量归因到
Pod、namespace 和顶层 workload。Grafana dashboard 用于按集群、节点和工作负载排查
RDMA 流量与健康问题。

- [RDMA 可观测性使用指南](./usage/rdma-metrics.zh.md)

## 开发

- [NVAIR 开发环境指南](./development/dev-with-nvair.md)：搭建本地拓扑和端到端开发环境。

## 设计

- [Unifabric API 参考](./reference/README.zh.md)：查询 `FabricNode`、`Switch` 和 `Topology`。
- [Topology CRD 设计](./design/topology-crd.zh.md)：了解性能域、Node 路径和拓扑数据模型。
- [Scale-out 拓扑发现设计](./design/scaleout-topology.zh.md)：了解 Scale-out 拓扑的发现与构建过程。
- [RDMA 指标模型与 Pod 归因设计](./design/rdma-metrics.md)：了解指标定义与工作负载归因模型。
- [拓扑可视化设计](./design/topology-visualization.zh.md)：了解 Topology HTTP API，以及渲染它的 Grafana
  datasource/panel 插件。
