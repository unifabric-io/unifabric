# AI 高速网络流量观测

English version: [README.md](./README.md)

## 简介

Unifabric AI 高速网络流量观测采集 AI 应用在高速网络上的通信流量,并把这些流量关联到 Pod、namespace 和顶层 workload。观测范围既包括 NVLink 域内 GPU 之间的 NVLink 通信,也包括 Pod 经过 RDMA 网卡和交换机的跨节点通信。观测结果以 Prometheus 指标、`RDMAEndpoint` 资源和 Grafana 仪表盘的形式提供。

通过本文你将学习到:

- 流量观测覆盖通信路径上的哪些位置,以及各位置当前是否支持。
- 各位置的数据如何关联到同一个 Pod 身份。
- 各观测方式的详细文档在哪里查看。

## 背景

AI 训练任务和推理任务的本质是 GPU 之间的通信。一个任务的 Pod 通常分布在多个节点上:同一个 NVLink 域内的 GPU 通过 NVLink 通信,NVLink 域之外的 GPU 则使用 RDMA 通信,流量依次经过 Pod 的 RDMA 网卡、主机的 RDMA 网卡和交换机。Pod 读写模型、数据集和 KV Cache 的流量同样经过 RDMA 网卡和交换机。这条路径上任何一处发生拥塞或错误,都会拖慢整个任务。

然而,这条路径对云原生体系几乎是一个黑盒:

- 常规的网络观测手段失效。

  RDMA 绕过内核协议栈,由网卡直接收发数据。抓包、连接跟踪等基于内核协议栈的工具只能看到 TCP/IP 流量,看不到 RDMA 流量,网卡上只剩下设备级的累计计数器。

- 各处的数据互不相通,也没有业务身份。

  交换机的计数器在网络管理平台中,主机网卡的计数器在主机的 sysfs 中;Pod 使用的 SR-IOV VF 位于 Pod 自己的网络命名空间中,主机上的监控工具看不到;host 模式的 Pod 共用主机网卡,计数器无法区分流量属于哪个 Pod。这些数据只记录设备 (例如 `mlx5_0` 或交换机端口),不记录 Pod。Kubernetes 认识 Pod,却看不到这些硬件,这就是云原生体系和高速网络之间的"最后一公里"问题。

- 通信关系是黑盒。

  即使知道一个 Pod 发送了多少流量,也不知道这些流量发给了谁。任务变慢时,运维无法回答"是谁的流量占用了带宽,又影响了哪些任务";性能调优时,也无法确认通信的对端、时序和消息大小是否符合预期。

Unifabric 的 AI 高速网络流量观测功能尝试解决这些问题:它在通信路径的各个位置采集流量,并把每一处的数据都关联到同一个 Pod 身份上。运维可以从一个 workload 出发,查到它的 Pod 在和谁通信、流量经过了哪些网卡、这些网卡上有没有拥塞和错误,从而判断任务变慢是否由网络引起,以及是谁的流量影响了谁。

## 前提条件

- 已按[基础安装](../../getting-started.zh.md)完成 Unifabric 部署。
- 集群已安装 Prometheus Operator。
- 仅 Grafana 仪表盘需要: 集群已安装 Grafana Operator (v5 及以上) 或 Grafana sidecar。

各观测方式的其他要求,参见对应的文档。

## 流量观测范围

下图展示了 AI 应用的通信路径,以及流量观测在路径上的观测位置。同一个 NVLink 域内的 GPU 通过 NVLink 通信,一个 NVLink 域可以跨越多个节点。经过 RDMA 网卡的通信从 Pod 中的应用进程提交 RDMA 请求开始,依次经过 Pod 的 RDMA 网卡、主机的 RDMA 网卡和交换机,最终到达对端节点上的 Pod。

![流量观测范围](../../images/traffic-observability-scope.png)

