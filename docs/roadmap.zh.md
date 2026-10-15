# 路线图

English version: [roadmap.md](./roadmap.md)

- 最后更新：2026-10-15
- 基线版本：v0.3.0（编写本路线图时的最新发布版本）

本路线图描述 Unifabric 的规划方向。内容会随社区反馈持续调整,条目、优先级
与目标版本均可能变化。欢迎通过
[GitHub issue](https://github.com/unifabric-io/unifabric/issues) 讨论或提出路线图条目。

## 规划条目

| 一级功能 | 二级功能 | 状态 | 目标版本 |
| --- | --- | --- | --- |
| 拓扑识别 | RDMA 网卡发现与分类（scale-out / storage） | Completed | v0.0.1 |
| | LLDP 邻居发现（节点与交换机 Agent） | Completed | v0.0.2 |
| | 交换机驱动的 scale-out 拓扑计算 | Completed | v0.0.2 |
| | 拓扑状态聚合与 Node 标签自动写入 | Completed | v0.1.0 |
| | 基于拓扑标签的调度集成（Kueue / Volcano / KAI TAS） | Completed | v0.1.0 |
| | NVIDIA Topograph / NetQ 导入（InfiniBand、Spectrum-X） | Completed | v0.3.0 |
| | 拓扑可视化（Topology API + Grafana） | Completed | v0.3.0 |
| | 原生 scale-up（NVLink）拓扑发现 | Planned | v0.5 |
| | 原生存储网络拓扑发现 | Planned | v0.5 |
| RDMA 观测 | RDMA 设备、端口、拥塞与错误指标 | Completed | v0.0.1 |
| | Pod、Namespace、Workload 归因 | Completed | v0.0.1 |
| | Prometheus 集成与 Grafana 仪表盘 | Completed | v0.1.0 |
| | eBPF Pod-to-Pod 流归因与 RDMAEndpoint | Completed | v0.3.0 |
| | 逐条 send 事件与 ClickHouse / Elasticsearch 存储 | Completed | v0.3.0 |
| | GPU-initiated RDMA（GDAKI / IBGDA）观测 | Planned | Backlog |
| RDMA 网络健康检查 | 被动健康状态（LLDP 就绪、交换机连接、探针） | Completed | v0.1.0 |
| | 主动网络路径探测与链路退化检测 | Planned | v0.4 |
| | 故障自动修复 | Planned | Backlog |

状态取值：`Planned`（规划中）、`In progress`（进行中）、`Completed`（已完成）、`Deferred`（暂缓）。
