# eBPF RDMA 流量归属设计

English: [rdma-flow.md](./rdma-flow.md)

## 1. 目的

宿主机 RDMA 计数器只能反映设备或 Pod 的发送量，无法说明流量发往哪个对端。Agent 的 `ebpfFlow` 组件无需修改应用，即可将 RDMA 发送流量归属到 Pod 间的流，帮助运维人员了解训练或推理任务的通信对端、消息大小，并按需查看每次发送提交。

该组件仅支持观测 **CPU 发起的 RDMA（CPU-initiated RDMA）**：CPU 必须通过 `libibverbs` provider 提交 `post_send` 或扩展 verbs 工作请求（WR）。只要 WR 仍由 CPU 提交，数据缓冲区位于 GPU 显存中就不影响观测。

**GPU 发起的 RDMA（GPU-initiated RDMA）** 通信由 GPU 内核直接向网卡队列提交 WQE，例如 GDAKI、GPUDirect Async 和 IBGDA。这些路径不经过组件挂载的 uprobe，因此无法观测。

组件默认关闭，通过 `agent.ebpfFlow.enabled` 按 Helm release 启用。代码位于 `pkg/agent/ebpfflow`，以 controller-runtime 的 `Runnable` 形式运行在现有的 Agent DaemonSet 内。

### 1.1 通信库兼容性

兼容性取决于最终的网络提交路径，不能仅凭通信库名称或版本判断。推理框架、通信库的支持情况及信息截至版本统一见[使用指南](../usage/rdma-flow.zh.md)中的「推理框架视角」和「通信库支持矩阵」。升级通信库后，应结合实际后端配置和日志重新确认提交路径。

## 2. 数据面

组件仅观测 `libibverbs` 路径；绕过 `libibverbs` 直接与内核或设备交互的库无法观测。

- **控制面。** 在节点上发现的每份 `libibverbs` 的 `ibv_modify_qp` 上挂载 uprobe，QP 携带地址向量进入 RTR 状态时触发。BPF 程序将目标 GID 或 LID、目标 QPN、端口、设备名和进程名写入以 `(tgid, qpn)` 为键的 `qp_infos` map，并发出 `QP_RTR` 事件。
- **回调发现。** `ibv_create_qp`、`ibv_reg_mr`、`ibv_query_qp` 等公开函数上的探针从存活的 verbs 对象中读取 provider 函数指针，将 `post_send`、`wr_start`、`wr_complete` 等地址通过 `callback_events` ring buffer 上报。用户态将这些地址换算为 provider 库内的文件偏移，再按地址挂载 uprobe。
- **数据面。** provider uprobe 在 `qp_stats` map 中按 `(tgid, qpn)` 累加有效载荷（payload）字节数与 WR 数，覆盖传统 `post_send` 路径及扩展 verbs 的 setter。`flowEvents.enabled` 为 `true` 时，还会将每次提交的 32 字节记录写入 `send_events` ring buffer；为 `false` 时，通过 BPF 配置 map 禁止生成这些记录。
- **持久化。** `qp_infos` 与 `qp_stats` 固定在 `/sys/fs/bpf/unifabric` 下，Agent 重启后保留目标信息与累计值，map 布局变化时重建。已发现的回调偏移按库内容标识缓存在 `/var/lib/unifabric/ebpf-flow` 中，重启后无需等待新的 verbs 对象即可重新挂载探针。

组件通过 `sched_process_exec` tracepoint 和周期性扫描 `/proc/*/maps` 发现进程与库，因此需要 `hostPID`。带版本的 `libibverbs` 符号通过 ELF 动态符号表解析，否则 `cilium/ebpf` 会选中兼容别名。

## 3. 归属

每个报告周期（默认 5 秒），组件关联 `qp_stats` 与 `qp_infos`，解析每个 QP 的源端和目的端。