| 观测位置 | 观测内容 | 支持情况 | 文档 |
| --- | --- | --- | --- |
| ① NVLink | NVLink 域内 GPU 之间的 NVLink 流量 | 规划中 | 无 |
| ② RDMA 提交路径 | Pod 之间的 RDMA 流: 谁和谁通信、流量多大、每次发送多大 | 已支持 (Alpha) | [基于 eBPF 的 RDMA tracing](./rdma-flow.zh.md)、[RDMA 发送事件存储](./rdma-flow-events.zh.md) |
| ③ 主机与 Pod 的 RDMA 网卡 | 网卡的吞吐、拥塞、错误和重传 | 已支持 (Stable) | [主机与 Pod 的 RDMA 网卡指标](./rdma-metrics.zh.md) |
| ④ 交换机 | 交换机端口上的 RDMA 流量 | 规划中 | 无 |

其中,Scale-out 网络和 Storage 网络的流量经过 ② 到 ④,Scale-up 网络的流量经过 ①。

### NVLink 流量观测

| 项目 | 说明 |
| --- | --- |
| 观测对象 | NVLink 域内 GPU 之间经过 NVLink 和 NVLink Switch 的流量 |
| 支持情况 | 规划中 |

NVLink 域的范围取决于服务器:

- 8 卡服务器 (例如 DGX B200) 中,NVLink 域就是一个节点,NVLink 流量只在节点内。
- 多节点 NVLink 系统 (例如 GB200 NVL72、GB300 NVL72) 中,一个机柜的 18 个计算节点通过 NVLink Switch 组成一个 72 GPU 的 NVLink 域,NVLink 流量会跨节点。

由于 NVLink 流量不经过 RDMA 网卡,网卡计数器和 eBPF 探针都看不到它。这一项计划采集 GPU 之间的 NVLink 流量,并关联到使用这些 GPU 的 Pod。

> 注意: 拓扑发现把 NVLink 域记录为 Scale-up 网络的 tier 1。参见[拓扑发现概念](../../concepts/topology-discovery.zh.md)。

### 主机与 Pod 的 RDMA 网卡观测

| 项目 | 说明 |
| --- | --- |
| 观测对象 | 主机的 RDMA 网卡和 Pod 使用的 RDMA 设备 |
| 数据来源 | sysfs 中的 RDMA 计数器,ethtool 优先级计数器 |
| 关联到 Pod 的方式 | Agent 识别使用 RDMA 的 Pod,读取 Pod 内 RDMA 设备的计数器 |
| 输出 | `unifabric_*` 指标,Grafana 中的 RDMA 集群、节点、Pod 和 AI Workload 仪表盘 |
| 支持情况 | 已支持 (Stable) |
| 文档 | [主机与 Pod 的 RDMA 网卡指标](./rdma-metrics.zh.md) |

