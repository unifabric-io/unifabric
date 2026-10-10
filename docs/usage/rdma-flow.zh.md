# eBPF RDMA flow 观测

English: [rdma-flow.md](./rdma-flow.md)

## 介绍

宿主机 RDMA 计数器只能说明某块设备或某个 Pod 发了多少，说不清发给了谁。
ebpf 级别的 flow 观测在不改动应用的前提下把 RDMA 发送流量归属到
Pod 到 Pod 的流，运维人员可以看到哪个训练或推理任务在和哪个对端通信、
消息有多大。

### 适用范围

该功能只观测 **CPU initiated RDMA**，判断标准只有一条：发送 WR 是否由 CPU
通过 `libibverbs` 提交。数据在 GPU 显存中不影响观测，GPUDirect RDMA 只要由
CPU 提交仍然可见。GPU kernel 直接驱动 NIC 的 GDAKI、GPUDirect Async、IBGDA
等 **GPU initiated** 路径绕过 `libibverbs`，无法观测。

同一个框架可能同时存在两类路径，需要按实际使用的通信方式判断。

#### 推理框架视角

| 推理框架 | 通信场景 | 底层通信库 | 是否支持观测 RDMA 流量 |
| --- | --- | --- | --- |
| SGLang | TP | NCCL | 支持 |
| | PP | NCCL | 支持 |
| | EP | `none`（默认）、DeepEP、Mooncake、NIXL-EP、FlashInfer | 通过 `--moe-a2a-backend` 选择。仅 `none` 通过 NCCL 通信，可观测。DeepEP、Mooncake、NIXL-EP、FlashInfer 不支持。 |
| | Prefill/Decode 分离的 KV Cache 传输 | | |
| vLLM | TP | NCCL | 支持 |
| | PP | NCCL | 支持 |
| | EP（MoE 通信） | `allgather_reducescatter`、`naive`、DeepEP（high-throughput / low-latency）、FlashInfer | 通过 `--all2all-backend` 选择。仅 `allgather_reducescatter` 和 `naive` 可观测。 |
| | Prefill/Decode 分离的 KV Cache 传输 | NixlConnector（NIXL）、MooncakeConnector（Mooncake）、P2pNcclConnector（NCCL）、LMCacheConnector | 支持。使用 LMCacheConnector 时需设置环境变量 `UCX_TLS=rc` 以走 RDMA。 |

#### 通信库视角

