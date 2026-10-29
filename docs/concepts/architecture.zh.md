# Unifabric 架构

English version: [architecture.md](./architecture.md)

## 概述

Unifabric 是面向 Kubernetes AI 集群高速网络的云原生可观测系统。AI 集群依赖三张高速网络：

- Scale-out 网络：通过 RoCE 或 InfiniBand 承载跨节点的 GPU 通信。
- Scale-up 网络：GPU 之间的高速互联，例如 NVIDIA NVLink。它连接单个节点内的 GPU，
  或同一多节点 NVLink 域中多个节点的 GPU。
- 存储网络：承载计算节点与高速存储之间的 RDMA 流量。

这些网络由多家厂商的交换机和网卡组成。各厂商的工具不与 Kubernetes 对接，而常见的
云原生可观测体系围绕 TCP/IP 构建。RDMA 绕过内核网络协议栈，这套体系无法观测到它。
因此 Kubernetes 无法把网络问题归因到具体的 Pod 或任务。Unifabric 通过三项能力补上这块空白：

- 拓扑识别：发现节点与交换机、交换机与交换机之间的连接，记录每个节点在网络中的位置。
- AI 高速网络流量观测：采集 RDMA 流量、拥塞和错误数据，并归因到 Pod、namespace 和 workload。
- 网络健康检查：持续报告 LLDP 链路和交换机连接的健康状态。

Unifabric 以统一的 Kubernetes 资源模型呈现结果：自定义资源、Node label 和 Prometheus
指标。无论设备来自哪家厂商，模型都相同。

设计目标：

- 通过 Kubernetes API 提供物理网络拓扑，无需人工维护布线记录。
- 为拓扑感知调度器提供准确的 Node label，并随拓扑变化自动更新。
- 在不修改应用的前提下，把网络流量归因到 workload。
- 以同一套数据模型支持 SONiC RoCE、NVIDIA Spectrum-X 和 InfiniBand 网络。每张网络
  按网络设备选择相应的发现方式。

非目标：

- Unifabric 不是调度器，不决定 Pod 的放置。它为 Kueue、Volcano、KAI Scheduler 等
  调度器提供拓扑数据。
- Unifabric 不配置交换机或网卡，只观测它们。
- Unifabric 不替代 NVIDIA NetQ 等厂商网络管理平台，但可以从中导入拓扑。

## 整体架构

![Unifabric 架构图](../images/architecture.png)

架构图分为四层。基础设施层包含交换机和 RDMA 节点。拓扑来源层包含 LLDP，以及可选的
NVIDIA Topograph 与 NetQ。Unifabric 运行在 Kubernetes 中，写入 Node label、自定义资源和
指标。调度器、Prometheus 和 Grafana 消费这些结果。

Unifabric 包含以下组件。除 switch-agent 外，其余组件都由 Helm chart 部署。

| 组件 | 形态 | 职责 |
| --- | --- | --- |
| Controller | Deployment | 基于 `FabricNode` 和 `Switch` 数据计算拓扑。把分层拓扑 label 写入 Node，把域 label 写入 `Switch` 资源，并把这些 label 聚合为 `Topology` 资源。它还为 Grafana 拓扑插件提供只读的 Topology HTTP API。 |
| Agent | DaemonSet | 在每个节点上发现 RDMA 网卡，并把网卡分类为 Scale-out 网卡或存储网卡。读取每块网卡的 LLDP 邻居，找出使用 RDMA 设备的 Pod，把结果写入该节点的 `FabricNode` 资源。暴露带 Pod 与 workload 归因的 RDMA 指标。启用 eBPF 流量归因后，还会发布 `RDMAEndpoint` 资源和 Pod 到 Pod 的流量指标。 |
| switch-agent | 交换机上的容器或 systemd 服务（可选） | 从交换机上的 `lldpd` 守护进程读取交换机的 LLDP 邻居，并通过端口 `8090` 上的 mTLS gRPC 提供这些数据。Controller 订阅每个 switch-agent。只有 RoCE 网络中 LLDP 发现需要它。 |
| NVIDIA Topograph | Deployment（可选） | 从 NVIDIA NetQ 读取 fabric 拓扑，或在 InfiniBand 网络中通过 `ibnetdiscover` 读取拓扑，并使用 Unifabric 的 label key 写入分层 Node label。只有当某张网络使用 `nv-topograph` 发现模式时，chart 才部署它。 |
| 流量事件收集器 | OpenTelemetry Collector Deployment（可选） | 通过 OTLP 接收各 Agent 发送的逐次发送 RDMA 流量事件，并写入 ClickHouse 或 Elasticsearch。 |
| Grafana 看板与插件 | Grafana Operator 资源（可选） | 提供 RDMA 指标看板、流量看板，以及拓扑数据源插件和面板插件。 |