- **源端。** 通过 cgroup 路径将 `tgid` 映射到 Pod UID，再通过 Pod 索引查找 Pod。
- **目的端。** 按以下顺序解析：
  1. RoCE GID 为 IPv4 映射地址或 IPv6 地址时，查询 Pod IP 索引。索引包含 Multus `network-status`、Calico `podIPs` 等 CNI 注解中的地址。
  2. 将链路本地 GID 中 EUI-64 编码的 MAC 与同类注解中的 MAC 匹配。
  3. 在 `RDMAEndpoint` 索引中查询 GID。节点上多个 Pod 共享同一 GID 时，根据目标 QPN 选择上报该 QPN 的 Pod；否则，使用 GID 的唯一持有者。仍有歧义的 GID 保持未解析。
  4. 没有 GRH 的 InfiniBand QP 携带 LID。取本地端口 GID 索引 0 的子网前缀，与 LID 组成键 `<subnetPrefix>/<lid>`，在 `RDMAEndpoint` 索引中查找，同样通过 QPN 进一步区分。
- **Owner。** 沿 `ownerReferences`，根据已知工作负载类型表向上查找顶层 owner，并按 UID 缓存。遇到未知类型时停止向上查找。

hostNetwork Pod 共享节点 IP，不纳入 IP 索引，只能通过 `RDMAEndpoint` 解析。对于源端或目的端尚未解析到 Pod 的流，组件跳过本轮统计且不推进基线。索引更新并解析成功后，此前的流量会一次性补记，因此不会产生带空标签的流量时间序列。

Pod 索引使用独立的集群级 controller-runtime 缓存，仅缓存 Pod，并只保留身份、ownerReferences、注解、hostNetwork 和 Pod IP 信息。这是因为 Agent 默认的缓存只包含本节点的 Pod。将对端解析完全迁移到 `RDMAEndpoint` 是计划中的后续工作。

## 4. RDMAEndpoint

每个 Agent 为本节点上持有 RDMA 设备或在 `qp_infos` 中仍有 QP 的每个 Pod 发布一个命名空间级 `RDMAEndpoint`。对象与 Pod 同名，通过 `controller: true` 的所有者引用关联到 Pod，仅携带观测到的状态，见 [API 参考](../reference/rdmaendpoint.md)。

- **设备发现。** 通过 `/proc/*/fd` 中的 `/dev/infiniband/uverbsN` 描述符发现设备持有者。在宿主机网络命名空间的 sysfs 中读取设备、端口、链路层、LID、子网前缀和 GID；对于独占 VF 的 Pod，则在其自身网络命名空间内读取。
- **发布时机。** `QP_RTR` 事件或已退出进程的清理操作会触发发布。两次运行至少间隔 `endpoint.minInterval`（默认 3 秒）；`endpoint.interval`（默认 30 秒）用于周期性发布，弥补事件丢失。
- **写入与刷新。** 使用 Server-Side Apply 写入，field manager 为 `unifabric-agent`，分别更新 metadata 和 `status` 子资源。内容未变化的对象最多每 5 分钟重写一次，以刷新 `lastUpdateTime`。读取端将超过 15 分钟未刷新的对象视为过期。
- **接管与清理。** 启动时，Agent 接管上一实例留下的带 `unifabric.io/node=<node>` 标签的对象，并删除对应 Pod 已不存在的对象。Pod 不再持有设备且没有 QP 时，删除对应对象，并记录日志 `rdma_endpoint_deleted reason=no_rdma_holder`。
- **发布范围。** 默认只发布 hostNetwork Pod 与尚未进入索引的 Pod，其余 Pod 通过 IP 解析。启用 `endpoint.allPods` 后发布所有持有者。InfiniBand 无法通过 LID 反查 Pod，因此必须启用此配置。

## 5. 指标

流量指标包含 12 个标签：5 个源 Pod 标签（`src_pod_namespace`、`src_pod_name`、`src_pod_top_owner_kind`、`src_pod_top_owner_namespace`、`src_pod_top_owner_name`）、对应的 5 个 `dst_pod_*` 标签，以及 `src_device` 和 `dst_device`。