| 通信库 | 是否支持观测 RDMA 流量 |
| --- | --- |
| NCCL | 普通 collective（`ncclAllReduce` 等）走 [`src/transport/net_ib.cc`](https://github.com/NVIDIA/nccl/blob/v2.29.2-1/src/transport/net_ib.cc) 的 CPU proxy 线程调用 `ibv_post_send`，可观测。仅当应用使用 NCCL Device API 的 GIN（symmetric memory kernel）时才进入 [`src/gin/gin_host.cc`](https://github.com/NVIDIA/nccl/blob/v2.29.2-1/src/gin/gin_host.cc) 的 GIN 路径，由两个环境变量决定：`NCCL_GIN_ENABLE`（默认 `1`，设为 `0` 禁用 GIN）和 `NCCL_GIN_TYPE`（默认 `-1` 自动选择，`2` 为 Proxy，`3` 为 GDAKI）。Proxy 由 CPU proxy 线程提交，可观测。GDAKI 由 GPU 直接发起，不可观测。vLLM 和 SGLang 的 TP/PP 使用普通 collective，可观测。 |
| NIXL | UCX backend 走 host IB/RoCE verbs 时支持。DOCA GPUNetIO 和 UCX GPU Device API 不支持。 |
| Mooncake | 支持。 |
| LMCache | 支持，需设置 `UCX_TLS=rc`。 |
| DeepEP | 不支持。 |
| FlashInfer | 不支持。 |
| verbs、MPI、perftest | 支持。 |

## 前提

- Linux 节点支持 BPF ring buffer 与 uprobe，内核 5.8 及以上。
- 要观测的应用必须使用 host `libibverbs` 路径；仅仅使用 RDMA 或 GPU buffer
  不代表可观测。

## 开启步骤

### 基本功能

基本功能从 BPF map 周期性聚合 RDMA flow，并通过 Agent 的 metrics Service
导出 Prometheus metrics，包括每条流的累计字节、WR 与活跃 QP。该模式不产生
逐次发送事件，也不需要配置事件存储后端。

```bash
helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set agent.ebpfFlow.enabled=true \
  --wait
```

如果是 InfiniBand 网络环境还需设置 `--set agent.ebpfFlow.endpoint.allPods=true`，因为 IB 网络对端按 LID 寻址，每个 RDMA Pod 的信息都必须同步到 `RDMAEndpoint` CRD。

全部配置项与默认值见 [Helm values 参考](../../chart/README.md) 的
`agent.ebpfFlow` 部分，它们与 Agent 配置文件的 `ebpfFlow` 段一一对应。

#### 验证

打开 Grafana，查看 Unifabric RDMA Pod/Nodes 面板，确保 RDMA send details (CPU initiated) 组下面的数据显示。

![Unifabric RDMA Pods 面板](../images/pods.png)

![Unifabric RDMA Nodes 面板](../images/nodes.png)


### 详细事件

基本功能导出的是预聚合 Prometheus metrics，详细事件则捕获 RDMA 应用每次
`post_send`，生成包含源/目的 Pod、QP、网卡、发送字节数、WR 数和时间戳的
event record。Agent 通过 OTLP 把 event record 发送给 Event Collector，再由
Collector 写入事件存储后端。

`flowEvents` 默认关闭。开启详细事件时必须同时选择至少一个存储后端。当前
Chart 支持 **ClickHouse** 和 **Elasticsearch**，可以启用其中一个，也可以同时
启用；当前尚不支持 Kafka。

#### 使用内置 ClickHouse

下面的命令开启详细事件、Event Collector 和内置 ClickHouse：

```bash
helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set agent.ebpfFlow.enabled=true \
  --set agent.ebpfFlow.flowEvents.enabled=true \
  --set ebpfFlowEventCollector.clickhouse.enabled=true \
  --wait
```

不填写 `clickhouse.endpoint` 时，Chart 会部署内置单副本 ClickHouse。默认用户
为 `rdma`，默认密码为 `rdma-flow`，数据表为 `rdma.rdma_sends`。默认使用
20Gi PVC 和集群默认 StorageClass。内置实例没有复制与备份，仅用于测试和评估。

#### 使用内置 Elasticsearch

```bash
helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set agent.ebpfFlow.enabled=true \
  --set agent.ebpfFlow.flowEvents.enabled=true \
  --set ebpfFlowEventCollector.elasticsearch.enabled=true \
  --wait
```

不填写 `elasticsearch.endpoints` 时，Chart 会部署内置单节点 Elasticsearch。
默认用户为 `elastic`，默认密码为 `rdma-flow`，事件写入 `rdma-sends` data
stream。数据默认使用 `emptyDir`，Pod 重建后会丢失，仅用于测试和评估。

#### 使用外置 ClickHouse

外置 ClickHouse 需要填写 native protocol endpoint，并提供密码或包含
`password` 键的 Secret：

```bash
kubectl -n unifabric-system create secret generic clickhouse-credentials \
  --from-literal=password='<clickhouse-password>'
```

将后端配置写入 values 文件：

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

也可以直接设置 `clickhouse.password`，但生产环境建议使用 Secret。填写
`endpoint` 后 Chart 不会部署内置 ClickHouse。

#### 使用外置 Elasticsearch

先创建包含 `password` 键的 Secret：

```bash
kubectl -n unifabric-system create secret generic elasticsearch-credentials \
  --from-literal=password='<elasticsearch-password>'
```

将后端配置写入 values 文件：

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

也可以直接设置 `elasticsearch.password`。填写 `endpoints` 后 Chart 不会部署
内置 Elasticsearch。两个后端同时启用时会收到相同的 event record，其中一个
不可用不会影响另一个。

将外置后端配置保存为 `rdma-flow-events.yaml` 后执行：

```bash
helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  -f rdma-flow-events.yaml \
  --wait
```

#### 事件参数与验证

- Agent 自动连接 Chart 部署的 Event Collector Service，无需配置 OTLP endpoint。
- `flowEvents.aggregate` 把窗口内同一 QP 的相同提交合并为一条 event record
  并带 `count`；`flowEvents.sample` 每 N 条事件保留一条。
- Elasticsearch 通常比 ClickHouse 占用更多磁盘，适合与日志关联；ClickHouse
  更适合聚合分析。

可通过 Prometheus 检查事件采集速率和消息大小分布：

```promql
sum by (src_pod_name, dst_pod_name) (rate(rdma_flow_sends_total[1m]))
histogram_quantile(0.5, sum by (le) (rate(rdma_flow_send_bytes_bucket[5m])))
```

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

### Pods 的 Grafana 看板没有数据

1. 先确认应用满足观测前提：

   - 通信库实际使用 CPU 发起的 host `libibverbs` 路径。可参考上面的兼容性表，
     并结合 NCCL、UCX 或应用日志确认。GPU initiated 路径无法观测。
   - 排查期间应用确实产生了 RDMA 通信。通过应用日志、通信测试结果或 RDMA
     设备计数器确认有流量。

   任一条件不成立时都不会产生 flow 数据，无需继续检查 Agent 或 Grafana。

下面的命令假设命名空间为 `unifabric-system`。如果安装时使用了其他命名空间，
请相应修改 `NAMESPACE`。

2. 确认 Agent 已经在所有目标节点上运行，并选取一个有 RDMA 流量的 Agent Pod：

   ```bash
   NAMESPACE=unifabric-system
   kubectl -n "$NAMESPACE" get daemonset,pod \
     -l app.kubernetes.io/component=unifabric-agent -o wide

   AGENT_POD=$(kubectl -n "$NAMESPACE" get pod \
     -l app.kubernetes.io/component=unifabric-agent \
     --field-selector=status.phase=Running \
     -o jsonpath='{.items[0].metadata.name}')
   echo "$AGENT_POD"
   ```

   DaemonSet 的 `DESIRED`、`CURRENT` 与 `READY` 应一致。还要确认
   `agent.ebpfFlow.enabled=true`；未开启时 Agent 不会注册 `rdma_flow_*` 指标。

3. 直接检查该 Agent 的指标，区分采集问题和 Grafana/Prometheus 问题。在一个
   终端执行端口转发：

   ```bash
   kubectl -n "$NAMESPACE" port-forward "pod/$AGENT_POD" 8082:8082
   ```

   在另一个终端执行：

   ```bash
   curl -s http://127.0.0.1:8082/metrics | grep '^rdma_flow_'
   curl -s http://127.0.0.1:8082/metrics | \
     grep -E '^rdma_flow_agent_(qp_infos|qp_stats|qp_matched|qp_unmatched|qp_unresolved) '
   ```

   - 完全没有 `rdma_flow_*`：检查功能是否开启，以及下一步中的 BPF 加载日志。
   - `rdma_flow_agent_qp_stats` 为 `0`：Agent 没有观测到发送，确认应用正在产生
     host initiated RDMA 流量，并检查数据面探针是否挂载。
   - `rdma_flow_agent_qp_stats` 大于 `0`，但 `qp_matched` 为 `0`：检查
     `qp_infos` 和 provider callback。
   - `rdma_flow_agent_qp_unresolved` 大于 `0`：Agent 已发现 QP，但无法把源或目的
     地址解析为 Pod。InfiniBand 环境需确认
     `agent.ebpfFlow.endpoint.allPods=true`，并检查 `RDMAEndpoint` 是否齐全。
   - 已有 `rdma_flow_bytes_total`、`rdma_flow_wrs_total` 或 `rdma_flow_qps` 数据：
     Agent 采集正常，继续检查 Prometheus 和 Grafana。

4. 检查 BPF 对象、探针与固定 map：

   ```bash
   kubectl -n "$NAMESPACE" logs "$AGENT_POD" -c agent | \
     grep -E 'bpf_collection_loaded|bootstrap_probes_attached|provider_callback_restored|callback_received|load bpf collection|attach_bootstrap_probes'

   kubectl -n "$NAMESPACE" exec "$AGENT_POD" -c agent -- \
     ls -l /sys/fs/bpf/unifabric
   ```

   日志中应出现 `bpf_collection_loaded` 和 `bootstrap_probes_attached`。负载创建
   QP 后应出现 `provider_callback_restored`；`callback_received` 仅在 debug
   日志级别输出。`/sys/fs/bpf/unifabric` 下应有 `qp_infos` 和 `qp_stats`。
   如果节点上已安装 `bpftool`，可进一步查看 map 内容：

   ```bash
   bpftool map dump pinned /sys/fs/bpf/unifabric/qp_infos
   bpftool map dump pinned /sys/fs/bpf/unifabric/qp_stats
   ```

   `qp_infos` 为空通常说明 QP 尚未进入 RTR，或 provider callback 没有被捕获；
   `qp_stats` 为空通常说明应用没有通过受支持的 `libibverbs` 路径提交发送。

5. 如果 Agent 本地已有流量指标，检查 Prometheus 抓取链路：

   ```bash
   kubectl -n "$NAMESPACE" get service,servicemonitor \
     -l app.kubernetes.io/component=unifabric-agent
   ```

   默认情况下 Chart 会创建 Agent metrics Service 和 ServiceMonitor。确认
   Prometheus 的 Targets 页面中对应 target 为 `UP`；如果 Prometheus 通过
   label 选择 ServiceMonitor，还需确保
   `nodeMetrics.serviceMonitor.labels` 满足该选择器。随后在 Prometheus 或
   Grafana Explore 中直接执行：

    ```promql
    sum(rate(rdma_flow_bytes_total[5m]))
    ```

    查询有结果但看板为空时，检查看板时间范围以及 namespace、workload、Pod
    等变量是否选中了实际产生 RDMA 流量的对象。
