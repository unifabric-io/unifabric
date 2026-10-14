# Unifabric 架构

English version: [architecture.md](./architecture.md)

## 概述

Unifabric 是面向 Kubernetes AI 集群的 RDMA 网络拓扑发现与可观测系统。RDMA
（服务器之间绕过 CPU 拷贝的直接内存访问）运行在 RoCE 或 InfiniBand 网络上。
Unifabric 把这张网络的层级结构、节点距离和网卡状态转换成 Kubernetes 可以查询的数据：
为调度器生成 Node label，为运维人员和工具生成自定义资源，为监控生成 Prometheus
指标和 Grafana dashboard。

设计目标：

- 让 Kubernetes 看见物理 RDMA 网络，无需人工维护布线记录。
- 把分层拓扑 label 写入 Node，支撑拓扑感知调度。
- 把 RDMA 流量、拥塞和错误归因到产生它们的 Pod 和 workload。
- 通过可插拔的发现方式支持 SONiC RoCE、NVIDIA Spectrum-X 和 InfiniBand 网络。

Unifabric 不实现调度器，也不实现放置策略。它只生产拓扑数据，供 Kueue TAS、Volcano、
KAI Scheduler 等调度器消费。它也不管理交换机配置，只观测网络。

## 整体架构

![Unifabric 架构图](../images/architecture.png)

Unifabric 运行在 Kubernetes 之上，位于物理 RDMA 网络与调度、观测体系之间。
它由三个组件组成。

Controller 是一个 Deployment。它基于 `FabricNode` 和 `Switch` 数据计算拓扑，
或导入 NVIDIA Topograph 发现的拓扑。它把分层拓扑 label 写入 Kubernetes Node，
并维护只读的 `Topology` 资源。它还提供可视化插件消费的 Topology HTTP API。

Agent 是一个 DaemonSet。它在每个节点上采集 RDMA 网卡、网卡的 LLDP 邻居，以及使用
RDMA 设备的 Pod，写入 `FabricNode` 资源。LLDP 是一种设备向链路对端通告自身信息的协议。
Agent 还向 Prometheus 暴露该节点的 RDMA 指标。

Switch Agent 是可选组件。它以容器、systemd 服务或二进制形式运行在 SONiC 交换机上，
通过 gRPC 向 Controller 上报交换机侧的 LLDP 邻居快照。当网络存在多级交换机时，
它支撑交换机到交换机的拓扑发现。

## 核心概念

| 术语 | 定义 |
| --- | --- |
| `FabricNode` | 集群级资源，每个 Kubernetes Node 对应一个，由 Agent 创建和更新。它记录节点的 RDMA 网卡、网卡的 LLDP 邻居、节点 IP、节点角色，以及使用 RDMA 设备的 Pod。它是拓扑发现的节点侧输入。 |
| `Switch` | 集群级资源，描述一台物理交换机。它记录交换机的管理端点、网络角色，以及 Switch Agent 上报的 LLDP 邻居快照。它是拓扑发现的交换机侧输入。 |
| `Topology` | 集群级只读资源，汇总发现结果。其 status 列出性能域、域之间的父子关系，以及每个 Node 的域路径。存在三个固定名字：`scaleout`、`scaleup` 和 `storage`。 |
| 性能域 | 在同一网络层级内通信的一组 Node 及服务它们的交换机。域构成一棵树。Tier 1 距离 Node 最近，spine、core 等更高层级是低层级的父域。 |
| Scale-out 网络 | 跨 leaf、spine、core 交换机层级的节点间 RDMA 网络。跨节点 GPU 训练流量使用它。发现结果写入 `scale-out.unifabric.io/tier-1` 等 label。 |
| Scale-up 网络 | NVLink、NVSwitch 等高带宽 GPU 互联域。同一域内的 GPU 通信不经过 scale-out 网络。 |
| Storage 网络 | 计算节点与存储服务之间的独立 RDMA 网络。Unifabric 单独建模它，使存储网卡不干扰计算拓扑。 |
| Node 拓扑 label | 以分层 label 形式写入 Kubernetes Node 的发现结果，例如 `scale-out.unifabric.io/tier-1=tier1-group1`。拓扑感知调度器消费这些 label。 |

各概念的关系如下。Agent 生产 `FabricNode` 资源，Switch Agent 补充 `Switch` 资源。
Controller 消费两者，计算性能域，以 Node 拓扑 label 写出，并汇总到 `Topology` 资源。
调度器读取 label。用户和可视化读取 `Topology` 资源。

## 工作流程

### 拓扑发现与消费

1. 采集。Agent 在每个节点上扫描 RDMA 网卡，把每张网卡归类为 scale-out、scale-up
   或 storage。它读取每张网卡的 LLDP 邻居，把结果写入 `FabricNode` status。
   在部署了 Switch Agent 的网络中，每台交换机向 Controller 上报自己的 LLDP 邻居快照，
   Controller 把它存入对应的 `Switch` status。
2. 计算。Controller 把 `FabricNode` LLDP 中的节点到交换机链路，与 `Switch` 快照或
   管理员声明的邻居 annotation 中的交换机间链路合并。它把共享下游 Node 的交换机归入
   分层性能域。在 Spectrum-X 和 InfiniBand 场景中，导入的 NVIDIA Topograph 结果替代这一步。
3. 发布。Controller 把每个 Node 的完整域路径写成分层 Node label，并把全部 label
   汇总到只读的 `Topology` status。
4. 消费。拓扑感知调度器读取 Node label 来聚合放置工作负载。运维人员和可视化 dashboard
   读取 `Topology`、`FabricNode` 和 `Switch` 资源。

### RDMA 可观测

1. Agent 从每个节点采集 RDMA 设备、端口和优先级计数器，覆盖吞吐、拥塞和错误。
2. Agent 把每个 RDMA 设备映射到使用它的 Pod，并解析每个 Pod 的顶层 workload。
   因此每条指标都携带 Pod、namespace 和 workload 归因。
3. Prometheus 抓取 Agent 的指标端点。Grafana dashboard 呈现节点级和 workload 级视图。

## 使用场景

- 拓扑感知调度。Kueue TAS、Volcano 或 KAI Scheduler 消费分层 Node label，把一个训练
  任务的所有 Pod 放到同一 leaf 或 spine 组下，减少跨层流量，提升 GPU 利用率。
- RDMA 流量与健康监控。运维人员在 Grafana 中观察节点级和 workload 级的 RDMA 吞吐、
  拥塞和错误指标。链路劣化和异常 workload 在拖慢训练任务之前即可被发现。
- 网络排障。任务变慢时，运维人员查看受影响节点的 `FabricNode` 资源和拓扑图。
  网卡状态、LLDP 邻居和域归属能定位故障的网卡、链路或交换机。
- 网络资产盘点。`FabricNode`、`Switch` 和 `Topology` 资源通过标准 Kubernetes API
  为平台团队提供 RDMA 网卡、交换机连接和网络层级的实时清单。
- 多网络集群。同时存在计算和存储 RDMA 网络的集群分别建模两张网络。
  存储流量和拓扑不会干扰计算侧的调度决策。
