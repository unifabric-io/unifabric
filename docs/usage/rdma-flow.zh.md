# eBPF RDMA 流归属

English: [rdma-flow.md](./rdma-flow.md)

本文说明如何打开 Agent 的 `ebpfFlow` 组件、验证 Pod 到 Pod 的 RDMA 流已被
归属、可选地经 OpenTelemetry Collector 存储每一次发送事件，以及归属缺失时
如何排查。设计见 [docs/design/rdma-flow.zh.md](../design/rdma-flow.zh.md)，
它发布的资源见 [docs/reference/rdmaendpoint.zh.md](../reference/rdmaendpoint.zh.md)。

## 前提

- Linux 节点支持 BPF ring buffer 与 uprobe，内核 5.8 及以上。
- 负载使用 `libibverbs`，绕过它的库产生的流量不可见。
- Agent DaemonSet 已经以 privileged 与 `hostPID` 运行，打开组件会额外挂载
  宿主机的 `/sys/fs/bpf` 与 `/var/lib/unifabric`。

## 打开组件

```yaml
agent:
  ebpfFlow:
    enabled: true
```

`helm upgrade` 即可，未打开开关的 release 没有任何变化。InfiniBand 环境还需
设置 `agent.ebpfFlow.endpoint.allPods: true`，因为对端按 LID 寻址，
每个 RDMA Pod 都必须发布 `RDMAEndpoint`。

全部配置项与默认值见 [Helm values 参考](../../chart/README.md) 的
`agent.ebpfFlow` 部分，它们与 Agent 配置文件的 `ebpfFlow` 段一一对应。

只需要累计流量统计、不需要逐次发送 ring buffer 事件和存储管线时：

```yaml
agent:
  ebpfFlow:
    enabled: true
    flowEvents:
      enabled: false
```

该模式保留流的字节、WR 与活跃 QP 指标，不产生发送次数与发送大小指标、
OTLP 行，也没有 send ring buffer 和 sink 队列开销。需要精确发送事件时保持
`flowEvents.enabled: true`（默认值）。

## 验证

确认每个 Agent 都启动了组件：

```bash
kubectl -n <ns> logs ds/<release>-agent -c agent | grep -E 'startup_complete|bpf_collection_loaded|bootstrap_probes_attached'
```

在两个 Pod 之间跑 RDMA 流量，例如在两台节点的特权 hostNetwork Pod 里运行
`perftest` 的 `ib_write_bw`，然后：

```bash
kubectl get rep -A
kubectl get rep -n <ns> <pod> -o yaml
```

每个持有 RDMA 设备的发送或接收 Pod 都有对象，承载已建连接的端口会列出
`queuePairs`。

经 Agent 的 metrics Service 或 Prometheus 查询：

```promql
sum by (src_pod_name, dst_pod_name) (rate(rdma_flow_bytes_total[1m]))
histogram_quantile(0.5, sum by (le) (rate(rdma_flow_send_bytes_bucket[5m])))
rdma_flow_agent_qp_unresolved
```

第一条是每条流的每秒字节，第二条是消息大小中位数，第三条是两端未都解析到
Pod 的 QP 数，索引追上后应回到零。

`grafanaDashboard.enabled` 为 true 时会随组件渲染 `Unifabric / RDMA Flow`
看板，语言由 `grafanaDashboard.language` 决定，选择 workload 与 Pod 后显示发往各对端的流量、消息大小分桶、Agent
健康以及 Agent 与存储后端的资源占用。

## 存储发送事件

流量指标是预聚合的。要保留每次提交及其 Pod 归属，部署可选的 Collector 与
一到两个后端。使用你自己的 ClickHouse 或 Elasticsearch：

```yaml
agent:
  ebpfFlow:
    enabled: true
    flowEvents:
      enabled: true
      aggregate: 100ms
ebpfFlowEventCollector:
  enabled: true
  clickhouse:
    enabled: true
    endpoint: tcp://clickhouse.data.svc:9000
    existingSecret: clickhouse-credentials
  elasticsearch:
    enabled: true
    endpoints:
      - http://elasticsearch.data.svc:9200
    existingSecret: elasticsearch-credentials
```

没有现成后端只想评估时，把 `endpoint` 与 `endpoints` 留空，chart 会在
Collector 旁部署内置的单副本实例：

```yaml
ebpfFlowEventCollector:
  enabled: true
  clickhouse:
    enabled: true
  elasticsearch:
    enabled: true
```

内置实例仅用于测试。它们只有一个副本，没有复制与备份，未设置 `password`
时使用固定密码 `rdma-flow`，内置 Elasticsearch 默认把数据放在 `emptyDir`。
内置 ClickHouse 使用默认 StorageClass 的 20Gi PersistentVolumeClaim，
`bundled.persistence` 可以指定 StorageClass、大小、已有 claim 或改用
`emptyDir`。一旦设置了 `endpoint` 或 `endpoints`，下次 upgrade 会移除内置
实例，生产流量请指向托管集群。

- `agent.ebpfFlow.otlp.endpoint` 为空时 Agent 自动把 OTLP log
  record 发到 Collector 的 Service。
- `flowEvents.aggregate` 把窗口内同一 QP 的相同提交合并为一行并带 `count`。
  大模型流量的消息大小高度重复，100ms 窗口通常能把行数降低一到两个量级。
  只需抽样明细时用 `flowEvents.sample` 每 N 条留一条。