这一项读取网卡的吞吐、拥塞 (ECN、CNP)、错误与重传计数器,以及各优先级的 pause 和丢弃计数器,并通过 `kind` 标签记录网卡所属的网络 (`scaleOut`、`storage` 或 `scaleUp`)。它能告诉你每张网卡和每个 Pod 收发了多少流量、有没有拥塞和错误,但不能告诉你流量发给了谁;通信对端由[基于 eBPF 的 RDMA tracing](#基于-ebpf-的-rdma-tracing) 提供。

### 基于 eBPF 的 RDMA tracing

| 项目 | 说明 |
| --- | --- |
| 观测对象 | CPU 通过 `libibverbs` 提交的每次 RDMA 发送 |
| 数据来源 | `libibverbs` 提交路径上的 eBPF uprobe |
| 关联到 Pod 的方式 | 源 Pod: 发送进程所在的 cgroup。目的 Pod: QP 的目的地址,RoCE 网络使用 GID,InfiniBand 网络使用 LID。 |
| 输出 | `rdma_flow_*` 指标和 `RDMAEndpoint` 资源。可选: 把每次发送存入 ClickHouse 或 Elasticsearch。 |
| 支持情况 | 已支持 (Alpha) |
| 文档 | [基于 eBPF 的 RDMA tracing](./rdma-flow.zh.md)、[RDMA 发送事件存储](./rdma-flow-events.zh.md) |

RDMA 通信绕过内核,网卡上只有总量计数。这一项在应用提交 RDMA 请求的位置挂载 eBPF 探针,无需修改应用就能还原 Pod 之间的 RDMA 流,包括谁和谁通信、何时通信、每次发送多大,让 RDMA 通信像 TCP 连接一样可观测。

限制:

- 只覆盖 CPU 发起的 RDMA。GPU 发起的 RDMA (例如 GDAKI、IBGDA) 和内核态的 RDMA 存储协议 (例如 NVMe over RDMA) 不经过 `libibverbs`,探针看不到它们。网卡计数器仍记录这部分流量。
- InfiniBand 网络需要设置 `agent.ebpfFlow.endpoint.allPods=true`,才能解析目的 Pod。
- Storage 网络上用户态存储客户端的 RDMA 流可以观测,但该场景尚未验证。

### 交换机 RDMA 流量观测

| 项目 | 说明 |
| --- | --- |
| 观测对象 | Scale-out 网络和 Storage 网络中,交换机端口上的 RDMA 流量 |
| 支持情况 | 规划中,参见 [#49](https://github.com/unifabric-io/unifabric/issues/49) 和 [#3](https://github.com/unifabric-io/unifabric/pull/3) |

这一项计划在交换机上采集流量样本,并把采样到的 RDMA 流量关联到 Pod。它不只回答"交换机上的这段流量属于哪个任务":结合主机与 Pod 的 RDMA 网卡观测和基于 eBPF 的 RDMA tracing,它能把一个 Pod 在主机网卡和沿途每台交换机上的 RDMA 指标串联起来,呈现这段流量从源 Pod 出发、经过哪些交换机和端口、到达目的 Pod 的端到端链路,从而定位拥塞或错误发生在路径的哪一段。

> 注意: 目前可以用拓扑发现确定网卡连接的交换机。拓扑发现会在 Node label 中记录节点所在的交换机域;使用 LLDP 发现时,`FabricNode` 还会记录每张网卡连接的交换机和端口。参见[拓扑发现概念](../../concepts/topology-discovery.zh.md)。

## 关联到 Pod 身份

上面各观测方式分别采集通信路径上一个位置的数据,而 Unifabric 在采集时就给每条数据加上了 Pod 身份,包括 Pod、namespace,以及 Pod 所属的训练或推理任务。因此,不同位置的数据都可以用同一个 Pod 或同一个任务来查询:在 Grafana 中选择一个 Pod 或一个任务,就能同时看到它的网卡流量,以及它与其他 Pod 之间的通信。

各观测方式如何关联到 Pod,以及在 Grafana 中能看到什么,参见:

- [主机与 Pod 的 RDMA 网卡指标](./rdma-metrics.zh.md#关联到-pod-和训练任务)
- [基于 eBPF 的 RDMA tracing](./rdma-flow.zh.md#查看结果)

## 参考

- [主机与 Pod 的 RDMA 网卡指标](./rdma-metrics.zh.md)、[基于 eBPF 的 RDMA tracing](./rdma-flow.zh.md)、[RDMA 发送事件存储](./rdma-flow-events.zh.md): 各观测方式的原理、配置、验证和故障排查。
- [架构](../../concepts/architecture.zh.md): Agent、Controller 和各组件之间的关系。
- [拓扑发现概念](../../concepts/topology-discovery.zh.md): 节点和网卡连接的交换机。
- [RDMA 指标模型与 Pod 归因设计](../../design/rdma-metrics.zh.md)、[eBPF RDMA 流归因设计](../../design/rdma-flow.zh.md)。
- [指标参考](../../reference/metrics.zh.md)、[RDMAEndpoint 参考](../../reference/rdmaendpoint.zh.md)、[FabricNode 参考](../../reference/fabricnode.zh.md)。
- [Helm values 参考](../../../chart/README.md): 全部 `nodeMetrics.*`、`agent.ebpfFlow.*`、`ebpfFlowEventCollector.*` 和 `grafanaDashboard.*` 参数。
- [路线图](../../roadmap.zh.md): 规划中的功能。
