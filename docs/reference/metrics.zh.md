# 指标参考

English version: [metrics.md](./metrics.md)

## 概述

本文列出 Unifabric Agent 导出的 AI 高速网络流量观测指标,以及 RDMA 发送事件的字段。
Agent 通过 `unifabric-agent-metrics` Service 导出全部指标。
如何使用这些指标,参见 [AI 高速网络流量观测](../usage/traffic-observability/README.zh.md)。

## 主机与 Pod 的 RDMA 网卡指标

全部指标的类型都是 Gauge。
计数器类指标保存抓取时刻的累计值,用 `rate()` 或 `increase()` 计算速率。

| 指标 | 类型 | 标签 | 含义 |
| --- | --- | --- | --- |
| `unifabric_node_info` | Gauge | `unifabric_node_name` | 值固定为 1。表示 Agent 在这个节点上运行。 |
| `unifabric_<counter>` | Gauge | [网卡样本标签](#网卡样本标签) | `/sys/class/infiniband/<device>/ports/<port>/counters/` 中的一个计数器,例如 `unifabric_port_xmit_data`、`unifabric_port_rcv_data`、`unifabric_unicast_xmit_packets`。`source` 为 `counters`。`port_xmit_data` 和 `port_rcv_data` 已换算为字节。 |
| `unifabric_<hw_counter>` | Gauge | [网卡样本标签](#网卡样本标签) | `/sys/class/infiniband/<device>/ports/<port>/hw_counters/` 中的一个计数器,例如 `unifabric_np_ecn_marked_roce_packets`、`unifabric_np_cnp_sent`、`unifabric_out_of_sequence`、`unifabric_local_ack_timeout_err`。`source` 为 `hw_counters`。计数器由网卡驱动提供,不同网卡不同。 |
| `unifabric_port_speed_mbps` | Gauge | [网卡样本标签](#网卡样本标签) | 网卡速率,单位 Mbit/s。`source` 为 `interface`。 |
| `unifabric_port_mtu` | Gauge | [网卡样本标签](#网卡样本标签) | 网卡 MTU。`source` 为 `interface`。 |
| `unifabric_port_oper_state` | Gauge | [网卡样本标签](#网卡样本标签) | 网卡状态为 `up` 时为 1,否则为 0。`source` 为 `interface`。 |
| `unifabric_rdma_device_tos` | Gauge | [网卡样本标签](#网卡样本标签) | RDMA 设备的 traffic class。`source` 为 `device`。host 模式的 Pod 没有这个指标。 |
| `unifabric_rx_pause`、`unifabric_tx_pause` | Gauge | [网卡样本标签](#网卡样本标签) | 一个优先级收到和发出的 pause 帧数。`priority` 为优先级。`source` 为 `ethtool`。主机上只采集 PF。 |
| `unifabric_rx_discards`、`unifabric_tx_discards` | Gauge | [网卡样本标签](#网卡样本标签) | 一个优先级接收和发送方向丢弃的报文数。`priority` 为优先级。`source` 为 `ethtool`。主机上只采集 PF。 |

### 网卡样本标签

除 `unifabric_node_info` 外,所有网卡指标都带有以下标签。不适用的标签值为空。

| 标签 | 含义 |
| --- | --- |
| `node_name` | 节点名称。 |
| `device` | RDMA 设备,例如 `mlx5_0`。 |
| `ifname` | RDMA 设备对应的网卡。 |
| `parent_ifname` | VF 对应的 PF 网卡。对于 PF,与 `ifname` 相同。 |
| `port` | RDMA 端口号。 |
| `priority` | ethtool 优先级计数器的优先级。 |
| `pod_namespace`、`pod_name` | 样本所属的 Pod。`scope` 为 `host` 时为空。 |
| `topowner_api_version`、`topowner_kind`、`topowner_namespace`、`topowner_name` | Pod 所属的顶层 workload,例如 `PyTorchJob`。 |
| `host_rdma` | Pod 是否以 host 模式使用 RDMA。 |
| `is_root` | `ifname` 与 `parent_ifname` 是否相同。 |
| `kind` | 网卡所属的网络: `scaleOut`、`storage`、`scaleUp`,或者为空。 |
| `scope` | `host` 表示主机网卡样本,`pod` 表示 Pod 样本。 |
| `source` | 数据来源: `counters`、`hw_counters`、`interface`、`device` 或 `ethtool`。 |

## RDMA tracing 指标

开启 `agent.ebpfFlow.enabled` 后,Agent 导出以下指标。

| 指标 | 类型 | 标签 | 含义 |
| --- | --- | --- | --- |
| `rdma_flow_bytes_total` | Counter | [流标签](#流标签) | 一条流累计发送的字节数。 |
| `rdma_flow_wrs_total` | Counter | [流标签](#流标签) | 一条流累计发送的 WR 数。 |
| `rdma_flow_qps` | Gauge | [流标签](#流标签) | 一条流当前活跃的 QP 数。 |
| `rdma_flow_sends_total` | Counter | [流标签](#流标签) | 一条流累计的发送次数。只在开启 `agent.ebpfFlow.flowEvents.enabled` 时导出。 |
| `rdma_flow_send_bytes` | Histogram | [流标签](#流标签) | 每次发送的字节数分布。只在开启 `agent.ebpfFlow.flowEvents.enabled` 时导出。 |
| `rdma_flow_agent_qp_infos` | Gauge | 无 | `qp_infos` BPF map 中的条目数。 |
| `rdma_flow_agent_qp_stats` | Gauge | 无 | `qp_stats` BPF map 中的条目数。 |
| `rdma_flow_agent_qp_matched` | Gauge | 无 | 在 `qp_infos` 中找到目的地址的 `qp_stats` 条目数。 |
| `rdma_flow_agent_qp_unmatched` | Gauge | 无 | 在 `qp_infos` 中没有找到目的地址的 `qp_stats` 条目数。 |
| `rdma_flow_agent_qp_unresolved` | Gauge | 无 | 源端或目的端无法解析为 Pod 而被跳过的 `qp_stats` 条目数。 |
| `rdma_flow_agent_flows` | Gauge | 无 | 最近一次汇总中不同的源 Pod 与目的 Pod 组合数。 |
| `rdma_flow_agent_send_events_received` | Gauge | 无 | Agent 启动后从 ring buffer 读到的发送事件数。 |
| `rdma_flow_agent_send_events_sampled` | Gauge | 无 | Agent 启动后经采样选出、准备存储的发送事件数。 |
| `rdma_flow_agent_send_events_dropped` | Gauge | `sink` | 因队列满或连接断开而丢弃的事件数。 |
| `rdma_flow_agent_send_queue_length` | Gauge | `sink` | 等待写入的事件数。 |
| `rdma_flow_agent_send_rows_inserted_total` | Counter | `sink` | 已写入的事件数。 |
| `rdma_flow_agent_send_rows_failed_total` | Counter | `sink` | 写入失败的事件数。 |
| `rdma_flow_agent_send_insert_seconds` | Histogram | `sink` | 每批写入的耗时。 |
| `rdma_flow_agent_send_sink_connected` | Gauge | `sink` | Agent 与 Event Collector 连接正常时为 1。 |

`sink` 标签的取值为 `otlp`。

### 流标签

| 标签 | 含义 |
| --- | --- |
| `src_pod_namespace`、`src_pod_name` | 提交 WR 的 Pod。 |
| `src_pod_top_owner_kind`、`src_pod_top_owner_namespace`、`src_pod_top_owner_name` | 源 Pod 所属的顶层 workload。 |
| `dst_pod_namespace`、`dst_pod_name` | 对端 Pod。 |
| `dst_pod_top_owner_kind`、`dst_pod_top_owner_namespace`、`dst_pod_top_owner_name` | 对端 Pod 所属的顶层 workload。 |
| `src_device` | 源 RDMA 设备,例如 `mlx5_0`。 |
| `dst_device` | 目的 RDMA 设备。未知时为空。 |

## RDMA 发送事件字段

开启 `agent.ebpfFlow.flowEvents.enabled` 后,每次发送成为一条事件,写入 ClickHouse 表或 Elasticsearch data stream。

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| `Timestamp` | 时间,纳秒精度 | 发送时间。 |
| `node` | 字符串 | 节点名称。 |
| `tgid` | 整数 | 发送进程的 ID。 |
| `qpn` | 整数 | QP 编号。 |
| `bytes` | 整数 | 一次提交的字节数。 |
| `wrs` | 整数 | 一次提交的 WR 数。 |
| `count` | 整数 | 这条记录合并的提交次数。没有合并时为 1。 |
| `kind` | 字符串 | 提交方式: `post_send`、`wr_set_sge`、`wr_set_sge_list`、`wr_set_inline_data` 或 `wr_set_inline_data_list`。无法识别时为 `unknown_type_<N>`。 |
| `src_device`、`dst_device` | 字符串 | 源 RDMA 设备和目的 RDMA 设备。 |
| `src_pod_namespace`、`src_pod_name`、`src_pod_top_owner_kind`、`src_pod_top_owner_namespace`、`src_pod_top_owner_name` | 字符串 | 源 Pod 和它所属的顶层 workload。 |
| `dst_pod_namespace`、`dst_pod_name`、`dst_pod_top_owner_kind`、`dst_pod_top_owner_namespace`、`dst_pod_top_owner_name` | 字符串 | 目的 Pod 和它所属的顶层 workload。 |
