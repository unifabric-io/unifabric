# sFlow 处理使用指南

本文说明如何启用 Unifabric sFlow collector，配置交换机 sFlow exporter，准备 ClickHouse，并验证 switch / workload dashboard。

实现设计见 [sFlow 处理设计](../design/sflow.zh.md)。

## 前置条件

- 一个可访问的 ClickHouse 实例，并开放 native protocol 端口。
- 已使用 `docs/sql/sflow-flows-raw.sql` 创建 `flows_raw` 表。
- 交换机能够把 sFlow v5 datagram 发送到 Kubernetes Service 地址。
- 如果 `grafanaDashboard.enabled=true`，集群需要具备 Grafana dashboard 导入能力。

## 准备 ClickHouse

创建表：

```bash
clickhouse-client --multiquery < docs/sql/sflow-flows-raw.sql
```

如果目标 database 不是 `default`，需要在执行前修改 SQL 中的 database 名称，并将 `sflow.clickhouse.database` 和 `sflow.clickhouse.table` 设置为对应目标。

## 启用 Collector

创建 values 文件：

```yaml
sflow:
  enabled: true
  service:
    type: NodePort
    port: 6343
    # 可选。不设置或设置为 0 时，由 Kubernetes 自动分配 nodePort。
    nodePort: 0
  clickhouse:
    address: clickhouse-clickhouse.clickhouse.svc.cluster.local:9000
    database: default
    username: default
    passwordSecret:
      name: clickhouse-auth
      key: password
    table: flows_raw
```

默认使用 `NodePort`，因为交换机 sFlow 流量通常来自集群外部。交换机需要固定 UDP node port 时，可以设置 `sflow.service.nodePort`；保持为 `0` 时，由 Kubernetes 自动分配。环境支持 UDP 外部负载均衡时，也可以把 `sflow.service.type` 改成 `LoadBalancer`。

渲染并安装：

```bash
helm template unifabric ./chart -f sflow-values.yaml
helm upgrade --install unifabric ./chart -f sflow-values.yaml
```

## 配置交换机

将每台交换机的 sFlow exporter 指向可访问的 Node 地址和 Service nodePort。如果 `sflow.service.type=LoadBalancer`，则指向 LoadBalancer 地址和 UDP `sflow.service.port`。容器内监听端口默认仍是 `6343`。

sampling rate 需要结合交换机能力和预期流量设置。数值越小，collector 收到的记录越多，ClickHouse 写入压力也越高。如果 collector 出现 queue drop，可以提高 sampling rate，增大 `sflow.writer.queueSize`，或扩展存储写入能力。

## 验证 Collector

检查渲染出的资源：

```bash
kubectl -n unifabric-system get deploy,svc,cm -l app.kubernetes.io/component=unifabric-sflow
kubectl -n unifabric-system get clusterrole,clusterrolebinding -l app.kubernetes.io/component=unifabric-sflow
```

检查 Pod 状态和日志：

```bash
kubectl -n unifabric-system get pods -l app.kubernetes.io/component=unifabric-sflow
kubectl -n unifabric-system logs deploy/unifabric-sflow -c sflow
```

从集群内检查 metrics endpoint：

```bash
POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-sflow -o jsonpath='{.items[0].status.podIP}')
kubectl -n unifabric-system run sflow-metrics-check --rm -i --restart=Never --image=curlimages/curl -- \
  "http://${POD_IP}:8084/metrics"
```

常用指标：

- `unifabric_sflow_datagrams_accepted_total`：collector 接收到的 datagram 数量。
- `unifabric_sflow_decode_errors_total`：格式错误或不支持的 datagram 数量。
- `unifabric_sflow_records_decoded_total`：生成的规范化记录数量。
- `unifabric_sflow_records_written_total`：写入 ClickHouse 的记录数量。
- `unifabric_sflow_records_dropped_total`：writer queue 满后丢弃的记录数量。
- `unifabric_sflow_write_errors_total`：ClickHouse 写入失败次数。
- `unifabric_sflow_queue_depth`：当前排队等待写入的记录数量。

## 验证写入记录

查询最近记录：

```sql
SELECT
  time_flow_start,
  IPv4NumToString(reinterpretAsUInt32(reverse(substring(sampler_address, 13, 4)))) AS switch,
  src_k8s_pod_namespace,
  src_k8s_pod_name,
  src_k8s_top_owner_kind,
  src_k8s_top_owner_name,
  dst_k8s_pod_namespace,
  dst_k8s_pod_name,
  dst_k8s_top_owner_kind,
  dst_k8s_top_owner_name,
  bytes,
  packets
FROM flows_raw
WHERE time_flow_start > now() - INTERVAL 10 MINUTE
ORDER BY time_flow_start DESC
LIMIT 20;
```

预期结果：涉及当前集群 Pod 的记录，会在命中的源端或目的端包含 Pod、Node 和 workload owner 字段。访问外部端点的流量，在未命中的一侧保留空 Kubernetes 字段。

## 验证 Dashboard

当 `grafanaDashboard.enabled=true` 且 `sflow.enabled=true` 时，chart 会渲染：

- `switch-sflow.json`
- `workload-sflow.json`

switch dashboard 从交换机 sampler address 进入，展示 sampled traffic、workloads、Pods、samples、protocols 和 top talkers。workload dashboard 从 workload owner 进入，按 switch 和 Pod 展示发送与接收流量。

如果 dashboard 没有数据，先确认所选时间范围内 ClickHouse 有行，再检查 dashboard data source 和变量。

## 排障

如果 collector 没有收到 datagram：

- 确认 Service type 和地址能被交换机访问。
- 确认交换机 exporter 指向 UDP `6343`，或配置的 `sflow.service.port`。
- 确认 Kubernetes NetworkPolicy 或外部防火墙允许 UDP 流量。

如果行已写入，但没有 Pod 字段：

- 确认 sampled record 中的端点 IP 是 Pod IP，而不只是 Node 地址或 NAT 后地址。
- 确认目标 Pod 处于 Running 状态，并已有 Pod IP。
- 确认 collector RBAC 可以 list Pods，并读取 owner 对象。
- Pod 创建后，等待一个 Pod cache refresh interval 再检查。

如果 ClickHouse 写入失败：

- 确认 `sflow.clickhouse.address` 包含 host 和 port。
- 确认表已创建，并与 `docs/sql/sflow-flows-raw.sql` 匹配。
- 确认配置的 username 和 Secret password 有效。
- 检查 `unifabric_sflow_write_errors_total` 和 collector 日志。

如果记录被丢弃：

- 检查 `unifabric_sflow_queue_depth` 和 `unifabric_sflow_records_dropped_total`。
- 提高交换机 sampling rate，降低输入量。
- 增大 `sflow.writer.queueSize` 或 `sflow.writer.batchSize`。
- 检查 ClickHouse insert latency 和资源使用情况。