- 使用 ClickHouse 时 Collector 用 init 容器创建数据库和为每个属性建强类型列
  的表，TTL 为 `clickhouse.ttlDays`，exporter 不会自行建表。加列在
  `chart/templates/EbpfFlowEventCollector.yaml` 的 schema 里追加
  `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`。
- 使用 Elasticsearch 时文档以 `ecs` 映射模式写入 `rdma-sends` 数据流。
  `elasticsearch.schema.create` 在 Collector 启动前写入索引模板与 ILM 策略：
  字符串属性全部为 keyword、`date_nanos` 时间戳、关闭动态映射、单分片无副本，
  按 `schema.ilm.rolloverMaxAge` 或 `rolloverMaxPrimaryShardSize` 滚动并在
  `deleteMinAge` 后删除。内置实例每次启动都重放模板，因为它属于集群状态。
  新增属性需在 `chart/templates/EbpfFlowEventCollector.yaml` 的模板里加字段后
  `_rollover`。`grafanaDashboard.kind` 为 `GrafanaDashboard` 时会为数据流创建
  `GrafanaDatasource`，`Unifabric / RDMA Flow Sends` 看板据此查询精确的
  消息大小分位、最常见大小、按进程的 QP 数与原始记录。
- 已有 Secret 需要含 `password` 键。不指定 `existingSecret` 时 chart 把
  `password` 写入自己的 Secret。

两个后端收到相同的记录，一个不可用不影响另一个。对这类数据 Elasticsearch
的磁盘占用通常是 ClickHouse 的 5 到 10 倍，适合与日志关联而不是聚合分析。

ClickHouse 查询示例：

```sql
SELECT src_pod_name, dst_pod_name, sum(count), sum(bytes * count),
       quantileWeighted(0.5)(bytes, count)
FROM rdma.rdma_sends
WHERE Timestamp > now() - INTERVAL 5 MINUTE
GROUP BY 1, 2 ORDER BY 3 DESC
```

## 其他 workload 控制器的 owner 解析

流量标签携带每个 Pod 的顶层 owner。Agent 只沿它有权限读取的类型向上追溯
`ownerReferences`，因此 Argo Workflows 这类控制器需要先加一条读权限，其 Pod
才会标到 workflow 而不是中间资源。第三方类型的规则定义在 values 的
`agent.workloadOwnerRules`，这是一个以 API group 为键、复数资源名为值的 map，
用户 values 会与默认值合并，新增一个控制器只需多加一个键：

```yaml
agent:
  workloadOwnerRules:
    argoproj.io: [workflows]
```

chart 把这个 map 渲染成 `<release>-agent-workload-owners` ClusterRole。流量解析实际会追溯的类型列表在
`pkg/agent/ebpfflow/owners.go`，新增 group 时也要在那里加一项。

## 排障

Agent 日志是结构化的，按 `msg` 字段过滤。

| 现象 | 检查 |
| --- | --- |
| 完全没有 `rdma_flow_*` 序列 | Agent 日志中的 `bpf_collection_loaded` 与 `bootstrap_probes_attached`。`load bpf collection` 错误说明内核缺 BPF 特性或镜像缺 BPF 对象，`attach_bootstrap_probes` 错误会指出无法挂探针的库。 |
| 有流量但 `rdma_flow_agent_qp_stats` 为零 | 数据面探针还没挂上。负载创建 QP 后应出现 `provider_callback_restored` 或 `callback_received`，否则该 provider 不受支持或负载绕过了 `libibverbs`。 |
| `rdma_flow_agent_qp_unresolved` 居高不下 | `qp_stats` debug 记录里有 `source` 与 `destination`。目的为节点 IP 或 LID 而为空时，接收节点需要有 `RDMAEndpoint`，查 `kubectl get rep -A` 与发送端 Agent 的 `rdma_endpoint_refresh_complete`。目的为 Pod IP 时需要 Pod IP 索引，查 `pod_index_refresh_complete` 与 `get_pod_list` 错误。 |
| 缺少某个 `RDMAEndpoint` | Pod 必须持有打开的 `/dev/infiniband/uverbsN`。自带网络命名空间的 Pod 只有在 `endpoint.allPods` 打开时才发布。`rdma_endpoint_deleted reason=no_rdma_holder` 表示该 Pod 最后一个 RDMA 进程已退出。 |
| `RDMAEndpoint` 没有 `queuePairs` | QP 还没进入 RTR，例如正在等待客户端的服务端。 |
| `rdma_flow_agent_send_events_dropped{sink="otlp"}` 增长 | Collector 不可达或慢于事件速率。查 `rdma_flow_agent_send_sink_connected`、Collector 的 `otelcol_exporter_send_failed_log_records`，并调大 `sends.aggregate` 或 `sends.sample`。 |
| Agent 重启后计数归零 | 固定的 map 与新版本布局不兼容而被重建，日志为 `reuse_pinned_maps status=layout_incompatible`，Prometheus 的 `rate()` 能处理归零。 |

在节点上查看固定的 map 与偏移缓存：

```bash
ls /sys/fs/bpf/unifabric
cat /var/lib/unifabric/ebpf-flow/callback-offsets.json
```

停止 Agent 后删除两者可强制重新发现。
