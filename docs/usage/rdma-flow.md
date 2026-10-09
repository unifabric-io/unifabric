# Observing RDMA Flow Attribution with eBPF

Chinese: [rdma-flow.zh.md](./rdma-flow.zh.md)

## Introduction

Host RDMA counters show how much a device or Pod has sent, but not which peer
received the traffic. eBPF RDMA flow observability attributes send traffic to
Pod-to-Pod flows without modifying applications, helping operators identify
the communication peers and message sizes of training or inference workloads.

### Scope

This feature observes only **CPU-initiated RDMA**: send work requests (WRs)
must be submitted by the CPU through `libibverbs`. Data in GPU memory does
not affect observability; GPUDirect RDMA remains observable as long as the CPU
submits the WRs.

**GPU-initiated RDMA** paths such as GDAKI, GPUDirect Async, and IBGDA use GPU
kernels to drive the NIC directly, bypassing `libibverbs`, and are not
observable.

A single framework may use both kinds of paths, so judge by the communication
mode actually in use.

#### Inference framework view

| Inference framework | Communication scenario | RDMA flow observable | Information as of version |
| --- | --- | --- | --- |
| SGLang | TP | Uses NCCL; supported. | v0.5.21 |
| | PP | Uses NCCL; supported. | v0.5.21 |
| | EP | Depends on the SGLang startup argument `--moe-a2a-backend`:<br />- `none` (default): uses NCCL; supported.<br />- DeepEP, Mooncake, NIXL-EP, FlashInfer: unsupported. | v0.5.21 |
| | KV cache transfer in Prefill/Decode disaggregation | Selected through the SGLang startup argument `--disaggregation-transfer-backend`:<br />- NIXL, Mooncake: both submit RDMA requests from the CPU and are supported.<br /><br />See the [PD Disaggregation](https://docs.sglang.io/docs/advanced_features/pd_disaggregation) documentation for configuration details. | v0.5.21 |
| vLLM | TP | Uses NCCL; supported. | v0.31.0 |
| | PP | Uses NCCL; supported. | v0.31.0 |
| | EP (MoE communication) | Depends on the vLLM startup argument `--all2all-backend`:<br />- `allgather_reducescatter`: uses NCCL collectives; supported.<br />- `deepep_high_throughput`, `deepep_low_latency`, `nixl_ep`, `flashinfer_nvlink_two_sided`, `flashinfer_nvlink_one_sided`, `mori_high_throughput`, `mori_low_latency`, `deepep_v2`, `moonep`: unsupported.<br />See the [moe_kernel_features](https://docs.vllm.ai/en/latest/design/moe_kernel_features/) documentation for details. | v0.31.0 |
| | KV cache transfer in Prefill/Decode disaggregation | Depends on the connector and backend configuration in the vLLM startup argument `--kv-transfer-config`:<br />- NixlConnector: select the backend through the `backends` array in `kv_connector_extra_config`:<br />&nbsp;&nbsp;- `UCX` (default): submits RDMA requests from the CPU; supported.<br />&nbsp;&nbsp;- `LIBFABRIC`: submits RDMA requests from the CPU; supported.<br />See [nixl_connector_usage](https://docs.vllm.ai/en/latest/features/nixl_connector_usage/) for configuration details.<br /><br />- MooncakeConnector: supported when `mooncake_protocol` is `rdma` (the default). Other values are unsupported. See [mooncake_connector_usage](https://github.com/vllm-project/vllm/blob/main/docs/features/mooncake_connector_usage.md) for configuration details.<br /><br />- LMCacheConnector: supported. LMCache uses the UCX communication library and prefers the RDMA network when RDMA is available in the environment.<br /><br />- MoRIIOConnector: supported when `backend` is `rdma` (the default). Other values are unsupported. See [MoRIIOConnector](https://docs.vllm.ai/en/latest/features/moriio_connector_usage/#rdma-backend) for configuration details. | v0.31.0<br />LMCache: v0.5.5 |

#### Communication library support matrix

| Communication library | RDMA flow observable | Information as of version |
| --- | --- | --- |
| NCCL | Observability depends on the actual RDMA submission path:<br />- With the [NCCL API](https://github.com/NVIDIA/nccl/blob/v2.32.3-1/src/nccl.h.in#L602), the CPU proxy thread submits RDMA requests through `libibverbs`; supported.<br />- With the [NCCL-GIN API](https://github.com/NVIDIA/nccl/blob/v2.32.3-1/src/include/nccl_device/gin.h#L356), when the `NCCL_GIN_TYPE` environment variable is left unset (the default) or explicitly set to `NCCL_GIN_TYPE=3` (GDAKI), direct GPU control of the NIC is preferred, bypassing `libibverbs`; unsupported.<br />- With the [NCCL-GIN API](https://github.com/NVIDIA/nccl/blob/v2.32.3-1/src/include/nccl_device/gin.h#L356), setting the environment variable `NCCL_GIN_TYPE=2` selects Proxy mode, where the CPU submits RDMA requests; supported. | v2.32.3 |
| UCX | Select transport paths through the `UCX_TLS` environment variable:<br />- `rc`, `dc` paths: supported.<br />- Other transport paths: RDMA observability is unsupported. | v1.22.0 |
| DeepEP | Unsupported. DeepEP submits RDMA requests directly from the GPU. | - |
| FlashInfer | Unsupported. FlashInfer initiates RDMA from the GPU; the CPU is not on the critical path. | - |
| LIBFABRIC | Supported. | v2.7.0 |

## Requirements

- Linux nodes with BPF ring buffer and uprobe support, kernel 5.8 or later.
- Applications must use the host `libibverbs` path. Merely using RDMA or GPU
  buffers does not make a workload observable.

## Enable

### Basic functionality

The basic functionality periodically aggregates RDMA flows from BPF maps and
exports Prometheus metrics through the Agent metrics Service, including
cumulative byte and WR counts and the number of active QPs for each flow. It does not produce
per-send events and does not require an event storage backend.

```bash
helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set agent.ebpfFlow.enabled=true \
  --wait
```

On InfiniBand fabrics, also set
`--set agent.ebpfFlow.endpoint.allPods=true`. Because IB peers are addressed
by LID, information about every RDMA Pod must be synchronized to an
`RDMAEndpoint` custom resource.

All settings and their defaults are listed under `agent.ebpfFlow` in the
[Helm values reference](../../chart/README.md). They map one to one onto the
`ebpfFlow` section of the Agent config file.

#### Verify

Open Grafana and view the Unifabric RDMA Pods and Nodes dashboards. Verify that
the panels under RDMA send details (CPU initiated) display data.

![Unifabric RDMA Pods dashboard](../images/pods.png)

![Unifabric RDMA Nodes dashboard](../images/nodes.png)

### Detailed events

The basic functionality exports pre-aggregated Prometheus metrics. Detailed
events instead capture every RDMA `post_send` as an event record containing
the source and destination Pods, QP, network interfaces, bytes sent, WR count,
and timestamp.
The Agent sends these records over OTLP to the Event Collector, which writes
them to an event storage backend.

`flowEvents` is disabled by default. Enabling detailed events requires at
least one storage backend. The chart currently supports **ClickHouse** and
**Elasticsearch**; either one or both can be enabled. Kafka is not currently
supported.

#### Bundled ClickHouse

The following command enables detailed events, the Event Collector, and the
bundled ClickHouse instance:

```bash
helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set agent.ebpfFlow.enabled=true \
  --set agent.ebpfFlow.flowEvents.enabled=true \
  --set ebpfFlowEventCollector.clickhouse.enabled=true \
  --wait
```

When `clickhouse.endpoint` is empty, the chart deploys a bundled single-replica
ClickHouse. The default user is `rdma`, the default password is `rdma-flow`,
and records are stored in `rdma.rdma_sends`. By default, it uses a `20Gi` PVC
with the cluster's default StorageClass.

This instance has no replication or backups and is intended only for testing
and evaluation.

#### Bundled Elasticsearch

```bash
helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set agent.ebpfFlow.enabled=true \
  --set agent.ebpfFlow.flowEvents.enabled=true \
  --set ebpfFlowEventCollector.elasticsearch.enabled=true \
  --wait
```

When `elasticsearch.endpoints` is empty, the chart deploys a bundled
single-node Elasticsearch. The default user is `elastic`, the default password
is `rdma-flow`, and records are written to the `rdma-sends` data stream.

Data is stored in an `emptyDir` by default and is lost when the Pod is
recreated. This instance is intended only for testing and evaluation.

#### External ClickHouse

Provide a native protocol endpoint and either a password or a Secret containing
a `password` key:

```bash
kubectl -n unifabric-system create secret generic clickhouse-credentials \
  --from-literal=password='<clickhouse-password>'
```

Add the backend configuration to a values file:

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

`clickhouse.password` can be set directly, but a Secret is recommended for
production. Setting `endpoint` prevents deployment of the bundled ClickHouse.

#### External Elasticsearch

First create a Secret containing a `password` key:

```bash
kubectl -n unifabric-system create secret generic elasticsearch-credentials \
  --from-literal=password='<elasticsearch-password>'
```

Add the backend configuration to a values file:

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

`elasticsearch.password` can be set directly. Setting `endpoints` prevents
deployment of the bundled Elasticsearch.

When both backends are enabled, they receive the same event records; one
being unavailable does not affect the other.

Save the external backend configuration as `rdma-flow-events.yaml`, then run:

```bash
helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  -f rdma-flow-events.yaml \
  --wait
```

#### Event settings and verification

- The Agent automatically connects to the Event Collector Service deployed by
  the chart; no OTLP endpoint configuration is required.
- `flowEvents.aggregate` merges identical submissions on one QP within the
  window into one event record with a `count`; `flowEvents.sample` retains one
  event in every N.
- Elasticsearch generally uses more disk and is useful for correlation with
  logs; ClickHouse is better suited to aggregate analysis.

Use Prometheus to verify the event collection rate and message-size
distribution:

```promql
sum by (src_pod_name, dst_pod_name) (rate(rdma_flow_sends_total[1m]))
histogram_quantile(0.5, sum by (le) (rate(rdma_flow_send_bytes_bucket[5m])))
```

Example ClickHouse query:

```sql
SELECT src_pod_name, dst_pod_name, sum(count), sum(bytes * count),
       quantileWeighted(0.5)(bytes, count)
FROM rdma.rdma_sends
WHERE Timestamp > now() - INTERVAL 5 MINUTE
GROUP BY 1, 2 ORDER BY 3 DESC
```

## Owner resolution for other workload controllers

Flow labels include the top-level owner of each Pod. The Agent follows
`ownerReferences` only through resource types it is allowed to read. For
controllers such as Argo Workflows, read permissions must be added before
the Agent can resolve a Pod's owner to the Workflow rather than an
intermediate resource.

Permission rules for third-party resource types are defined in values as
`agent.workloadOwnerRules`, a map keyed by API group whose values are lists
of plural resource names. User-provided values merge with the defaults;
to add a controller permission rule, add the corresponding API group and
resource names:

```yaml
agent:
  workloadOwnerRules:
    argoproj.io: [workflows]
```

The chart renders these rules into the `<release>-agent-workload-owners`
ClusterRole. The resource types followed by the flow resolver are listed in
`pkg/agent/ebpfflow/owners.go`; adding an API group also requires a
corresponding entry in that file.

## Troubleshooting

### No data in the Pods Grafana dashboard

The commands below assume the installation namespace is `unifabric-system`.
If you installed the chart in another namespace, adjust `NAMESPACE` accordingly.

1. First verify that the application meets the observability prerequisites:

   - The communication library actually uses a CPU-initiated host
     `libibverbs` path. Refer to the support information above and confirm the
     path in NCCL, UCX, or application logs. GPU-initiated paths are not
     observable.
   - The application actually produces RDMA traffic during troubleshooting.
     Confirm traffic through application logs, communication test results, or
     RDMA device counters.

   If either condition is false, no flow data will be produced, so there is no
   reason to continue checking the Agent or Grafana.

2. Verify that the Agent is running on every target node, then select an Agent
   Pod on a node that has RDMA traffic:

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

   The DaemonSet `DESIRED`, `CURRENT`, and `READY` values should match. Also
   verify that `agent.ebpfFlow.enabled=true`; when disabled, the Agent does not
   register `rdma_flow_*` metrics.

3. Inspect metrics directly from that Agent to distinguish collection issues
   from Grafana or Prometheus issues. In one terminal, start port forwarding:

   ```bash
   kubectl -n "$NAMESPACE" port-forward "pod/$AGENT_POD" 8082:8082
   ```

   In another terminal, run:

   ```bash
   curl -s http://127.0.0.1:8082/metrics | grep '^rdma_flow_'
   curl -s http://127.0.0.1:8082/metrics | \
     grep -E '^rdma_flow_agent_(qp_infos|qp_stats|qp_matched|qp_unmatched|qp_unresolved) '
   ```

   - No `rdma_flow_*` metrics: verify that the feature is enabled, then inspect
     the BPF loading logs in the next step.
   - `rdma_flow_agent_qp_stats` is `0`: the Agent has not observed any sends.
     Verify that the application is producing CPU-initiated RDMA traffic and
     that the data plane probes are attached.
   - `rdma_flow_agent_qp_stats` is greater than `0`, but `qp_matched` is `0`:
     inspect `qp_infos` and the provider callback.
   - `rdma_flow_agent_qp_unresolved` is greater than `0`: the Agent found QPs
     but cannot resolve the source or destination to a Pod. On InfiniBand,
     verify `agent.ebpfFlow.endpoint.allPods=true` and check that all required
     `RDMAEndpoint` objects exist.
   - `rdma_flow_bytes_total`, `rdma_flow_wrs_total`, or `rdma_flow_qps` has
     data: Agent collection works; continue with the Prometheus and Grafana
     checks.

4. Check the BPF object, probes, and pinned maps:

   ```bash
   kubectl -n "$NAMESPACE" logs "$AGENT_POD" -c agent | \
     grep -E 'bpf_collection_loaded|bootstrap_probes_attached|provider_callback_restored|callback_received|load bpf collection|attach_bootstrap_probes'

   kubectl -n "$NAMESPACE" exec "$AGENT_POD" -c agent -- \
     ls -l /sys/fs/bpf/unifabric
   ```

   The logs should contain `bpf_collection_loaded` and
   `bootstrap_probes_attached`. After the workload creates a QP,
   `provider_callback_restored` should appear; `callback_received` is emitted
   only at debug log level. `/sys/fs/bpf/unifabric` should contain `qp_infos`
   and `qp_stats`.

   If `bpftool` is installed on the node, inspect the maps:

   ```bash
   bpftool map dump pinned /sys/fs/bpf/unifabric/qp_infos
   bpftool map dump pinned /sys/fs/bpf/unifabric/qp_stats
   ```

   An empty `qp_infos` usually means that no QP has reached RTR or the provider
   callback was not captured. An empty `qp_stats` usually means that the
   application did not submit sends through a supported `libibverbs` path.

5. If the Agent exposes flow metrics locally, inspect the Prometheus scrape
   path:

   ```bash
   kubectl -n "$NAMESPACE" get service,servicemonitor \
     -l app.kubernetes.io/component=unifabric-agent
   ```

   By default, the chart creates an Agent metrics Service and ServiceMonitor.
   Verify that the corresponding target is `UP` on the Prometheus Targets page.
   If Prometheus selects ServiceMonitors by label, ensure that
   `nodeMetrics.serviceMonitor.labels` matches its selector. Then run this query
   in Prometheus or Grafana Explore:

   ```promql
   sum(rate(rdma_flow_bytes_total[5m]))
   ```

   If the query returns data but the dashboard is empty, verify the dashboard
   time range and ensure that the namespace, workload, and Pod variables select
   the objects that are generating RDMA traffic.
