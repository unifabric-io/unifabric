# RDMA 发送事件存储

> 成熟度: Alpha

English version: [rdma-flow-events.md](./rdma-flow-events.md)

## 简介

[基于 eBPF 的 RDMA tracing](./rdma-flow.zh.md)导出的是预先汇总的流指标,RDMA 发送事件存储在此基础上更进一步:Agent 把应用的每次 RDMA 发送写成一条事件并存入数据库,运维可以用这些事件分析 Pod 通信的时序、对端和消息大小。

本文说明如何选择存储后端,以及如何配置、验证和排查发送事件存储。

| 项目 | 说明 |
| --- | --- |
| 适用网络 | Scale-out、Storage |
| 数据来源 | 基于 eBPF 的 RDMA tracing 的探针,通过 BPF ring buffer 输出每次发送 |
| 观测对象 | CPU 发起的每次 RDMA 发送 |
| 输出 | ClickHouse 表或 Elasticsearch data stream 中的事件,`rdma_flow_sends_total` 和 `rdma_flow_send_bytes` 指标 |
| 集群侧要求 | 已开启基于 eBPF 的 RDMA tracing。使用内置 ClickHouse 时,集群有默认 StorageClass。 |
| 默认状态 | 默认关闭 |

数据按以下路径流动:

1. Agent 从 BPF ring buffer 读取每次发送,解析源 Pod 和目的 Pod。
2. Agent 通过 OTLP gRPC 把事件发给 Event Collector。Event Collector 是 Chart 部署的 OpenTelemetry Collector,对应的 Service 为 `unifabric-ebpf-flow-event-collector`。
3. Event Collector 把事件写入 ClickHouse、Elasticsearch,或者同时写入两者。

