# 基于 eBPF 的 RDMA tracing

> 成熟度: Alpha

English version: [rdma-flow.md](./rdma-flow.md)

## 简介

网卡计数器只能记录一张网卡或一个 Pod 发送了多少流量,无法记录流量发给了谁。基于 eBPF 的 RDMA tracing 在 `libibverbs` 的提交路径上挂载 eBPF 探针,无需修改应用,就能把每次 RDMA 发送归属到一条 Pod 之间的流。运维可以借此查看训练任务和推理任务的通信对端、流量大小和消息大小。

本文说明如何开启、验证和排查基于 eBPF 的 RDMA tracing。

| 项目 | 说明 |
| --- | --- |
| 适用网络 | Scale-out、Storage |
| 数据来源 | `libibverbs` 提交路径上的 eBPF uprobe |
| 观测对象 | CPU 发起的 RDMA 通信 |
| 统计维度 | 源 Pod 和目的 Pod、源设备和目的设备、两端所属的任务 |
| 输出 | `rdma_flow_*` 指标、`RDMAEndpoint` 资源,Grafana 中 Pod 和 AI Workload 仪表盘的"RDMA 发送明细(CPU 侧发起)"分组 |
| 节点侧要求 | Linux 内核 5.8 及以上,支持 BPF ring buffer 和 uprobe |
| 默认状态 | 默认关闭 |

Agent 按以下方式还原一条流:

- 数据面: eBPF 探针记录每个 QP 发送的字节数和 WR 数,存入 BPF map。
- 源端: Agent 按发送进程所在的 cgroup 找到源 Pod。
- 目的端: Agent 根据 QP 的目的地址找到目的 Pod。RoCE 网络使用 GID (即 Pod IP 或 MAC),InfiniBand 网络使用 LID。
- `RDMAEndpoint`: 每个 Agent 发布本节点上 RDMA Pod 的设备、GID、LID 和 QP,其他节点的 Agent 用它来解析目的 Pod。

