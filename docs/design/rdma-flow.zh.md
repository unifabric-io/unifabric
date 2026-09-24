# eBPF RDMA 流归属设计

English: [rdma-flow.md](./rdma-flow.md)

## 1. 目的

宿主机 RDMA 计数器只能说明某块设备或某个 Pod 发了多少，说不清发给了谁。
Agent 的 `ebpfFlow` 组件在不改动应用的前提下把 RDMA 发送流量归属到
Pod 到 Pod 的流，运维人员可以看到哪个训练或推理任务在和哪个对端通信、
消息有多大，并可选地查看每一次提交。

组件默认关闭，通过 `agent.ebpfFlow.enabled` 按 release 打开。
代码位于 `pkg/agent/ebpfflow`，作为 controller-runtime 的 `Runnable`
运行在现有的 Agent DaemonSet 内。

## 2. 数据面

组件只观察 `libibverbs`，绕过它直接与内核或设备交互的库不可见。

- **控制面。** 在节点上找到的每一份 `libibverbs` 的 `ibv_modify_qp`
  上挂 uprobe，QP 带地址向量进入 RTR 时触发。BPF 程序把目标 GID 或 LID、
  目标 QPN、端口、设备名和进程名写入以 `(tgid, qpn)` 为键的 `qp_infos`
  map，并发出 `QP_RTR` 事件。
- **回调发现。** `ibv_create_qp`、`ibv_reg_mr`、`ibv_query_qp` 等公开
  探针从存活的 verbs 对象读取 provider 函数指针，把 `post_send`、
  `wr_start`、`wr_complete` 等地址经 `callback_events` ring buffer 上报。
  用户态把它们换算为 provider 库内的文件偏移并挂只按地址的 uprobe。
- **数据面。** provider uprobe 在 `qp_stats` map 中按 `(tgid, qpn)` 累加
  payload 字节与 WR 数，同时覆盖传统 `post_send` 路径与扩展 verbs 的
  setter。`events.enabled` 为 true 时还会把每次提交的 32 字节记录推入
  `send_events` ring buffer；为 false 时 BPF 配置 map 从源头关闭这些记录。
- **持久化。** `qp_infos` 与 `qp_stats` 固定在 `/sys/fs/bpf/unifabric`，
  Agent 重启后保留目标与累计值，布局变化时重建。学到的回调偏移按库内容
  标识缓存在 `/var/lib/unifabric/ebpf-flow`，重启后无需等待新 verbs 对象
  即可重新挂载。

进程与库通过 `sched_process_exec` tracepoint 与周期扫描 `/proc/*/maps`
发现，因此需要 `hostPID`。带版本的 `libibverbs` 符号通过 ELF 动态符号表
解析，否则 `cilium/ebpf` 会选中兼容别名。

## 3. 归属

每个报告周期（默认 5s）组件把 `qp_stats` 与 `qp_infos` 连接起来，
解析每个 QP 的两端。

- **源端。** `tgid` 经 cgroup 路径映射到 Pod UID，再经 Pod 索引到 Pod。
- **目的端，按顺序。**
  1. RoCE GID 若为 IPv4 映射或 IPv6 地址，查 Pod IP 索引，索引含 Multus
     `network-status` 与 Calico `podIPs` 等 CNI 注解。
  2. 链路本地 GID 携带 EUI-64 编码的 MAC，与同类注解中的 MAC 匹配。
  3. 在 `RDMAEndpoint` 索引中查 GID。节点上多个 Pod 共享同一 GID 时由
     目标 QPN 选出上报了它的那个，否则唯一持有者胜出，仍有歧义的 GID
     保持未解析。
  4. 没有 GRH 的 InfiniBand QP 携带 LID。本地端口的子网前缀取自 GID
     index 0，组成键 `<subnetPrefix>/<lid>` 在 `RDMAEndpoint` 索引中查找，
     同样以 QPN 细化。
- **Owner。** 沿 `ownerReferences` 用已知 workload 类型表向上查找顶层
  owner，按 UID 缓存，未知类型停在该级。

hostNetwork Pod 共享节点 IP，被排除在 IP 索引之外，只能经 `RDMAEndpoint`
解析。两端未都解析到 Pod 的流被跳过且不推进基线，因此索引追上前的流量
在解析成功后一次补记，也不会出现空标签的序列。

Pod 索引是一个独立的集群级 controller-runtime cache，只缓存 Pod 并裁剪到
身份、ownerReferences、注解、hostNetwork 与 Pod IP，因为 Agent 默认的
cache 只包含本节点的 Pod。把对端解析完全迁移到 `RDMAEndpoint` 是计划中的
后续工作。

## 4. RDMAEndpoint

每个 Agent 为本节点上每个持有 RDMA 设备或在 `qp_infos` 中仍有 QP 的 Pod
发布一个命名空间级 `RDMAEndpoint`。对象与 Pod 同名，以 `controller: true`
挂到 Pod 上，只携带观察到的 status，见 [API 参考](../reference/rdmaendpoint.md)。

- 设备持有者通过 `/proc/*/fd` 中的 `/dev/infiniband/uverbsN` 描述符发现。
  设备、端口、链路层、LID、子网前缀与 GID 在宿主机网络命名空间的 sysfs
  中读取，独占 VF 的 Pod 则在其自身命名空间内读取。
- 发布由事件驱动。`QP_RTR` 事件或清理已退出进程会触发一次运行，
  两次运行至少间隔 `endpoint.minInterval`（默认 3s），`endpoint.interval`
  （默认 30s）是覆盖事件丢失的兜底周期。