| 指标 | 类型 | 含义 |
| --- | --- | --- |
| `rdma_flow_bytes_total` | counter | 源端发往目的端的有效载荷字节数。 |
| `rdma_flow_wrs_total` | counter | 提交的 WR 数。 |
| `rdma_flow_qps` | gauge | 上一轮报告中归属到该流的 QP 数。 |
| `rdma_flow_sends_total` | counter | 发送提交次数，每次 `post_send` 或扩展 verbs setter 计一次。 |
| `rdma_flow_send_bytes` | histogram | 每次提交的有效载荷字节数，桶边界为 64 B 到 1 GiB 范围内的 2 的幂。 |
| `rdma_flow_agent_qp_infos`、`_qp_stats`、`_qp_matched`、`_qp_unmatched`、`_qp_unresolved`、`_flows` | gauge | 上一轮的 map 大小与关联结果。 |
| `rdma_flow_agent_send_events_received`、`_sampled` | gauge | 从 ring buffer 读取的发送事件数，以及被选中写入存储的发送事件数。 |
| `rdma_flow_agent_send_events_dropped{sink}`、`_send_queue_length{sink}`、`_send_rows_inserted_total{sink}`、`_send_rows_failed_total{sink}`、`_send_insert_seconds{sink}`、`_send_sink_connected{sink}` | 按 sink | 存储路径健康度。 |

Counter 仅在单个 Agent 进程内单调递增。map 中的计数回退表示条目已重建，组件将其按新条目处理。

与流量归属指标不同，发送计数与直方图接受源端或目的端未解析的事件，并保留对应的空标签，以覆盖全部提交的消息大小分布。

## 6. 发送事件管线

`agent.ebpfFlow.flowEvents.enabled` 控制逐次发送事件采集，默认为 `false`。只需累计流量指标时，关闭事件采集可避免 ring buffer 和导出管线的开销。

- **累计模式（`false`）。** 探针仍更新 `qp_infos` 与 `qp_stats`，继续提供 `rdma_flow_bytes_total`、`rdma_flow_wrs_total`、`rdma_flow_qps` 和 `rdma_flow_agent_qp_*` 健康指标。BPF 程序不写入 `send_events`，Agent 不打开该 ring buffer、不创建 sink 队列、不连接 OTLP，也不产生发送大小指标。
- **事件模式（`true`）。** 除累计指标外，每次提交还会写入 ring buffer。Agent 提供 `rdma_flow_sends_total` 与 `rdma_flow_send_bytes`，并可通过 OTLP 导出带归属信息的事件记录。

Helm 的 `ebpfFlowEventCollector` 依赖逐次发送事件。`flowEvents.enabled` 为 `false` 时启用 Event Collector，会在模板渲染阶段报错。

发送事件使用上一轮报告发布的快照解析归属，再通过 `sendSink` 接口分发到各 sink。每个 sink 具有独立的有界队列、写入协程、惰性连接和计数。

- **队列与批量发送。** `flowEvents.queue` 控制队列容量，默认为 200000 行。每积累 `flowEvents.batch` 行（默认 10000 行）发送一批；不足一批时，在 `flowEvents.flush` 间隔（默认 1 秒）后发送。队列满或发送失败时直接丢弃记录，不重试，确保 ring buffer 消费者不阻塞。
- **采样。** `flowEvents.sample` 每 N 条事件仅存储一条，指标仍统计全部事件。
- **聚合。** `flowEvents.aggregate` 将窗口内同一 QP 中 `(kind, bytes, wrs)` 相同的提交合并为一条记录，并附带 `count`。大模型流量的消息大小高度重复，100 ms 窗口通常能将记录数降低一到两个量级。
- **连接与重试。** 连接按需建立，每 30 秒重试一次。断连期间到达的记录会被丢弃并计数。

当前唯一的 sink 是 `otlp`。每条事件记录转换为一条 OpenTelemetry 日志记录，通过 OTLP gRPC 发送：

- `Timestamp` 为通过 `CLOCK_MONOTONIC` 换算的 BPF 事件时间。
- `EventName` 与 `Body` 均为 `rdma.send`。
- resource 包含 `service.name=unifabric-agent` 与 `k8s.node.name`。
- 属性采用扁平结构，并保留类型：`tgid`、`qpn`、`bytes`、`wrs`、`count` 为 int64；`node`、`kind`、`src_device`、`dst_device` 和 10 个 Pod 标签为字符串。

存储后端由 Event Collector 负责，配置方式见[使用指南](../usage/rdma-flow.zh.md)。

## 7. 限制

- 绕过 `libibverbs` 的库产生的流量不可见。
- 不解析 UD QP 的归属。
- 发送事件逐条上报。提交速率极高时，16 MiB ring buffer 可能溢出，导致明细丢失，但不影响 `qp_stats` 累计值。
- Agent 运行期间流的时间序列不会被移除。
- 集群级 Pod 缓存在每个节点上占用的内存与集群 Pod 数量成正比。
