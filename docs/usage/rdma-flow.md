# eBPF RDMA Flow Attribution

中文版：[rdma-flow.zh.md](./rdma-flow.zh.md)

This guide enables the `ebpfFlow` component of the Agent, verifies that
Pod-to-Pod RDMA flows are attributed, optionally stores every send event
through an OpenTelemetry Collector and explains how to troubleshoot missing
attribution. The design is described in
[docs/design/rdma-flow.md](../design/rdma-flow.md) and the resource it
publishes in [docs/reference/rdmaendpoint.md](../reference/rdmaendpoint.md).

## Requirements

- Linux nodes with BPF ring buffer and uprobe support, kernel 5.8 or later.
- Workloads using `libibverbs`. Traffic from libraries that bypass it is not
  observed.
- The Agent DaemonSet already runs privileged with `hostPID`. Enabling the
  component adds host mounts for `/sys/fs/bpf` and `/var/lib/unifabric`.

## Enable the component

```yaml
agent:
  ebpfFlow:
    enabled: true
```

Apply with `helm upgrade`. Nothing else changes for releases that keep the
switch off. On InfiniBand fabrics also set
`agent.ebpfFlow.endpoint.allPods: true`, because peers are addressed by
LID and every RDMA Pod must publish an `RDMAEndpoint`.

All settings and their defaults are listed under `agent.ebpfFlow` in the
[Helm values reference](../../chart/README.md). They map one to one onto the
`ebpfFlow` section of the Agent config file.

For cumulative flow statistics without per-send ring-buffer events or a
storage pipeline:

```yaml
agent:
  ebpfFlow:
    enabled: true
    flowEvents:
      enabled: false
```

This keeps byte, WR and active-QP flow metrics. It omits send-count and
send-size metrics, OTLP rows and their ring-buffer/queue overhead. Keep
`flowEvents.enabled: true` (the default) when exact send events are required.

## Verify

Check that the component started on every Agent:

```bash
kubectl -n <ns> logs ds/<release>-agent -c agent | grep -E 'startup_complete|bpf_collection_loaded|bootstrap_probes_attached'
```

Run RDMA traffic between two Pods, for example `ib_write_bw` from
`perftest` in two privileged hostNetwork Pods on different nodes, then:

```bash
kubectl get rep -A
kubectl get rep -n <ns> <pod> -o yaml
```

Each sending or receiving Pod that holds an RDMA device has an object, and
the port that carries an established connection lists its `queuePairs`.

Query the metrics through the Agent metrics Service or Prometheus:

```promql
sum by (src_pod_name, dst_pod_name) (rate(rdma_flow_bytes_total[1m]))
histogram_quantile(0.5, sum by (le) (rate(rdma_flow_send_bytes_bucket[5m])))
rdma_flow_agent_qp_unresolved
```

The first shows bytes per second per flow, the second the median message
size, the third counts queue pairs whose ends could not both be resolved to a
Pod and should return to zero once indexes catch up.

The `Unifabric / RDMA Flow` Grafana dashboard renders with the component
when `grafanaDashboard.enabled` is true, in the language selected by
`grafanaDashboard.language`. It selects a workload and a Pod and
shows the traffic to each peer, message size buckets, Agent health and the
resource use of the Agent and of the storage backends.

## Store send events

Flow metrics are pre-aggregated. To keep every submission with its Pod
attribution, deploy the optional Collector and one or two backends. With your
own ClickHouse or Elasticsearch:

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

For an evaluation without existing backends, leave `endpoint` and
`endpoints` empty and the chart deploys bundled single replica instances
next to the Collector:

```yaml
ebpfFlowEventCollector:
  enabled: true
  clickhouse:
    enabled: true
  elasticsearch:
    enabled: true
```

The bundled instances are for testing only. They run one replica without
replication or backups, use the fixed password `rdma-flow` unless
`password` is set, and the bundled Elasticsearch keeps its data on an
`emptyDir` by default. The bundled ClickHouse uses a 20Gi
PersistentVolumeClaim from the default StorageClass, `bundled.persistence`
selects the class, the size, an existing claim or an `emptyDir`. Setting
`endpoint` or `endpoints` removes the bundled instance on the next upgrade,
so point production traffic at a managed cluster.

- The Agents send OTLP log records to the Collector Service automatically
  when `agent.ebpfFlow.otlp.endpoint` is empty.
- `flowEvents.aggregate` merges identical submissions of a queue pair inside the
  window into one row with a `count`. LLM traffic repeats a few message
  sizes, so a 100 ms window usually reduces rows by one to two orders of
  magnitude. `flowEvents.sample` keeps one event in N when detail can be sampled.
