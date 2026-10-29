# RDMA Send Event Storage

> Maturity: Alpha

中文版: [rdma-flow-events.zh.md](./rdma-flow-events.zh.md)

## Introduction

[eBPF-based RDMA tracing](./rdma-flow.md) exports flow metrics that are already summed.
RDMA send event storage extends it: the Agent writes each RDMA send of the application as one event and stores it in a database.
Operators use these events to analyze the timing, the peers, and the message sizes of Pod communication.
This document tells you how to choose a storage backend, and how to configure, verify, and troubleshoot send event storage.

| Item | Description |
| --- | --- |
| Applicable networks | Scale-out, Storage |
| Data source | The probes of eBPF-based RDMA tracing, which put each send in a BPF ring buffer |
| Observed object | Each CPU-initiated RDMA send |
| Output | Events in a ClickHouse table or an Elasticsearch data stream, and the `rdma_flow_sends_total` and `rdma_flow_send_bytes` metrics |
| Cluster side requirements | eBPF-based RDMA tracing is enabled. For the bundled ClickHouse, the cluster has a default StorageClass. |
| Default state | Disabled by default |

The data goes through this path:

1. The Agent reads each send from the BPF ring buffer, and resolves the source Pod and the destination Pod.
2. The Agent sends the events to the Event Collector through OTLP gRPC. The Event Collector is an OpenTelemetry Collector that the chart deploys. Its Service is `unifabric-ebpf-flow-event-collector`.
3. The Event Collector writes the events to ClickHouse, to Elasticsearch, or to both.