事件的字段参见[指标参考](../../reference/metrics.zh.md#rdma-发送事件字段)。

限制:

- 内置的 ClickHouse 和 Elasticsearch 只有一个副本,没有备份,并且使用固定的凭据,只适合测试和评估。
- Agent 只能把事件发给 Chart 部署的 Event Collector,不能配置外部的 Collector。
- Chart 目前只支持 ClickHouse 和 Elasticsearch,不支持 Kafka。
- 发送频繁的应用会产生大量事件,可以通过采样和合并降低事件量,参见 [Helm values 参考](../../../chart/README.md)中的 `agent.ebpfFlow.flowEvents.*`。

本功能是 AI 高速网络流量观测的一部分,完整的观测范围参见 [AI 高速网络流量观测](./README.zh.md#流量观测范围)。

## 前提条件

- 已按[基于 eBPF 的 RDMA tracing](./rdma-flow.zh.md#配置)开启 `agent.ebpfFlow.enabled=true`。
- 使用外置后端时,已准备好 ClickHouse 或 Elasticsearch,并且 Event Collector 能够连接到它。
- 使用内置 ClickHouse 时,集群有默认 StorageClass,或者已通过 `ebpfFlowEventCollector.clickhouse.bundled.persistence.storageClassName` 指定 StorageClass。

## 选择存储后端

两个后端可以单独使用,也可以同时使用。同时启用时,两者会收到相同的事件,其中一个不可用也不会影响另一个。

| 对比项 | ClickHouse | Elasticsearch |
| --- | --- | --- |
| 适合的场景 | 用 SQL 做聚合分析 | 与日志关联,在 Grafana 中查看发送记录 |
| 磁盘占用 | 较小 | 较大 |
| Grafana 仪表盘 | 仪表盘中的发送记录面板不查询 ClickHouse | Pod 和 AI Workload 仪表盘中的消息大小、进程与 QP、最近发送记录面板查询 Elasticsearch |
| 数据保留 | 默认 3 天 | 默认 3 天 |
| 内置实例的存储 | PVC,默认 `20Gi` | `emptyDir`,Pod 重建后数据丢失 |

不填写外置端点时,Chart 会部署内置实例;填写外置端点后,Chart 就不再部署内置实例。数据保留时间、内置实例的资源和存储等参数,参见 [Helm values 参考](../../../chart/README.md)中的 `ebpfFlowEventCollector.*`。

## 配置

### Case 1: 使用内置 ClickHouse

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set agent.ebpfFlow.enabled=true \
  --set agent.ebpfFlow.flowEvents.enabled=true \
  --set ebpfFlowEventCollector.clickhouse.enabled=true \
  --wait
```

Chart 会部署一个单副本的 ClickHouse `unifabric-ebpf-flow-clickhouse`:

- 用户为 `rdma`,密码为 `rdma-flow`。
- 事件写入 `rdma.rdma_sends` 表。
- 数据保存在 `20Gi` 的 PVC 中,使用集群默认 StorageClass。

Chart 还会安装"Unifabric / ClickHouse 服务器"仪表盘,用于查看内置 ClickHouse 的状态。

### Case 2: 使用内置 Elasticsearch

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set agent.ebpfFlow.enabled=true \
  --set agent.ebpfFlow.flowEvents.enabled=true \
  --set ebpfFlowEventCollector.elasticsearch.enabled=true \
  --wait
```

Chart 会部署一个单节点的 Elasticsearch `unifabric-elasticsearch`:

- 用户为 `elastic`,密码为 `rdma-flow`。
- 事件写入 `rdma-sends` data stream。
- 数据默认保存在 `emptyDir` 中,Pod 重建后会丢失。如果要使用 PVC,请设置 `ebpfFlowEventCollector.elasticsearch.bundled.persistence.enabled=true`。

Chart 还会安装"Unifabric / Elasticsearch 服务器"仪表盘,用于查看内置 Elasticsearch 的状态。

### Case 3: 使用外置 ClickHouse

1. 创建包含 `password` 键的 Secret:

   ```bash
   kubectl -n unifabric-system create secret generic clickhouse-credentials \
     --from-literal=password='<clickhouse-password>'
   ```

2. 把后端配置写入 `rdma-flow-events.yaml`,其中 `endpoint` 使用 ClickHouse 原生协议端点:

   ```yaml
   agent:
     ebpfFlow:
       enabled: true
       flowEvents:
         enabled: true
   ebpfFlowEventCollector:
     clickhouse:
       enabled: true
       endpoint: tcp://clickhouse.data.svc:9000
       database: rdma
       table: rdma_sends
       username: rdma
       existingSecret: clickhouse-credentials
   ```

3. 应用配置:

   ```bash
   helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
     --namespace unifabric-system \
     --reuse-values \
     -f rdma-flow-events.yaml \
     --wait
   ```

Event Collector 启动前,init 容器会在 ClickHouse 中创建数据库和表,因此用户 `rdma` 需要有建库和建表的权限。如果你自己管理表结构,请设置 `ebpfFlowEventCollector.clickhouse.schema.create=false`。

> 注意: 也可以直接设置 `ebpfFlowEventCollector.clickhouse.password`,但生产环境建议使用 Secret。

### Case 4: 使用外置 Elasticsearch

1. 创建包含 `password` 键的 Secret:

   ```bash
   kubectl -n unifabric-system create secret generic elasticsearch-credentials \
     --from-literal=password='<elasticsearch-password>'
   ```

2. 把后端配置写入 `rdma-flow-events.yaml`:

   ```yaml
   agent:
     ebpfFlow:
       enabled: true
       flowEvents:
         enabled: true
   ebpfFlowEventCollector:
     elasticsearch:
       enabled: true
       endpoints:
         - http://elasticsearch.data.svc:9200
       user: elastic
       index: rdma-sends
       existingSecret: elasticsearch-credentials
   ```

3. 应用配置:

   ```bash
   helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
     --namespace unifabric-system \
     --reuse-values \
     -f rdma-flow-events.yaml \
     --wait
   ```

Event Collector 启动前,init 容器会为 data stream 写入索引模板和 ILM 策略。如果你自己管理索引模板,请设置 `ebpfFlowEventCollector.elasticsearch.schema.create=false`。

> 注意: Pod 和 AI Workload 仪表盘中的发送记录面板通过 `GrafanaDatasource` 查询 Elasticsearch,而 Chart 只在 `grafanaDashboard.kind=GrafanaDashboard` (默认) 时创建这个数据源。如果使用 `ConfigMap` 安装仪表盘,需要在 Grafana 中手动创建 uid 为 `unifabric-elasticsearch` 的 Elasticsearch 数据源。

## 验证

1. 确认 Event Collector 和内置后端在运行:

   ```bash
   kubectl -n unifabric-system get deployment \
     unifabric-ebpf-flow-event-collector \
     unifabric-ebpf-flow-clickhouse \
     unifabric-elasticsearch
   ```

   只启用了一个内置后端时,另一个 Deployment 不存在。

2. 在应用产生 RDMA 通信后,在 Prometheus 或 Grafana Explore 中确认 Agent 已连接 Event Collector 并写入事件:

   ```promql
   rdma_flow_agent_send_sink_connected
   sum(rate(rdma_flow_agent_send_rows_inserted_total[5m]))
   ```

   `rdma_flow_agent_send_sink_connected` 应为 `1`。

3. 查看发送次数和消息大小:

   ```promql
   sum by (src_pod_name, dst_pod_name) (rate(rdma_flow_sends_total[1m]))
   histogram_quantile(0.5, sum by (le) (rate(rdma_flow_send_bytes_bucket[5m])))
   ```

4. 如果使用 ClickHouse,查询最近 5 分钟的事件:

   ```bash
   kubectl -n unifabric-system exec deploy/unifabric-ebpf-flow-clickhouse -- \
     clickhouse-client --user rdma --password rdma-flow --query "
       SELECT src_pod_name, dst_pod_name, sum(count) AS sends,
              sum(bytes * count) AS bytes,
              quantileWeighted(0.5)(bytes, count) AS p50_bytes
       FROM rdma.rdma_sends
       WHERE Timestamp > now() - INTERVAL 5 MINUTE
       GROUP BY src_pod_name, dst_pod_name
       ORDER BY sends DESC"
   ```

   <!-- TODO: 待真实环境采集输出 -->

   使用外置 ClickHouse 时,用你自己的客户端和凭据执行同样的 SQL。

5. 如果使用 Elasticsearch,确认 data stream 存在:

   ```bash
   kubectl -n unifabric-system exec deploy/unifabric-elasticsearch -c elasticsearch -- \
     curl -s -u elastic:rdma-flow 'http://localhost:9200/_data_stream/rdma-sends'
   ```

   <!-- TODO: 待真实环境采集输出 -->

   然后在 Grafana 中打开 Unifabric RDMA Pod 仪表盘,确认"最近发送记录"面板有数据。

## 故障排查

### 后端中没有事件

1. 确认 Agent 读到了发送事件。对 Agent Pod 执行端口转发后执行:

   ```bash
   curl -s http://127.0.0.1:8082/metrics | \
     grep -E '^rdma_flow_agent_send_(events_received|events_sampled|events_dropped|sink_connected|queue_length|rows_inserted_total|rows_failed_total)'
   ```

   - `rdma_flow_agent_send_events_received` 为 `0`: Agent 没有收到发送事件。按 [基于 eBPF 的 RDMA tracing 的故障排查](./rdma-flow.zh.md#故障排查)检查探针。
   - `rdma_flow_agent_send_sink_connected` 为 `0`: Agent 没有连上 Event Collector。检查下一步。
   - `rdma_flow_agent_send_rows_failed_total` 持续增长: Event Collector 拒绝了写入。检查下一步。

2. 查看 Event Collector 的日志:

   ```bash
   kubectl -n unifabric-system logs deploy/unifabric-ebpf-flow-event-collector --tail 200
   ```

   日志中的 exporter 错误说明 Event Collector 无法写入后端。检查后端地址、凭据和网络连通性。

3. 使用外置后端时,查看 init 容器创建表结构的日志。ClickHouse 的 init 容器为 `clickhouse-schema`,Elasticsearch 的 init 容器为 `elasticsearch-schema`:

   ```bash
   kubectl -n unifabric-system logs deploy/unifabric-ebpf-flow-event-collector -c clickhouse-schema
   kubectl -n unifabric-system logs deploy/unifabric-ebpf-flow-event-collector -c elasticsearch-schema
   ```

   如果用户没有建表权限,让数据库管理员预先创建表结构,并设置 `schema.create=false`。

### 事件被丢弃

如果 `rdma_flow_agent_send_events_dropped` 持续增长,说明事件产生的速度超过了写入速度,或者 Agent 与 Event Collector 之间的连接已断开。

- 通过采样或合并降低事件量,或者增大 Agent 的事件队列以吸收突发流量,参见 [Helm values 参考](../../../chart/README.md)中的 `agent.ebpfFlow.flowEvents.*`。
- 增大 Event Collector 的资源,并检查后端的写入性能,参见 [Helm values 参考](../../../chart/README.md)中的 `ebpfFlowEventCollector.resources`。

### 仪表盘中的发送记录面板没有数据

1. 确认已启用 Elasticsearch 后端,ClickHouse 中的事件不会显示在这些面板中。
2. 确认 `ebpfFlowEventCollector.elasticsearch.grafanaDatasource.enabled=true` (默认),并且 `grafanaDashboard.kind=GrafanaDashboard`。
3. 确认 `GrafanaDatasource` 存在:

   ```bash
   kubectl -n unifabric-system get grafanadatasource unifabric-ebpf-flow-event-collector-elasticsearch
   ```

## 参考

- [AI 高速网络流量观测](./README.zh.md): 完整的观测范围。
- [基于 eBPF 的 RDMA tracing](./rdma-flow.zh.md): 发送事件存储的前提。
- [指标参考](../../reference/metrics.zh.md#rdma-发送事件字段): 事件的字段。
- [eBPF RDMA 流归因设计](../../design/rdma-flow.zh.md#6-发送事件管线): 发送事件管线的设计。
- [Helm values 参考](../../../chart/README.md): 全部 `agent.ebpfFlow.flowEvents.*` 和 `ebpfFlowEventCollector.*` 参数。