## 核心概念

| 术语 | 定义 |
| --- | --- |
| Scale-out 网络 | 节点之间的 GPU 网络，采用 leaf-spine 或更多层级的 Clos 架构。它的 Node label 前缀为 `scale-out.unifabric.io`，`Topology` 资源为 `scaleout`。 |
| Scale-up 网络 | GPU 高速互联，例如 NVLink。只有多节点 NVLink 系统（例如 GB200 NVL72）需要拓扑发现。它的 Node label 为 `scale-up.unifabric.io/tier-1`，`Topology` 资源为 `scaleup`。 |
| 存储网络 | 计算节点与存储之间的 RDMA 网络。Unifabric 把它与 Scale-out 网络分开建模，存储网卡不会改变计算拓扑。它的 Node label 前缀为 `storage.unifabric.io`，`Topology` 资源为 `storage`。 |
| 性能域 | 同一张网络中某一层级上的一组节点，以及为它们服务的交换机。域内流量不经过更高层级。域构成一棵树：tier 1 是 leaf 层，tier 2 是 spine 层，tier 3 是 core 层。在 Scale-up 网络中，tier 1 是 NVLink 域。 |
| Node 拓扑 label | 节点的域路径，每个层级写一个 label，例如 `scale-out.unifabric.io/tier-1=tier1-group1`。两个节点某个 label 的值相同，表示它们在该层级属于同一个域。 |
| 发现模式 | 一张网络的拓扑来源，由 Helm 值 `topoDiscovery.<network>.mode` 设置。`unifabric-roce` 基于 LLDP 数据计算拓扑；`nv-topograph` 从 NVIDIA Topograph 导入拓扑；`manual` 表示由管理员写入 label。 |
| `FabricNode` | 集群级资源，每个 Kubernetes Node 对应一个，由 Agent 写入。它记录 RDMA 网卡及其 LLDP 邻居、节点 IP 与角色、使用 RDMA 设备的 Pod，以及节点的健康 condition。参见 [FabricNode 参考](../reference/fabricnode.zh.md)。 |
| `Switch` | 集群级资源，对应一台物理交换机。用户创建它，并填写交换机的管理 IP 和网络角色。Controller 把 switch-agent 连接状态和 LLDP 邻居写入它的 status。参见 [Switch 参考](../reference/switch.zh.md)。 |
| `Topology` | 集群级只读资源，由 Controller 从 Node 和 `Switch` 的 label 聚合生成。它列出一张网络的性能域、各域的父域，以及每个节点的域路径。三个资源分别是 `scaleout`、`scaleup` 和 `storage`。参见 [Topology 参考](../reference/topology.zh.md)。 |
| `RDMAEndpoint` | namespace 级、只有 status 的资源，每个打开 RDMA 设备的 Pod 对应一个。启用 eBPF 流量归因后由 Agent 发布。它记录 Pod 的 GID、LID 和 queue pair，使其他节点上的 Agent 能找到流量的目的 Pod。参见 [RDMAEndpoint 参考](../reference/rdmaendpoint.zh.md)。 |
| workload 归因 | 把 RDMA 设备或 queue pair 映射到使用它的 Pod，再把 Pod 映射到顶层 owner（例如 Job 或 LeaderWorkerSet）。RDMA 指标和流量数据都带有这些 label。 |

概念之间的关系如下。每张网络有一个发现模式。在 `unifabric-roce` 模式下，Agent 通过
`FabricNode` 提供节点侧链路，`Switch` 资源提供交换机侧链路；Controller 计算性能域，
并写为 Node 拓扑 label。在 `nv-topograph` 模式下，由 NVIDIA Topograph 写入同样的 label。
两种模式下，Controller 都会把 label 聚合为该网络的 `Topology` 资源。

