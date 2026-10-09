# 基于 eBPF 观察 RDMA 流量归属

English: [rdma-flow.md](./rdma-flow.md)

## 介绍

宿主机 RDMA 计数器只能反映设备或 Pod 的发送量，无法说明流量发往哪个对端。eBPF RDMA 流量观测无需修改应用，即可将发送流量归属到 Pod 间的流，帮助运维人员了解训练或推理任务的通信对端和消息大小。

### 适用范围

该功能仅支持观测 **CPU 发起的 RDMA（CPU-initiated RDMA）**：发送工作请求（WR）必须由 CPU 通过 `libibverbs` 提交。数据位于 GPU 显存中不影响观测；GPUDirect RDMA 只要由 CPU 提交，仍可观测。

GDAKI、GPUDirect Async、IBGDA 等 **GPU 发起的 RDMA（GPU-initiated RDMA）** 路径由 GPU 内核直接驱动网卡，绕过 `libibverbs`，无法观测。

同一个框架可能同时存在两类路径，需要按实际使用的通信方式判断。

#### 推理框架视角

| 推理框架 | 通信场景 | 是否支持观测 RDMA 流量 | 信息截至版本 |
| --- | --- | --- | --- |
| SGLang | TP | 使用 NCCL 通信，支持。 | v0.5.21 |
| | PP | 使用 NCCL 通信，支持。 | v0.5.21 |
| | EP | 取决于 SGLang 启动参数 `--moe-a2a-backend`：<br />- `none`（默认）：使用 NCCL 通信，支持。<br />- DeepEP、Mooncake、NIXL-EP、FlashInfer：不支持。 | v0.5.21 |
| | Prefill/Decode 分离的 KV Cache 传输 | 通过 SGLang 启动参数 `--disaggregation-transfer-backend` 选择：<br />- NIXL、Mooncake：均由 CPU 提交 RDMA 请求，支持。<br /><br />配置详情见 [PD Disaggregation](https://docs.sglang.io/docs/advanced_features/pd_disaggregation) 文档。 | v0.5.21 |
| vLLM | TP | 使用 NCCL 通信，支持。 | v0.31.0 |
| | PP | 使用 NCCL 通信，支持。 | v0.31.0 |
| | EP（MoE 通信） | 取决于 vLLM 启动参数 `--all2all-backend`：<br />- `allgather_reducescatter`：使用 NCCL 集合通信，支持。<br />- `deepep_high_throughput`、`deepep_low_latency`、`nixl_ep`、`flashinfer_nvlink_two_sided`、`flashinfer_nvlink_one_sided`、`mori_high_throughput`、`mori_low_latency`、`deepep_v2`、`moonep`：不支持。<br />详情见 [moe_kernel_features](https://docs.vllm.ai/en/latest/design/moe_kernel_features/) 文档。 | v0.31.0 |
| | Prefill/Decode 分离的 KV Cache 传输 | 取决于 vLLM 启动参数 `--kv-transfer-config` 中的 Connector 和后端配置：<br />- NixlConnector：通过 `kv_connector_extra_config` 中的 `backends` 数组选择后端：<br />&nbsp;&nbsp;- `UCX`（默认）：由 CPU 提交 RDMA 请求，支持。<br />&nbsp;&nbsp;- `LIBFABRIC`：由 CPU 提交 RDMA 请求，支持。<br />配置方式见 [nixl_connector_usage](https://docs.vllm.ai/en/latest/features/nixl_connector_usage/)。<br /><br />- MooncakeConnector：`mooncake_protocol` 默认值为 `rdma` 时支持。其他值不支持。配置参考 [mooncake_connector_usage](https://github.com/vllm-project/vllm/blob/main/docs/features/mooncake_connector_usage.md)。<br /><br />- LMCacheConnector：支持。LMCache 底层使用 UCX 通信库，环境有 RDMA 时，优先使用 RDMA 网络。<br /><br />- MoRIIOConnector： `backend`  默认值为 `rdma` 时支持，其它值不支持。配置方式见 [MoRIIOConnector](https://docs.vllm.ai/en/latest/features/moriio_connector_usage/#rdma-backend)。 | v0.31.0<br />LMCache：v0.5.5 |

#### 通信库支持矩阵

| 通信库 | 是否支持观测 RDMA 流量 | 信息截至版本 |
| --- | --- | --- |
| NCCL | 是否支持取决于实际的 RDMA 提交路径：<br />- 使用 [NCCL API](https://github.com/NVIDIA/nccl/blob/v2.32.3-1/src/nccl.h.in#L602) ，由 CPU 代理线程通过 `libibverbs` 提交 RDMA 请求，支持。<br />- 使用 [NCCL-GIN API](https://github.com/NVIDIA/nccl/blob/v2.32.3-1/src/include/nccl_device/gin.h#L356) ，环境变量 `NCCL_GIN_TYPE` 默认值为空，或者环境变量指定为 `NCCL_GIN_TYPE=3` GDAKI，会优先由 GPU 直接驱动网卡，绕过 `libibverbs`，不支持。<br />- 使用 [NCCL-GIN API](https://github.com/NVIDIA/nccl/blob/v2.32.3-1/src/include/nccl_device/gin.h#L356),  设置环境变量 `NCCL_GIN_TYPE=2` 代表 Proxy 模式，由 CPU 提交 RDMA 请求，支持。 | v2.32.3 |
| UCX | 通过环境变量 `UCX_TLS` 选择传输路径：<br />- `rc`、`dc` 路径：支持。<br />- 其他传输路径：不支持 RDMA 观测。 | v1.22.0 |
| DeepEP | 不支持。DeepEP 由 GPU 直接提交 RDMA 请求。 | - |
| FlashInfer | 不支持。FlashInfer 由 GPU 发起 RDMA，CPU 不在关键路径上。 | - |
| LIBFABRIC | 支持。 | v2.7.0 |

## 前提

- Linux 节点支持 BPF ring buffer 与 uprobe，内核 5.8 及以上。
- 要观测的应用必须使用主机端 `libibverbs` 路径；仅使用 RDMA 或 GPU 缓冲区不代表可观测。

## 开启步骤

### 基本功能

基本功能周期性地从 BPF map 聚合 RDMA 流，并通过 Agent 的指标 Service 导出 Prometheus 指标，包括每条流的累计字节数、累计 WR 数和活跃 QP 数。该模式不产生逐次发送事件，也不需要配置事件存储后端。

```bash
helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set agent.ebpfFlow.enabled=true \
  --wait
```

InfiniBand 网络环境还需设置 `--set agent.ebpfFlow.endpoint.allPods=true`。由于 IB 网络对端按 LID 寻址，每个 RDMA Pod 的信息都必须同步到 `RDMAEndpoint` 自定义资源。

全部配置项与默认值见 [Helm values 参考](../../chart/README.md) 的 `agent.ebpfFlow` 部分，与 Agent 配置文件的 `ebpfFlow` 段一一对应。

#### 验证

打开 Grafana，查看 Unifabric RDMA Pods 和 Nodes 看板，确认「RDMA send details (CPU initiated)」分组下的面板显示数据。

![Unifabric RDMA Pods 看板](../images/pods.png)

![Unifabric RDMA Nodes 看板](../images/nodes.png)

### 详细事件

基本功能导出预聚合的 Prometheus 指标；详细事件则捕获 RDMA 应用的每次 `post_send`，生成包含源 Pod、目的 Pod、QP、网卡、发送字节数、WR 数和时间戳的事件记录。Agent 通过 OTLP 将事件记录发送给 Event Collector，再由 Event Collector 写入事件存储后端。

`flowEvents` 默认关闭。开启详细事件时，必须同时启用至少一个存储后端。当前 Chart 支持 **ClickHouse** 和 **Elasticsearch**，可启用其中一个或同时启用两者；尚不支持 Kafka。

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

不填写 `clickhouse.endpoint` 时，Chart 会部署内置单副本 ClickHouse。默认用户为 `rdma`，默认密码为 `rdma-flow`，数据表为 `rdma.rdma_sends`。默认使用容量为 `20Gi` 的 PVC 和集群默认 StorageClass。

内置实例没有复制与备份，仅用于测试和评估。

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

不填写 `elasticsearch.endpoints` 时，Chart 会部署内置单节点 Elasticsearch。默认用户为 `elastic`，默认密码为 `rdma-flow`，事件写入 `rdma-sends` 数据流（data stream）。

数据默认存储在 `emptyDir` 中，Pod 重建后会丢失。内置实例仅用于测试和评估。

#### 使用外置 ClickHouse

使用外置 ClickHouse 时，需要填写原生协议端点（native protocol endpoint），并提供密码或包含 `password` 键的 Secret：

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

也可以直接设置 `clickhouse.password`，但生产环境建议使用 Secret。填写 `endpoint` 后，Chart 不会部署内置 ClickHouse。

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

也可以直接设置 `elasticsearch.password`。填写 `endpoints` 后，Chart 不会部署内置 Elasticsearch。

两个后端同时启用时，会收到相同的事件记录；其中一个后端不可用不会影响另一个。

将外置后端配置保存为 `rdma-flow-events.yaml` 后执行：

```bash
helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  -f rdma-flow-events.yaml \
  --wait
```

#### 事件参数与验证

- Agent 自动连接 Chart 部署的 Event Collector Service，无需配置 OTLP 端点。
- `flowEvents.aggregate` 将窗口内同一 QP 的相同提交合并为一条事件记录，并附带 `count`；`flowEvents.sample` 每 N 条事件保留一条。
- Elasticsearch 通常比 ClickHouse 占用更多磁盘空间，适合与日志关联；ClickHouse 更适合聚合分析。

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

流量标签包含每个 Pod 的顶层 owner。Agent 只沿有权限读取的资源类型向上追溯 `ownerReferences`。因此，对于 Argo Workflows 这类控制器，需要先添加读取权限，才能将 Pod 的 owner 解析为 Workflow，而不是中间资源。

第三方资源类型的权限规则定义在 values 的 `agent.workloadOwnerRules` 中。这是一个以 API 组为键、以复数资源名列表为值的映射。用户提供的 values 会与默认值合并；添加控制器权限规则时，可增加对应的 API 组及资源名：

```yaml
agent:
  workloadOwnerRules:
    argoproj.io: [workflows]
```

Chart 将这些规则渲染为 `<release>-agent-workload-owners` ClusterRole。流量解析实际追溯的资源类型列在 `pkg/agent/ebpfflow/owners.go` 中；新增 API 组时，还需在该文件中添加对应条目。

## 排障

### Pods 的 Grafana 看板没有数据

以下命令假设安装命名空间为 `unifabric-system`。如果安装时使用了其他命名空间，请相应修改 `NAMESPACE`。

1. 先确认应用满足观测前提：

   - 通信库实际使用由 CPU 发起的主机端 `libibverbs` 路径。参考上方的支持说明，并结合 NCCL、UCX 或应用日志确认。GPU 发起的 RDMA 路径无法观测。
   - 排查期间应用确实产生了 RDMA 通信。通过应用日志、通信测试结果或 RDMA 设备计数器确认有流量。

   任一条件不成立时，都不会产生流量观测数据，无需继续检查 Agent 或 Grafana。

2. 确认 Agent 已在所有目标节点上运行，并选取一个位于有 RDMA 流量节点上的 Agent Pod：

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

   DaemonSet 的 `DESIRED`、`CURRENT` 与 `READY` 应一致。还需确认 `agent.ebpfFlow.enabled=true`；未开启时，Agent 不会注册 `rdma_flow_*` 指标。

3. 直接检查所选 Agent 的指标，区分采集问题与 Grafana 或 Prometheus 问题。在一个终端执行端口转发：

   ```bash
   kubectl -n "$NAMESPACE" port-forward "pod/$AGENT_POD" 8082:8082
   ```

   在另一个终端执行：

   ```bash
   curl -s http://127.0.0.1:8082/metrics | grep '^rdma_flow_'
   curl -s http://127.0.0.1:8082/metrics | \
     grep -E '^rdma_flow_agent_(qp_infos|qp_stats|qp_matched|qp_unmatched|qp_unresolved) '
   ```

   - 完全没有 `rdma_flow_*` 指标：检查功能是否开启，并按下一步检查 BPF 加载日志。
   - `rdma_flow_agent_qp_stats` 为 `0`：Agent 没有观测到发送。确认应用正在产生由 CPU 发起的 RDMA 流量，并检查数据面探针是否挂载。
   - `rdma_flow_agent_qp_stats` 大于 `0`，但 `qp_matched` 为 `0`：检查 `qp_infos` 和 provider 回调。
   - `rdma_flow_agent_qp_unresolved` 大于 `0`：Agent 已发现 QP，但无法将源地址或目的地址解析为 Pod。InfiniBand 环境需确认 `agent.ebpfFlow.endpoint.allPods=true`，并检查 `RDMAEndpoint` 是否齐全。
   - 已有 `rdma_flow_bytes_total`、`rdma_flow_wrs_total` 或 `rdma_flow_qps` 数据：Agent 采集正常，继续检查 Prometheus 和 Grafana。

4. 检查 BPF 对象、探针与固定 map：

   ```bash
   kubectl -n "$NAMESPACE" logs "$AGENT_POD" -c agent | \
     grep -E 'bpf_collection_loaded|bootstrap_probes_attached|provider_callback_restored|callback_received|load bpf collection|attach_bootstrap_probes'

   kubectl -n "$NAMESPACE" exec "$AGENT_POD" -c agent -- \
     ls -l /sys/fs/bpf/unifabric
   ```

   日志中应出现 `bpf_collection_loaded` 和 `bootstrap_probes_attached`。工作负载创建 QP 后应出现 `provider_callback_restored`；`callback_received` 仅在 debug 日志级别输出。`/sys/fs/bpf/unifabric` 下应有 `qp_infos` 和 `qp_stats`。

   如果节点上已安装 `bpftool`，可进一步查看 map 内容：

   ```bash
   bpftool map dump pinned /sys/fs/bpf/unifabric/qp_infos
   bpftool map dump pinned /sys/fs/bpf/unifabric/qp_stats
   ```

   `qp_infos` 为空通常说明 QP 尚未进入 RTR，或 provider 回调未被捕获；`qp_stats` 为空通常说明应用未通过受支持的 `libibverbs` 路径提交发送。

5. 如果 Agent 本地已有流量指标，检查 Prometheus 抓取链路：

   ```bash
   kubectl -n "$NAMESPACE" get service,servicemonitor \
     -l app.kubernetes.io/component=unifabric-agent
   ```

   默认情况下，Chart 会创建 Agent 指标 Service 和 ServiceMonitor。确认 Prometheus 的 Targets 页面中对应抓取目标为 `UP`；如果 Prometheus 通过标签选择 ServiceMonitor，还需确保 `nodeMetrics.serviceMonitor.labels` 满足选择器要求。随后在 Prometheus 或 Grafana Explore 中执行：

    ```promql
    sum(rate(rdma_flow_bytes_total[5m]))
    ```

   查询有结果但看板为空时，检查看板时间范围，以及 namespace、workload、Pod 等变量是否选中了实际产生 RDMA 流量的对象。