- With ClickHouse the Collector runs an init container that creates the
  database and a table with typed columns for every attribute, TTL
  `clickhouse.ttlDays`. The exporter never creates the schema itself. Column
  additions are made in the schema of `chart/templates/EbpfFlowEventCollector.yaml`
  as `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`.
- With Elasticsearch the documents go to the `rdma-sends` data stream in
  `ecs` mapping mode. `elasticsearch.schema.create` writes the index
  template and ILM policy before the Collector starts: keyword fields for
  every string attribute, `date_nanos` timestamps, dynamic mapping off, one
  shard without replicas, rollover after `schema.ilm.rolloverMaxAge` or
  `rolloverMaxPrimaryShardSize` and deletion after `deleteMinAge`. The
  bundled instance reapplies the template on every start because it is
  cluster state. A new attribute needs a field in the template of
  `chart/templates/EbpfFlowEventCollector.yaml` followed by a `_rollover`. A
  `GrafanaDatasource` for the data stream is created when
  `grafanaDashboard.kind` is `GrafanaDashboard`, and the
  `Unifabric / RDMA Flow Sends` dashboard queries it for exact message size
  percentiles, the most common sizes, per process queue pair counts and raw
  records.
- Existing secrets need a `password` key. Without `existingSecret` the chart
  writes the `password` values into a Secret of its own.

Both backends receive the same records, one being unavailable does not
affect the other. Elasticsearch typically uses 5 to 10 times the disk of
ClickHouse for this data and suits correlation with logs rather than
aggregate analysis.

Example ClickHouse query:

```sql
SELECT src_pod_name, dst_pod_name, sum(count), sum(bytes * count),
       quantileWeighted(0.5)(bytes, count)
FROM rdma.rdma_sends
WHERE Timestamp > now() - INTERVAL 5 MINUTE
GROUP BY 1, 2 ORDER BY 3 DESC
```

## Owner resolution for other workload controllers

Flow labels carry the top-level owner of each Pod. The agent follows a
Pod's `ownerReferences` only through kinds it is allowed to read, so a
controller such as Argo Workflows needs a read rule before its Pods are
labelled with the workflow instead of the intermediate resource. Rules for
third-party kinds are defined in values as `agent.workloadOwnerRules`, a
map keyed by API group with the plural resources as value. User values
merge with the defaults, so adding a controller is one extra key:

```yaml
agent:
  workloadOwnerRules:
    argoproj.io: [workflows]
```

The chart renders the map into the `<release>-agent-workload-owners`
ClusterRole. The
kinds the flow resolver follows are listed in
`pkg/agent/ebpfflow/owners.go`, a new group also needs an entry there.

## Troubleshooting

Agent logs are structured, filter on the `msg` field.

| Symptom | What to check |
| --- | --- |
| No `rdma_flow_*` series at all | `bpf_collection_loaded` and `bootstrap_probes_attached` in the Agent log. `load bpf collection` errors mean the kernel lacks BPF features or the BPF object is missing from the image. `attach_bootstrap_probes` errors name the library that could not be probed. |
| `rdma_flow_agent_qp_stats` is zero while traffic flows | No data plane probes yet. `provider_callback_restored` or `callback_received` should appear once the workload creates a queue pair, otherwise the provider is not a supported `libibverbs` provider or the workload bypasses `libibverbs`. |
| `rdma_flow_agent_qp_unresolved` stays high | `qp_stats` debug records show `source` and `destination`. Empty destinations addressed to a node IP or a LID need an `RDMAEndpoint` on the receiving node, check `kubectl get rep -A` and `rdma_endpoint_refresh_complete` in the sending Agent. Destinations with a Pod IP need the Pod IP index, check `pod_index_refresh_complete` and `get_pod_list` errors. |
| An `RDMAEndpoint` is missing | The Pod must hold `/dev/infiniband/uverbsN` open. Pods with their own network namespace are only published with `endpoint.allPods`. `rdma_endpoint_deleted reason=no_rdma_holder` means the last RDMA process of the Pod exited. |
| `RDMAEndpoint` has no `queuePairs` | The queue pair has not reached RTR yet, for example a server waiting for its client. |
| `rdma_flow_agent_send_events_dropped{sink="otlp"}` grows | The Collector is unreachable or slower than the event rate. Check `rdma_flow_agent_send_sink_connected`, the Collector `otelcol_exporter_send_failed_log_records` metric and raise `sends.aggregate` or `sends.sample`. |
| Counters reset after an Agent restart | Pinned maps were incompatible with the new version and recreated, logged as `reuse_pinned_maps status=layout_incompatible`. Prometheus `rate()` handles the reset. |

Inspect the pinned maps and the offset cache on a node with

```bash
ls /sys/fs/bpf/unifabric
cat /var/lib/unifabric/ebpf-flow/callback-offsets.json
```

Deleting both while the Agent is stopped forces a clean rediscovery.
