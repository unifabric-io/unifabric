# Unifabric

[English](./README.md) | 简体中文

[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](./LICENSE)
[![Release](https://img.shields.io/github/v/release/unifabric-io/unifabric)](https://github.com/unifabric-io/unifabric/releases)

> 面向 AI 高速网络的云原生可观测系统

## 简介

现代 AI 集群依赖三张高速网络——GPU 跨节点通信的 Scale-out 网络（RoCE / InfiniBand）、
节点内 GPU 互联的 Scale-up 网络（NVLink 等），以及高速存储网络。训练与推理的性能
高度依赖这三张网络：一条链路的拥塞或劣化，足以拖慢整个任务，甚至中断训练。

但这三张网络对云原生体系几乎是黑盒。它们由多家厂商的交换机、路由器和网卡组成，
硬件各自的观测工具互不相通，也不与 Kubernetes 的资源模型和监控体系对接；而社区
现有的云原生可观测方案围绕 TCP/IP 构建，不覆盖 RDMA 与 NVLink。网络异常无法归因
到具体的 Pod 和训练任务——这是 AI workload 可观测的"最后一公里"问题。

Unifabric 用云原生的方式补上这块空白：自动识别三张网络的拓扑，观测 RDMA 和 NVLink
流量并归因到 workload，主动探测网络健康。无论底层是哪家厂商的设备，都以统一的
Kubernetes 资源模型（CRD、Node label、metrics）呈现，并且观测不止于看板——拓扑
label 直接被 Kueue、Volcano 等调度器消费，提升 AI 集群的训练与推理效率。

## 架构

![Unifabric 产品架构图](./docs/images/product-architecture.png)

Unifabric 构建在 scale-out、scale-up 和存储三张网络之上：直接发现这些网络，或从
NVIDIA Topograph、NetQ 导入拓扑，以统一的 Kubernetes 资源模型对外提供三大核心能力，
由调度器、Grafana 和 Prometheus 消费。

## 核心特性

### 拓扑识别

拓扑识别功能实时探测并识别 scale-out、scale-up 和存储三张网络中主机、交换机等
设备之间的连接关系，把原本散落在各家网络设备里的物理拓扑，变成 Kubernetes 中
可直接消费的标准资源。

具备如下功能：

- **分层 Node label 发布**：自动把节点在网络中的位置（leaf、spine 等层级）写入
  Node label，供 Kueue TAS、Volcano、KAI Scheduler 等调度器直接消费，把通信
  密集的任务收敛在同一交换机域内，缩短通信路径，提升训练与推理效率
- **拓扑可视化**：发布 `Topology` 资源并在 Grafana 中交互式呈现全网拓扑，
  让运维无需登录交换机即可掌握全局连接状态，排障时可快速确认受影响的节点与链路范围

功能适用：

- 支持通用 SONiC RoCE 交换机（LLDP 自主发现）、NVIDIA Spectrum-X 和
  InfiniBand（从 NVIDIA Topograph 导入）
- 支持裸金属与云化环境

### AI 高速网络流量观测

AI 高速网络流量观测功能通过读取网卡计数器、采集交换机指标、挂载 eBPF 探针等方式，
采集三张高速网络的流量数据，并统一归因到 Pod、namespace、workload 和主机，把设备视角的流量变成
业务视角的可观测指标。观测的目的，是让网络问题可以用业务语言回答——任务变慢是否
由网络引起、是谁的流量影响了谁——为 AI 训练与推理的性能分析和故障定位提供依据。

具备如下功能：

- **主机与 Pod 网卡的 RDMA 指标统计**：覆盖吞吐、拥塞与错误指标，
  为性能分析和故障定位提供端侧数据
- **交换机流量的 Pod 身份关联**：把 scale-out 与存储网络中交换机上的 RDMA 流量
  关联到具体 Pod，打通"设备流量"与"业务身份"之间的断层
- **基于 eBPF 的 RDMA 流观测**：RDMA 通信绕过内核协议栈，
  抓包和连接跟踪等常规手段全部失效，网卡上只剩总量计数，通信关系完全是黑盒。
  针对 CPU 发起的 RDMA 通信，该功能在 verbs 库的提交路径上挂载 eBPF 探针，
  无需修改应用即可还原出 Pod 级的 RDMA 流：谁在和谁通信、何时通信、每次发送多大，
  让 RDMA 通信像 TCP 连接一样可观测
- **NVLink 流量指标（规划中）**：采集 scale-up 网络中 GPU 间 NVLink 通信指标，
  让节点内 GPU 互联这一传统监控盲区变得可观测

应用场景：

- **训练/推理任务变慢时**：定位通信热点——在 AI workload 级别关联交换机、
  Pod 网卡等路径上的 RDMA 指标，找出占用带宽的热点应用
- **性能分析与调优时**：分析 Pod RDMA 通信的时序、peer 与报文大小，
  确认通信模式是否符合预期
- **网络拥塞或异常排障时**：通过预置 Grafana 仪表盘快速回答"是谁的流量、
  影响了谁"，确定影响范围

### 网络健康检查

网络健康检查功能持续对三张高速网络的链路和配置做巡检，主动发现问题，
而不是等业务报障后再排查。

具备如下功能：

- **被动健康状态采集**：持续收集 LLDP 就绪、交换机连接状态与探针结果，
  实时反映网络基础设施的健康水位
- **主动网络路径探测与链路退化检测（规划中）**：主动打流探测网络路径，
  发现尚未触发告警的链路劣化

应用场景：

- **大规模集群日常巡检时**：持续掌握链路和交换机的健康水位，
  无需逐台设备人工检查
- **训练任务开跑前**：提前验证网络路径健康，避免长时间训练任务
  因链路劣化或配置故障中断或降速

## 快速开始

按照[部署指南](./docs/getting-started.zh.md)安装 Unifabric。该文档覆盖默认的
SONiC RoCE 网络，也支持 InfiniBand 和 NVIDIA Spectrum-X 两种环境。

安装完成后，使用各项功能参考:

- 拓扑识别与可视化：[拓扑可视化](./docs/usage/topology-visualization.zh.md)
- 拓扑感知调度：[Kueue / Volcano / KAI 调度集成](./docs/usage/workload-tas.zh.md)
- AI 高速网络流量观测：[AI 高速网络流量观测](./docs/usage/traffic-observability/README.zh.md)
- 交换机流量观测：[Switch Agent 部署](./docs/usage/switch-agent-deployment.zh.md)

## 路线图

规划中的功能与未来方向见[路线图](./docs/roadmap.zh.md)。

## 社区与贡献

欢迎参与贡献：

- 通过 [GitHub issue](https://github.com/unifabric-io/unifabric/issues) 报告 bug 或提出功能建议。
- 按照 [NVAIR 开发环境指南](./docs/development/dev-with-nvair.md)搭建开发环境。
- 提交 pull request 前，运行 `make build`、`make test-unit` 和 `make check-license`。
- 提交 commit 时使用 `git commit -s` 签署 [Developer Certificate of Origin](https://developercertificate.org/)。

## 来源

DaoCloud 于 2026-04-19 创建本项目。溯源信息见公开仓库
[unifabric-io/unifabric](https://github.com/unifabric-io/unifabric) 的历史记录。

## 许可证

Apache License 2.0，详见 [LICENSE](./LICENSE)。