For the fields of an event, see the [metrics reference](../../reference/metrics.md#rdma-send-event-fields).

Limits:

- The bundled ClickHouse and Elasticsearch are only for tests and evaluation. They have one replica, no backups, and fixed credentials.
- The Agent can send events only to the Event Collector that the chart deploys. You cannot configure an external Collector.
- The chart supports only ClickHouse and Elasticsearch. It does not support Kafka.
- Applications that send often make many events. You can use sampling and merging to decrease the number of events. See `agent.ebpfFlow.flowEvents.*` in the [Helm values reference](../../../chart/README.md).

This item is part of AI high-speed network traffic observability. For the full scope, see [AI high-speed network traffic observability](./README.md#traffic-observability-scope).

## Prerequisites

- You enabled `agent.ebpfFlow.enabled=true` as described in [eBPF-based RDMA tracing](./rdma-flow.md#configuration).
- For an external backend, ClickHouse or Elasticsearch is ready, and the Event Collector can connect to it.
- For the bundled ClickHouse, the cluster has a default StorageClass, or you set `ebpfFlowEventCollector.clickhouse.bundled.persistence.storageClassName`.

## Choose a storage backend

You can use one backend, or both together.
When both are enabled, they get the same events. If one backend is not available, the other is not affected.

| Item | ClickHouse | Elasticsearch |
| --- | --- | --- |
| Use it for | Aggregate analysis with SQL | Correlation with logs, and viewing send records in Grafana |
| Disk usage | Smaller | Larger |
| Grafana dashboards | The send record panels in the dashboards do not query ClickHouse | The message size, process and QP, and latest send record panels in the Pod and AI Workload dashboards query Elasticsearch |
| Data retention | 3 days by default | 3 days by default |
| Storage of the bundled instance | PVC, `20Gi` by default | `emptyDir`. The data is lost when the Pod is created again |

If you do not set an external endpoint, the chart deploys a bundled instance.
If you set an external endpoint, the chart does not deploy a bundled instance.

For the data retention, and the resources and storage of the bundled instances, see `ebpfFlowEventCollector.*` in the [Helm values reference](../../../chart/README.md).

## Configuration

### Case 1: Use the bundled ClickHouse

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set agent.ebpfFlow.enabled=true \
  --set agent.ebpfFlow.flowEvents.enabled=true \
  --set ebpfFlowEventCollector.clickhouse.enabled=true \
  --wait
```

The chart deploys a single-replica ClickHouse, `unifabric-ebpf-flow-clickhouse`:

- The user is `rdma`, and the password is `rdma-flow`.
- The events go to the `rdma.rdma_sends` table.
- The data is on a `20Gi` PVC with the default StorageClass of the cluster.

The chart also installs the "Unifabric / ClickHouse Server" dashboard, which shows the state of the bundled ClickHouse.

### Case 2: Use the bundled Elasticsearch

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set agent.ebpfFlow.enabled=true \
  --set agent.ebpfFlow.flowEvents.enabled=true \
  --set ebpfFlowEventCollector.elasticsearch.enabled=true \
  --wait
```

The chart deploys a single-node Elasticsearch, `unifabric-elasticsearch`:

- The user is `elastic`, and the password is `rdma-flow`.
- The events go to the `rdma-sends` data stream.
- By default, the data is in an `emptyDir` and is lost when the Pod is created again. To use a PVC, set `ebpfFlowEventCollector.elasticsearch.bundled.persistence.enabled=true`.

The chart also installs the "Unifabric / Elasticsearch Server" dashboard, which shows the state of the bundled Elasticsearch.

### Case 3: Use an external ClickHouse

1. Create a Secret with a `password` key:

   ```bash
   kubectl -n unifabric-system create secret generic clickhouse-credentials \
     --from-literal=password='<clickhouse-password>'
   ```

2. Write the backend configuration to `rdma-flow-events.yaml`. For `endpoint`, use the ClickHouse native protocol endpoint:

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

3. Apply the configuration:

   ```bash
   helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
     --namespace unifabric-system \
     --reuse-values \
     -f rdma-flow-events.yaml \
     --wait
   ```

Before the Event Collector starts, an init container creates the database and the table in ClickHouse.
The `rdma` user needs permission to create databases and tables.
If you manage the schema yourself, set `ebpfFlowEventCollector.clickhouse.schema.create=false`.

> Note: You can also set `ebpfFlowEventCollector.clickhouse.password` directly. For production, use a Secret.

### Case 4: Use an external Elasticsearch

1. Create a Secret with a `password` key:

   ```bash
   kubectl -n unifabric-system create secret generic elasticsearch-credentials \
     --from-literal=password='<elasticsearch-password>'
   ```

2. Write the backend configuration to `rdma-flow-events.yaml`:

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

3. Apply the configuration:

   ```bash
   helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
     --namespace unifabric-system \
     --reuse-values \
     -f rdma-flow-events.yaml \
     --wait
   ```

Before the Event Collector starts, an init container writes the index template and the ILM policy of the data stream.
If you manage the index template yourself, set `ebpfFlowEventCollector.elasticsearch.schema.create=false`.

> Note: The send record panels in the Pod and AI Workload dashboards query Elasticsearch through a `GrafanaDatasource`.
> The chart creates this data source only when `grafanaDashboard.kind=GrafanaDashboard` (default).
> If you install the dashboards as `ConfigMap`, create an Elasticsearch data source with the uid `unifabric-elasticsearch` in Grafana manually.

## Verification

1. Make sure that the Event Collector and the bundled backends run:

   ```bash
   kubectl -n unifabric-system get deployment \
     unifabric-ebpf-flow-event-collector \
     unifabric-ebpf-flow-clickhouse \
     unifabric-elasticsearch
   ```

   If you enabled only one bundled backend, the other Deployment does not exist.

2. After the application makes RDMA traffic, make sure in Prometheus or Grafana Explore that the Agent connected to the Event Collector and wrote events:

   ```promql
   rdma_flow_agent_send_sink_connected
   sum(rate(rdma_flow_agent_send_rows_inserted_total[5m]))
   ```

   `rdma_flow_agent_send_sink_connected` must be `1`.

3. View the send counts and the message sizes:

   ```promql
   sum by (src_pod_name, dst_pod_name) (rate(rdma_flow_sends_total[1m]))
   histogram_quantile(0.5, sum by (le) (rate(rdma_flow_send_bytes_bucket[5m])))
   ```

4. If you use ClickHouse, query the events of the last 5 minutes:

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

   <!-- TODO: capture output from a real environment -->

   For an external ClickHouse, run the same SQL with your own client and credentials.

5. If you use Elasticsearch, make sure that the data stream exists:

   ```bash
   kubectl -n unifabric-system exec deploy/unifabric-elasticsearch -c elasticsearch -- \
     curl -s -u elastic:rdma-flow 'http://localhost:9200/_data_stream/rdma-sends'
   ```

   <!-- TODO: capture output from a real environment -->

   Then open the Unifabric RDMA Pod dashboard in Grafana, and make sure that the "Latest send records" panel has data.

## Troubleshooting

### The backend has no events

1. Make sure that the Agent reads send events. After you start a port forward to an Agent Pod, run:

   ```bash
   curl -s http://127.0.0.1:8082/metrics | \
     grep -E '^rdma_flow_agent_send_(events_received|events_sampled|events_dropped|sink_connected|queue_length|rows_inserted_total|rows_failed_total)'
   ```

   - `rdma_flow_agent_send_events_received` is `0`: the Agent did not get send events. Check the probes as described in [eBPF-based RDMA tracing troubleshooting](./rdma-flow.md#troubleshooting).
   - `rdma_flow_agent_send_sink_connected` is `0`: the Agent did not connect to the Event Collector. Do the next step.
   - `rdma_flow_agent_send_rows_failed_total` keeps increasing: the Event Collector refused the writes. Do the next step.

2. Read the logs of the Event Collector:

   ```bash
   kubectl -n unifabric-system logs deploy/unifabric-ebpf-flow-event-collector --tail 200
   ```

   Exporter errors in the logs mean that the Event Collector cannot write to the backend. Check the backend address, the credentials, and the network connection.

3. For an external backend, read the logs of the init container that creates the schema. The ClickHouse init container is `clickhouse-schema`. The Elasticsearch init container is `elasticsearch-schema`:

   ```bash
   kubectl -n unifabric-system logs deploy/unifabric-ebpf-flow-event-collector -c clickhouse-schema
   kubectl -n unifabric-system logs deploy/unifabric-ebpf-flow-event-collector -c elasticsearch-schema
   ```

   If the user cannot create tables, ask the database administrator to create the schema first, and set `schema.create=false`.

### Events are dropped

If `rdma_flow_agent_send_events_dropped` keeps increasing, events are made faster than they are written, or the connection between the Agent and the Event Collector is broken.

- Use sampling or merging to decrease the number of events, or increase the event queue of the Agent to absorb bursts. See `agent.ebpfFlow.flowEvents.*` in the [Helm values reference](../../../chart/README.md).
- Increase the resources of the Event Collector, and check the write performance of the backend. See `ebpfFlowEventCollector.resources` in the [Helm values reference](../../../chart/README.md).

### The send record panels in the dashboards have no data

1. Make sure that the Elasticsearch backend is enabled. These panels do not show events in ClickHouse.
2. Make sure that `ebpfFlowEventCollector.elasticsearch.grafanaDatasource.enabled=true` (default) and `grafanaDashboard.kind=GrafanaDashboard`.
3. Make sure that the `GrafanaDatasource` exists:

   ```bash
   kubectl -n unifabric-system get grafanadatasource unifabric-ebpf-flow-event-collector-elasticsearch
   ```

## References

- [AI high-speed network traffic observability](./README.md): the full scope.
- [eBPF-based RDMA tracing](./rdma-flow.md): the prerequisite of send event storage.
- [Metrics reference](../../reference/metrics.md#rdma-send-event-fields): the fields of an event.
- [eBPF RDMA flow attribution design](../../design/rdma-flow.md#6-send-event-pipeline): the design of the send event pipeline.
- [Helm values reference](../../../chart/README.md): all `agent.ebpfFlow.flowEvents.*` and `ebpfFlowEventCollector.*` parameters.
