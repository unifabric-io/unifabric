# Unifabric

[English](./README.md) | 简体中文

[![License](https://img.shields.io/badge/license-Apache%202.0-blue)](./LICENSE)
[![Release](https://img.shields.io/github/v/release/unifabric-io/unifabric)](https://github.com/unifabric-io/unifabric/releases)

> 面向云原生 AI 集群高速网络的可观测系统

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

## 核心特性

### 拓扑识别

自动发现 scale-out、scale-up 和存储三张网络的拓扑，发布为 `Topology` 资源和分层
Node label，并在 Grafana 中交互式可视化。Kueue TAS、Volcano、KAI Scheduler 等
调度器消费这些 label，把通信密集的任务收敛在同一交换机域内；运维在拓扑图上快速
圈定故障域。支持通用 SONiC RoCE（LLDP 自主发现）、NVIDIA Spectrum-X 和
InfiniBand（从 NVIDIA Topograph 导入）。

### RDMA 与 NVLink 流量观测

观测 scale-out 和存储网络的 RDMA 流量，以及 scale-up 网络的 NVLink 流量，采集面
横跨主机网卡、容器网卡、交换机和 eBPF 流归因。吞吐、拥塞、错误指标归因到 Pod、
namespace 和 workload，回答"是谁的流量、影响了谁"，并提供预置 Grafana 仪表盘。

### 网络主动健康检查

不等业务报障，主动对高速网络路径做探测巡检，让链路劣化和配置故障在影响训练与推理任务之前暴露出来。

## 架构

![Unifabric 产品架构图](./docs/images/product-architecture.svg)

Unifabric 构建在 scale-out、scale-up 和存储三张网络之上：直接发现这些网络，或从
NVIDIA Topograph、NetQ 导入拓扑，以统一的 Kubernetes 资源模型对外提供三大核心能力，
由调度器、Grafana 和 Prometheus 消费。组件级架构详见
[架构总览](./docs/concepts/architecture.zh.md)。

## 快速开始

1. 完成[基础安装](./docs/getting-started.zh.md)。
2. 按照集群物理网络对应的文档操作：
   - [通用 SONiC RoCE](./docs/getting-started-sonic-roce.zh.md)
   - [NVIDIA Spectrum-X](./docs/getting-started-spectrum-x.zh.md)
   - [InfiniBand](./docs/getting-started-infiniband.zh.md)

要了解每种网络使用哪种发现方式，参见[拓扑发现概念](./docs/concepts/topology-discovery.zh.md)。

## 文档导航

| 分类 | 链接 |
| --- | --- |
| 文档索引 | [docs/README.zh.md](./docs/README.zh.md) |
| 架构与概念 | [架构总览](./docs/concepts/architecture.zh.md)、[拓扑发现](./docs/concepts/topology-discovery.zh.md) |
| 快速开始 | [基础安装](./docs/getting-started.zh.md)、[通用 SONiC RoCE](./docs/getting-started-sonic-roce.zh.md)、[NVIDIA Spectrum-X](./docs/getting-started-spectrum-x.zh.md)、[InfiniBand](./docs/getting-started-infiniband.zh.md) |
| 功能使用 | [拓扑可视化](./docs/usage/topology-visualization.zh.md)、[RDMA 指标](./docs/usage/rdma-metrics.zh.md)、[RDMA 流归因](./docs/usage/rdma-flow.zh.md)、[拓扑感知调度](./docs/usage/workload-tas.zh.md)、[Switch Agent 部署](./docs/usage/switch-agent-deployment.zh.md) |
| API 参考 | [FabricNode、Switch、Topology、RDMAEndpoint](./docs/reference/README.zh.md) |
| Helm values | [Chart 参考](./chart/README.md) |
| 设计文档 | [docs/design/](./docs/design/) |

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