全部指标和标签的定义,参见[指标参考](../../reference/metrics.zh.md#rdma-tracing-指标)。

限制:

- 只能观测 CPU 发起的 RDMA 通信。GPU 发起的 RDMA 通信不经过 `libibverbs`,探针看不到,详见[适用范围](#适用范围)。
- 只能观测用户态通过 `libibverbs` 提交的请求,内核态的 RDMA 协议 (例如 NVMe over RDMA) 不经过 `libibverbs`。
- Agent 在提交请求的一端记录流量,因此一条流的源 Pod 是提交 WR 的 Pod;对于 RDMA Read,数据实际是从目的 Pod 流向源 Pod。
- 源 Pod 或目的 Pod 尚未解析的流暂不计入指标,解析成功后 Agent 会一次性补记此前的流量。

本功能是 AI 高速网络流量观测的一部分,完整的观测范围参见 [AI 高速网络流量观测](./README.zh.md#流量观测范围)。

## 适用范围

基于 eBPF 的 RDMA tracing 只支持 **CPU 发起的 RDMA**,即发送请求 (WR) 必须由 CPU 通过 `libibverbs` 提交。数据是否位于 GPU 显存并不影响观测,因此只要由 CPU 提交,GPUDirect RDMA 仍然可以观测。

GDAKI、GPUDirect Async、IBGDA 等 **GPU 发起的 RDMA** 由 GPU kernel 直接驱动网卡,绕过了 `libibverbs`,因此无法观测。

同一个框架可能同时使用这两类路径,需要按实际使用的通信方式判断。

### 推理框架

| 推理框架 | 通信场景 | 是否支持观测 RDMA 流量 | 信息截至版本 |
| --- | --- | --- | --- |
| SGLang | TP | 使用 NCCL 通信,支持。 | v0.5.21 |
| | PP | 使用 NCCL 通信,支持。 | v0.5.21 |
| | EP | 取决于 SGLang 启动参数 `--moe-a2a-backend`:<br />- `none` (默认): 使用 NCCL 通信,支持。<br />- DeepEP、Mooncake、NIXL-EP、FlashInfer: 不支持。 | v0.5.21 |
| | Prefill/Decode 分离的 KV Cache 传输 | 通过 SGLang 启动参数 `--disaggregation-transfer-backend` 选择:<br />- NIXL、Mooncake: 均由 CPU 提交 RDMA 请求,支持。<br /><br />配置详情见 [PD Disaggregation](https://docs.sglang.io/docs/advanced_features/pd_disaggregation) 文档。 | v0.5.21 |
| vLLM | TP | 使用 NCCL 通信,支持。 | v0.31.0 |
| | PP | 使用 NCCL 通信,支持。 | v0.31.0 |
| | EP (MoE 通信) | 取决于 vLLM 启动参数 `--all2all-backend`:<br />- `allgather_reducescatter`: 使用 NCCL 集合通信,支持。<br />- `deepep_high_throughput`、`deepep_low_latency`、`nixl_ep`、`flashinfer_nvlink_two_sided`、`flashinfer_nvlink_one_sided`、`mori_high_throughput`、`mori_low_latency`、`deepep_v2`、`moonep`: 不支持。<br />详情见 [moe_kernel_features](https://docs.vllm.ai/en/latest/design/moe_kernel_features/) 文档。 | v0.31.0 |
| | Prefill/Decode 分离的 KV Cache 传输 | 取决于 vLLM 启动参数 `--kv-transfer-config` 中的 Connector 和后端配置:<br />- NixlConnector: 通过 `kv_connector_extra_config` 中的 `backends` 数组选择后端:<br />&nbsp;&nbsp;- `UCX` (默认): 由 CPU 提交 RDMA 请求,支持。<br />&nbsp;&nbsp;- `LIBFABRIC`: 由 CPU 提交 RDMA 请求,支持。<br />配置方式见 [nixl_connector_usage](https://docs.vllm.ai/en/latest/features/nixl_connector_usage/)。<br /><br />- MooncakeConnector: `mooncake_protocol` 为 `rdma` (默认值) 时支持。其他值不支持。配置方式见 [mooncake_connector_usage](https://github.com/vllm-project/vllm/blob/main/docs/features/mooncake_connector_usage.md)。<br /><br />- LMCacheConnector: 支持。LMCache 底层使用 UCX 通信库,环境中有 RDMA 时优先使用 RDMA 网络。<br /><br />- MoRIIOConnector: `backend` 为 `rdma` (默认值) 时支持。其他值不支持。配置方式见 [MoRIIOConnector](https://docs.vllm.ai/en/latest/features/moriio_connector_usage/#rdma-backend)。 | v0.31.0<br />LMCache: v0.5.5 |

### 通信库

| 通信库 | 是否支持观测 RDMA 流量 | 信息截至版本 |
| --- | --- | --- |
| NCCL | 取决于实际的 RDMA 提交路径:<br />- 使用 [NCCL API](https://github.com/NVIDIA/nccl/blob/v2.32.3-1/src/nccl.h.in#L602) 时,CPU 代理线程通过 `libibverbs` 提交 RDMA 请求,支持。<br />- 使用 [NCCL-GIN API](https://github.com/NVIDIA/nccl/blob/v2.32.3-1/src/include/nccl_device/gin.h#L356) 时,如果环境变量 `NCCL_GIN_TYPE` 为空 (默认) 或为 `3` (GDAKI),GPU 优先直接驱动网卡,绕过 `libibverbs`,不支持。<br />- 使用 [NCCL-GIN API](https://github.com/NVIDIA/nccl/blob/v2.32.3-1/src/include/nccl_device/gin.h#L356) 时,如果设置 `NCCL_GIN_TYPE=2` (Proxy 模式),由 CPU 提交 RDMA 请求,支持。 | v2.32.3 |
| UCX | 通过环境变量 `UCX_TLS` 选择传输路径:<br />- `rc`、`dc`: 支持。<br />- 其他传输路径: 不支持。 | v1.22.0 |
| DeepEP | 不支持。DeepEP 由 GPU 直接提交 RDMA 请求。 | - |
| FlashInfer | 不支持。FlashInfer 由 GPU 发起 RDMA,CPU 不在关键路径上。 | - |
| LIBFABRIC | 支持。 | v2.7.0 |

## 前提条件

- 满足 [AI 高速网络流量观测](./README.zh.md#前提条件)中的前提条件。
- 节点的 Linux 内核为 5.8 及以上,支持 BPF ring buffer 和 uprobe。
- 要观测的应用通过主机端的 `libibverbs` 路径提交 RDMA 请求。仅仅使用了 RDMA 设备或 GPU 缓冲区,并不代表应用可以被观测。

## 配置

执行以下命令,开启基于 eBPF 的 RDMA tracing:

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set agent.ebpfFlow.enabled=true \
  --wait
```

如果 Scale-out 网络或 Storage 网络是 InfiniBand,还需要添加 `--set agent.ebpfFlow.endpoint.allPods=true`。这是因为 InfiniBand 网络按 LID 寻址对端,Agent 无法从 LID 反查 Pod;开启这个参数后,Agent 会为每个 RDMA Pod 发布 `RDMAEndpoint`,其他节点的 Agent 据此解析目的 Pod。默认情况下,Agent 只为 hostNetwork Pod 和尚未进入 IP 索引的 Pod 发布 `RDMAEndpoint`,其他 Pod 通过 Pod IP 解析。

开启后,Agent 默认每 5 秒把 BPF map 中的数据汇总为流指标。这种方式不产生逐次发送事件,也不需要存储后端;如果需要每次发送的记录和消息大小分布,请继续配置 [RDMA 发送事件存储](./rdma-flow-events.zh.md)。

全部配置项参见 [Helm values 参考](../../../chart/README.md)中的 `agent.ebpfFlow.*`。

## 验证

1. 确认 Agent 已加载 BPF 程序并挂载探针:

   ```bash
   kubectl -n unifabric-system logs ds/unifabric-agent -c agent | \
     grep -E 'bpf_collection_loaded|bootstrap_probes_attached'
   ```

   <!-- TODO: 待真实环境采集输出 -->

2. 在应用产生 RDMA 通信后,对 Agent Pod 执行端口转发,并查看流指标:

   ```bash
   AGENT_POD=$(kubectl -n unifabric-system get pod \
     -l app.kubernetes.io/component=unifabric-agent \
     --field-selector=status.phase=Running \
     -o jsonpath='{.items[0].metadata.name}')
   kubectl -n unifabric-system port-forward "pod/$AGENT_POD" 8082:8082
   ```

   在另一个终端中执行:

   ```bash
   curl -s http://127.0.0.1:8082/metrics | grep '^rdma_flow_bytes_total'
   ```

   <!-- TODO: 待真实环境采集输出 -->

3. 确认 `RDMAEndpoint` 资源存在:

   ```bash
   kubectl get rdmaendpoints -A
   ```

   <!-- TODO: 待真实环境采集输出 -->

## 查看结果

打开 Grafana 的 Unifabric RDMA Pod 仪表盘或 Unifabric RDMA AI Workload 仪表盘,展开"RDMA 发送明细(CPU 侧发起)"分组。

### Unifabric RDMA Pod

选择 namespace 和 Pod,查看这个 Pod 和谁通信:

- 累计发送字节、累计发送次数、当前活跃 QP 和时间范围内的对端 Pod 数。
- 源 Pod 到每个目的 Pod 的累计字节数,以及每对网卡之间的累计流量。
- 消息大小的分布和最常见的消息大小。
- 源 Pod 内每个进程使用的 QP,以及最近的发送记录。

通过这个仪表盘可以确认一个 Pod 的通信对端和消息大小是否符合预期。同一仪表盘中网卡吞吐和错误的分组来自[主机与 Pod 的 RDMA 网卡指标](./rdma-metrics.zh.md#unifabric-rdma-pod),把两部分放在一起看,就能判断一个 Pod 的流量发给了谁,以及它的网卡上有没有拥塞和错误。

![Unifabric RDMA Pod 仪表盘](../../images/pods.png)

### Unifabric RDMA AI Workload

选择任务的 kind、namespace 和 name,例如一个 `PyTorchJob`,查看这个任务对外的通信:

- 任务累计发送的字节数、WR 数、发送次数和活跃 QP。
- 按源 Pod 和按对端 Pod 汇总的字节数。
- 任务对外的每一条流: 源 Pod、目的 Pod 和两端的网卡。

通过这个仪表盘可以找到一个任务中通信最多的 Pod 及其对端。

![Unifabric RDMA AI Workload 仪表盘](../../images/workloads.png)

也可以直接在 Prometheus 中按 Pod 查询,例如查看一个 Pod 发往每个对端的字节数:

```promql
sum by (dst_pod_namespace, dst_pod_name) (rate(rdma_flow_bytes_total{src_pod_namespace="tenant-a", src_pod_name="vllm-decode-0"}[5m]))
```

### 面板需要的配置

部分面板需要开启 [RDMA 发送事件存储](./rdma-flow-events.zh.md):

| 面板 | 需要的配置 |
| --- | --- |
| 累计发送字节、累计 WR、当前活跃 QP、源 Pod 到目的 Pod 累计通讯字节、按源 Pod 累计发送字节、按对端 Pod 累计发送字节、Workload 对外流量明细 | 本文的配置 |
| 累计发送次数、时间范围内对端 Pod 数、按源与目的 Pod 的累计发送和大小分位、网卡对累计通讯 | 还需开启 RDMA 发送事件存储 |
| 最常见的消息大小 Top 20、源 Pod 内的进程与 QP、最近发送记录 | 还需开启 RDMA 发送事件存储,并使用 Elasticsearch 后端 |

## 故障排查

### 仪表盘中没有 RDMA 流数据

以下命令假设安装命名空间为 `unifabric-system`,如果使用了其他命名空间,请修改 `NAMESPACE`。

1. 确认应用满足观测条件:

   - 通信库实际使用的是 CPU 发起的 `libibverbs` 路径。请参考[适用范围](#适用范围),并结合 NCCL、UCX 或应用日志确认。
   - 排查期间应用确实产生了 RDMA 通信。可以通过应用日志、通信测试结果或[主机与 Pod 的 RDMA 网卡指标](./rdma-metrics.zh.md)确认有流量。

   只要任一条件不成立,就不会产生流数据,也就无需继续检查 Agent 或 Grafana。

2. 确认 Agent 在所有目标节点上运行,并选取一个位于有 RDMA 流量节点上的 Agent Pod:

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

   DaemonSet 的 `DESIRED`、`CURRENT` 和 `READY` 应当一致。同时确认已设置 `agent.ebpfFlow.enabled=true`,否则 Agent 不会注册 `rdma_flow_*` 指标。

3. 直接查看这个 Agent 的指标,以区分是采集问题还是 Prometheus、Grafana 的问题。在一个终端中执行端口转发:

   ```bash
   kubectl -n "$NAMESPACE" port-forward "pod/$AGENT_POD" 8082:8082
   ```

   在另一个终端中执行:

   ```bash
   curl -s http://127.0.0.1:8082/metrics | grep '^rdma_flow_'
   curl -s http://127.0.0.1:8082/metrics | \
     grep -E '^rdma_flow_agent_(qp_infos|qp_stats|qp_matched|qp_unmatched|qp_unresolved) '
   ```

   - 没有任何 `rdma_flow_*` 指标: 确认功能已开启,然后按下一步检查 BPF 加载日志。
   - `rdma_flow_agent_qp_stats` 为 `0`: Agent 没有观测到任何发送。确认应用正在产生 CPU 发起的 RDMA 流量,并检查探针是否已挂载。
   - `rdma_flow_agent_qp_stats` 大于 `0`,但 `rdma_flow_agent_qp_matched` 为 `0`: 检查 `qp_infos` 和 provider 回调。
   - `rdma_flow_agent_qp_unresolved` 大于 `0`: Agent 发现了 QP,但无法把源地址或目的地址解析为 Pod。在 InfiniBand 网络中,确认已设置 `agent.ebpfFlow.endpoint.allPods=true`,并且 `RDMAEndpoint` 齐全。
   - 已有 `rdma_flow_bytes_total`、`rdma_flow_wrs_total` 或 `rdma_flow_qps`: Agent 采集正常,继续检查 Prometheus 和 Grafana。

4. 检查 BPF 对象、探针和固定的 BPF map:

   ```bash
   kubectl -n "$NAMESPACE" logs "$AGENT_POD" -c agent | \
     grep -E 'bpf_collection_loaded|bootstrap_probes_attached|provider_callback_restored|callback_received|load bpf collection|attach_bootstrap_probes'

   kubectl -n "$NAMESPACE" exec "$AGENT_POD" -c agent -- \
     ls -l /sys/fs/bpf/unifabric
   ```

   正常情况下,日志中会出现 `bpf_collection_loaded` 和 `bootstrap_probes_attached`;应用创建 QP 后,还会出现 `provider_callback_restored` (`callback_received` 只在 debug 日志级别输出)。`/sys/fs/bpf/unifabric` 目录下应当有 `qp_infos` 和 `qp_stats`。

   如果节点上安装了 `bpftool`,可以查看 map 的内容:

   ```bash
   bpftool map dump pinned /sys/fs/bpf/unifabric/qp_infos
   bpftool map dump pinned /sys/fs/bpf/unifabric/qp_stats
   ```

   `qp_infos` 为空,通常说明 QP 尚未进入 RTR 状态,或者 Agent 没有捕获到 provider 回调;`qp_stats` 为空,通常说明应用没有通过受支持的 `libibverbs` 路径提交发送。

5. 如果 Agent 本地已有流指标,检查 Prometheus 的抓取 (参见[主机与 Pod 的 RDMA 网卡指标](./rdma-metrics.zh.md#prometheus-中没有指标)),然后在 Prometheus 或 Grafana Explore 中执行:

   ```promql
   sum(rate(rdma_flow_bytes_total[5m]))
   ```

   如果查询有结果但仪表盘为空,检查仪表盘的时间范围,以及 namespace、workload、Pod 等变量是否选中了实际产生 RDMA 流量的对象。

### 部分面板没有数据

发送次数和消息大小相关的面板需要开启 [RDMA 发送事件存储](./rdma-flow-events.zh.md),参见[面板需要的配置](#面板需要的配置)。

### workload 标签停在中间资源

参见[主机与 Pod 的 RDMA 网卡指标](./rdma-metrics.zh.md#workload-标签为空或停在中间资源)。

## 参考

- [AI 高速网络流量观测](./README.zh.md): 完整的观测范围。
- [RDMA 发送事件存储](./rdma-flow-events.zh.md): 记录每次 RDMA 发送。
- [指标参考](../../reference/metrics.zh.md): 全部指标和标签。
- [eBPF RDMA 流归因设计](../../design/rdma-flow.zh.md): 数据面、归属和 `RDMAEndpoint` 的设计。
- [RDMAEndpoint 参考](../../reference/rdmaendpoint.zh.md): `RDMAEndpoint` 的字段。
- [Helm values 参考](../../../chart/README.md): 全部 `agent.ebpfFlow.*` 参数。