- 写入使用 Server-Side Apply，field manager 为 `unifabric-agent`，
  metadata 与 `status` 子资源各一次。内容不变的对象最多每 5 分钟重写一次
  以刷新 `lastUpdateTime`，读取端把超过 15 分钟未刷新的对象视为过期。
- 启动时 Agent 接管带 `unifabric.io/node=<node>` 标签、由上一实例留下的
  对象，并删除 Pod 已不存在的对象。Pod 不再持有设备且没有 QP 时对象被
  删除，日志为 `rdma_endpoint_deleted reason=no_rdma_holder`。
- 默认只发布 hostNetwork Pod 与尚未进入索引的 Pod，其余 Pod 经 IP 解析。
  `endpoint.allPods` 发布所有持有者，InfiniBand 因 LID 无法反查而必须打开。

## 5. 指标

流量指标带十个标签：`src_pod_namespace`、`src_pod_name`、
`src_pod_top_owner_kind`、`src_pod_top_owner_namespace`、
`src_pod_top_owner_name` 及对应的五个 `dst_pod_*`，再加 `src_device`
与 `dst_device`。

| 指标 | 类型 | 含义 |
| --- | --- | --- |
| `rdma_flow_bytes_total` | counter | 源端发往目的端的 payload 字节。 |
| `rdma_flow_wrs_total` | counter | 提交的 WR 数。 |
| `rdma_flow_qps` | gauge | 上一轮报告中归属到该流的 QP 数。 |
| `rdma_flow_sends_total` | counter | 发送提交次数，每次 `post_send` 或扩展 verbs setter 计一次。 |
| `rdma_flow_send_bytes` | histogram | 每次提交的 payload 字节，桶为 64 B 到 1 GiB 的 2 的幂。 |
| `rdma_flow_agent_qp_infos`、`_qp_stats`、`_qp_matched`、`_qp_unmatched`、`_qp_unresolved`、`_flows` | gauge | 上一轮的 map 大小与连接结果。 |
| `rdma_flow_agent_send_events_received`、`_sampled` | gauge | 从 ring buffer 读到与选入存储的发送事件数。 |
| `rdma_flow_agent_send_events_dropped{sink}`、`_send_queue_length{sink}`、`_send_rows_inserted_total{sink}`、`_send_rows_failed_total{sink}`、`_send_insert_seconds{sink}`、`_send_sink_connected{sink}` | 按 sink | 存储路径健康度。 |

Counter 只在一个 Agent 进程内单调。map 中的计数回退意味着条目被重建，
按新条目处理。发送计数与直方图接受未解析的端并带空标签，因为大小分布
只有覆盖全部提交才有意义。

## 6. 发送事件管线

`agent.ebpfFlow.flowEvents.enabled` 控制逐次发送事件采集，默认 true 以保持兼容：

- **累计模式（false）。** 探针仍更新 `qp_infos` 与 `qp_stats`，因此继续提供
  `rdma_flow_bytes_total`、`rdma_flow_wrs_total`、`rdma_flow_qps` 和
  `rdma_flow_agent_qp_*` 健康指标。BPF 程序不写 `send_events`，Agent 不打开
  该 ring buffer、不创建 sink 队列、不连接 OTLP，也不产生发送大小指标。
- **事件模式（true）。** 除累计指标外，每次提交还写入 ring buffer。Agent
  提供 `rdma_flow_sends_total` 与 `rdma_flow_send_bytes`，并可经 OTLP 导出
  带归属的事件行。

Helm 的 `ebpfFlowEventCollector` 依赖逐次事件。`flowEvents.enabled` 为 false 时打开
Collector 会在模板渲染阶段直接报错。

发送事件用上一轮报告发布的快照做归属，经 `sendSink` 接口分发到各 sink。
每个 sink 有独立的有界队列、写协程、惰性连接与计数。

- 队列容量 `flowEvents.queue`（默认 200000）行，每 `flowEvents.batch`（默认 10000）
  行或 `flowEvents.flush`（默认 1s）后的不完整批发送一次。队列满与发送失败都
  直接丢行不重试，ring buffer 消费者永不阻塞。
- `flowEvents.sample` 每 N 条事件只存一条，指标仍统计全部。
- `flowEvents.aggregate` 把窗口内同一 QP `(kind, bytes, wrs)` 相同的提交合并为
  一行并带 `count`。大模型流量的消息大小高度重复，100ms 窗口通常能把行数
  降低一到两个量级。
- 连接惰性建立并每 30s 重试，断连期间到达的行丢弃并计数。

唯一的 sink 是 `otlp`。每行成为一条经 OTLP gRPC 发送的 OpenTelemetry
log record，`Timestamp` 是由 `CLOCK_MONOTONIC` 换算的 BPF 事件时间，
`EventName` 与 `Body` 为 `rdma.send`，resource 带
`service.name=unifabric-agent` 与 `k8s.node.name`，属性扁平且有类型：
`tgid`、`qpn`、`bytes`、`wrs`、`count` 为 int64，`node`、`kind`、
`src_device`、`dst_device` 与十个 Pod 标签为字符串。后端由 Collector
负责，见[使用指南](../usage/rdma-flow.md)。

## 7. 限制

- 绕过 `libibverbs` 的库产生的流量不可见。
- UD QP 不做归属。
- 发送事件逐条上报，极高提交速率下 16 MiB ring buffer 可能溢出，
  丢失明细但不影响 `qp_stats` 累计值。
- Agent 运行期间流的时间序列不会被移除。
- 集群级 Pod cache 在每个节点上占用与集群 Pod 数成正比的内存。