## 工作流程

### 拓扑发现

1. 采集。每个节点上的 Agent 把 RDMA 网卡及其 LLDP 邻居写入 `FabricNode` 资源。对于每个
   `Switch` 资源，Controller 订阅对应的 switch-agent，并把交换机侧 LLDP 邻居写入
   `Switch` status。
2. 计算。在 `unifabric-roce` 模式下，Controller 合并 `FabricNode` 中的节点到 leaf 链路与
   交换机之间的链路。交换机之间的链路来自 switch-agent；没有运行 switch-agent 时，来自
   `Switch` 资源上的邻居 annotation。Controller 把交换机和节点分组为分层的性能域。
   在 `nv-topograph` 模式下，由 NVIDIA Topograph 基于 NetQ 或 `ibnetdiscover` 数据完成这一步。
3. 发布。每个节点的域路径写入 Node label。Controller 把 Node 和 `Switch` 的 label
   聚合为 `Topology` 资源。
4. 消费。拓扑感知调度器读取 Node label。Grafana 拓扑插件通过 Topology HTTP API 读取
   `Topology` 资源。

发现方式与 label 模型参见[拓扑发现](./topology-discovery.zh.md)。

### RDMA 指标

1. Agent 读取节点上每块 RDMA 网卡的设备、端口和优先级计数器，覆盖吞吐、拥塞和错误。
2. Agent 把每个 RDMA 设备映射到使用它的 Pod，再把每个 Pod 映射到顶层 workload owner。
   因此指标带有 Pod、namespace 和 workload 信息。
3. Prometheus 抓取 Agent 的指标端点。Grafana 看板按节点和按 workload 展示指标。

参见 [RDMA 指标模型与 Pod 归因设计](../design/rdma-metrics.zh.md)。

### RDMA 流量归因

1. `agent.ebpfFlow.enabled` 为 true 时，Agent 在节点上的 `libibverbs` 库上挂载 eBPF uprobe。
   探针记录每个 queue pair 的目的端，并统计 CPU 提交的字节数和 work request 数。
2. Agent 根据进程找到每个 queue pair 的源 Pod；根据 Pod IP，或其他节点上的 Agent 发布的
   `RDMAEndpoint` 资源，找到目的 Pod。
3. Agent 暴露 Pod 到 Pod 的流量指标。启用逐次发送事件后，它还通过 OTLP 把每次发送作为
   一条事件发给流量事件收集器。

Unifabric 只观测由 CPU 提交的 RDMA。由 GPU kernel 直接提交到网卡队列的 RDMA（例如 GDAKI
或 IBGDA）绕过了探针。参见 [eBPF RDMA 流量归因设计](../design/rdma-flow.zh.md)。

### 网络健康状态

1. Agent 在每个 `FabricNode` 上设置 `LLDPNeighborsReady` 和 `Ready` condition。某块网卡
   没有 LLDP 邻居时，节点变为未就绪，Agent 同时记录一条 Kubernetes Event。
2. Controller 收到 switch-agent 的有效快照后，把 `Switch` 的 `status.healthy` 设为 true；
   订阅失败时设为 false。

网络路径主动探测已在规划中，参见[路线图](../roadmap.zh.md)。

## 使用场景

- 拓扑感知调度。Kueue、Volcano 或 KAI Scheduler 读取分层 Node label，把同一个训练或推理
  任务的 Pod 放在同一个 leaf 或 spine 域内。这样可以缩短通信路径，并减少上层链路上
  其他任务的干扰。
- 诊断变慢的任务。训练或推理任务变慢时，运维人员在 Grafana 中关联该任务的 Pod 和节点的
  RDMA 指标，找出拥塞的链路和占用带宽的 workload。
- 分析通信模式。借助 eBPF 流量归因，运维人员可以看到哪些 Pod 之间在通信、发送了多少数据
  以及消息大小，从而确认任务的通信模式符合预期。
- 定位故障及其影响范围。某条链路报错时，拓扑图和 `FabricNode` 资源会显示受影响的网卡、
  leaf 交换机、节点和任务。运维人员无需逐台登录交换机。
- 日常巡检。`FabricNode` 的 condition 和 `Switch` 的健康状态持续反映大规模集群中
  LLDP 链路和交换机连接的状况。
