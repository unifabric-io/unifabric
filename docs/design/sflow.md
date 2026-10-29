# sFlow Processing

## Overview

This document describes the Unifabric sFlow processing design. Switches push
sFlow v5 datagrams to an independent `sflow` collector. The collector decodes
sampled flow records, matches source and destination IP addresses against the
current Kubernetes Pod inventory, fills Pod, Node, and top-level workload owner
fields, and exports the result as OTLP log records to the in-release
OpenTelemetry Collector, which writes them into the Elasticsearch
`unifabric-switch-sflow` data stream.
`chart/files/dashboard-en/switch-sflow.json`,
`chart/files/dashboard-en/workload-sflow.json`, and
`chart/files/dashboard-en/workload-pair-sflow.json` query this data stream
through the `unifabric-sflow-elasticsearch` Grafana datasource.

The collector is deployed separately from the existing Controller and Agent. It
is enabled only when `sflow.enabled=true`, exposes one UDP port for switches,
and does not change CRDs, Node labels, or the existing RDMA metrics collection
path. The current design focuses on one path: receive switch sFlow, fill
Kubernetes fields, export OTLP log records, and make those fields available to
the dashboards.

## Motivation

RDMA metrics can show NIC, port, and RDMA counters from the Node and Pod point
of view, but they do not directly answer how traffic looks from the switch side.
For example, an operator may need to know which workloads produced the most
traffic on a switch, or which switches carried traffic for a selected workload.
Switch-pushed sFlow data provides that perspective.

Raw sFlow records contain sampled network-layer and transport-layer information,
but they do not contain Kubernetes fields. If raw records are written directly
to Elasticsearch, dashboards can aggregate by IP, port, or protocol, but not by
Pod, Node, or workload owner. The collector therefore needs to connect endpoint
IPs to the current Kubernetes Pod inventory before exporting records.

The scope of this path is intentionally narrow. The collector only fills fields
for current sampled flow records and exports them; it does not rewrite
historical documents and does not try to build a topology model for all network
paths. When an endpoint IP does not match a Pod, Kubernetes fields stay empty
and dashboards can still show the base traffic data.

### Goals

- Expose one UDP port that receives sFlow v5 datagrams pushed by one or more
  switches.
- Decode supported sFlow flow samples into normalized flow records that can be
  exported over OTLP.
- Use the current Pod IP cache to fill Pod namespace, Pod name, Node name, and
  top-level workload owner fields for the source and destination sides.
- Export records with Kubernetes fields as OTLP log records in batches; the
  shared OpenTelemetry Collector writes them into the Elasticsearch
  `unifabric-switch-sflow` data stream.
- Expose health checks and Prometheus metrics for receive, decode, export, and
  overload status.
- Package the collector, UDP Service, ConfigMap, and RBAC through the Helm
  chart with disabled-by-default values.

### Non-Goals

- Collector high availability, sharded ingest, and cross-instance deduplication
  are out of scope for this phase.
- Historical flow documents are not rewritten; later Pod or owner changes do
  not modify documents already written to Elasticsearch.
- Existing Controller, Agent, CRDs, Node labels, and RDMA metrics semantics are
  not changed.
- The reference `gosflow/` directory is not committed or imported as production
  code.

## Proposal

The design has four parts.

1. `cmd/sflow` starts an independent process, loads config, creates the
   Kubernetes client, health checks, metrics endpoint, Pod cache updater, and
   UDP collector.
2. `pkg/sflow` decodes sFlow datagrams, extracts records from IPv4, IPv6, and
   sampled Ethernet headers, and converts them into the shared `FlowRecord`
   shape.
3. The collector periodically refreshes the Pod IP cache and uses `src_addr`
   and `dst_addr` to match Pod / Workload information before exporting records.
4. The writer batches records and exports them over OTLP gRPC to the in-release
   OpenTelemetry Collector (`ebpfFlowEventCollector`), which writes them into
   the Elasticsearch `unifabric-switch-sflow` data stream; dashboards query that data
   stream directly.

The collector uses a bounded queue between UDP receive and OTLP exports. When
exports slow down, the queue can absorb short bursts. When the queue is full,
the collector drops new normalized records and increments overload metrics,
which prevents unbounded memory growth.

### User Story

#### Story 1: View Kubernetes Workloads for Switch sFlow

As a cluster operator, I want Unifabric to receive switch-pushed sFlow data and
fill Pod and workload fields before exporting records. This lets me use the
switch and workload dashboards to inspect switch-observed traffic by Kubernetes
object without manually maintaining IP-to-Pod mappings.

### Constraints and Notes

- The collector is disabled by default. Deployment, Service, ConfigMap, and RBAC
  are rendered only when `sflow.enabled=true`.
- sFlow input uses UDP. The collector does not open reverse connections to
  switches.
- OTLP export targets the sFlow gRPC port of the in-release
  `ebpfFlowEventCollector` by default. `sflow.otel.endpoint` can point at an
  external Collector instead.
- When `sflow.otel.endpoint` is empty, `sflow.enabled=true` requires
  `ebpfFlowEventCollector.elasticsearch.enabled=true`; the chart fails the
  render otherwise.
- Pod / Workload fields come from the Pod cache visible to the collector and may
  briefly lag the live cluster state.
- Endpoints that do not match Pod IPs keep empty Kubernetes fields and do not
  block record exports.
- The `unifabric-switch-sflow` index template and ILM policy are applied by the
  Collector chart before ingestion. Dynamic mapping is disabled.
- Dashboards depend on fixed attribute fields. A new attribute needs a field in
  the index template followed by a data stream rollover.

### Risks and Mitigations

- Risk: slow OTLP exports may cause too many records to accumulate behind UDP
  receive. Mitigation: use a bounded writer queue, drop new records when the
  queue is full, and expose overload through
  `unifabric_sflow_records_dropped_total` and logs.
- Risk: the Pod cache may briefly differ from live Pod state, so some records
  may miss Pod or owner fields. Mitigation: refresh the cache periodically and
  keep unmatched fields empty without blocking base traffic records.
- Risk: owner lookup may fail or a related CRD may be absent. Mitigation:
  preserve matched Pod fields, leave owner fields empty, and keep the collector
  running.
- Risk: dashboards may drift from the Elasticsearch index template. Mitigation:
  verify that `switch-sflow.json`, `workload-sflow.json`, and
  `workload-pair-sflow.json` only query fields mapped by the index template.
- Risk: reference code under `gosflow/` may be committed accidentally or enter
  the image build context. Mitigation: ignore it in `.gitignore` and
  `.dockerignore`, and verify production code does not import it.

## Design Details

### Architecture

```mermaid
flowchart LR
  SW[Switch<br/>sFlow exporter]
  SVC[sFlow UDP Service]
  C[sFlow Collector<br/>cmd/sflow]
  P[Pod IP Cache<br/>Pods and owners]
  Q[Bounded writer queue]
  OC[OpenTelemetry Collector<br/>ebpfFlowEventCollector]
  ES[Elasticsearch<br/>unifabric-switch-sflow data stream]
  G[Grafana Dashboards<br/>switch/workload sFlow]

  SW -->|UDP sFlow v5| SVC
  SVC --> C
  P -->|IP match and Kubernetes fields| C
  C -->|normalized records| Q
  Q -->|OTLP gRPC log records| OC
  OC -->|elasticsearch exporter| ES
  G -->|Elasticsearch queries| ES
```

### Components

- `cmd/sflow`: process entrypoint, config loading, manager startup, health
  checks, metrics endpoint, Pod cache updates, and collector wiring.
- `pkg/sflow/decoder.go`: sFlow v5 datagram decoding and flow normalization.
- Pod IP cache code in `pkg/sflow`: maintains the IP-to-Pod mapping and fills
  Kubernetes fields for the source and destination sides.
- `pkg/sflow/owner.go`: best-effort top-level Kubernetes owner lookup.
- `pkg/sflow/otel.go`: OTLP logs exporter, attribute mapping, and batched
  exports to the OpenTelemetry Collector.
- `chart/templates/SFlow*.yaml`: disabled-by-default Deployment, ConfigMap,
  UDP Service, ServiceAccount, and RBAC.
- `chart/templates/EbpfFlowEventCollector.yaml`: shared OpenTelemetry Collector.
  sFlow adds a dedicated OTLP gRPC receiver on port 4319, an
  `elasticsearch/sflow` exporter, the `unifabric-switch-sflow` index template and ILM
  policy, and a Grafana datasource for the data stream.

### Data Flow

1. Switches send sFlow datagrams to the collector UDP Service.
2. The collector decodes supported flow samples and normalizes packet metadata.
3. Source and destination IPs are matched against the current Pod IP cache.
4. Matched endpoints receive Pod namespace, Pod name, Node name, and top-level
   owner fields.
5. Records enter a bounded writer queue.
6. The writer converts records to OTLP log records and exports them in batches
   to the OpenTelemetry Collector over gRPC.
7. The Collector writes documents into the Elasticsearch `unifabric-switch-sflow` data
   stream with ECS mapping.
8. Grafana dashboards query the data stream through the
   `unifabric-sflow-elasticsearch` datasource.

Unsupported sFlow records are skipped. Malformed datagrams increment decode
error metrics and do not stop the UDP receive loop.

### Decoding Model

The decoder accepts standard flow samples and expanded flow samples. It extracts
flow records from sampled IPv4 records, sampled IPv6 records, and sampled
Ethernet headers that contain IPv4 or IPv6 payloads.

Each normalized record contains:

- receive time and flow observation time
- sequence number and sampling rate
- switch sampler address normalized to 16 bytes
- sampled bytes and packets
- source and destination IPs normalized to 16 bytes
- Ethernet type, IP protocol, and transport ports when present
- source and destination AS values when gateway extensions are present

Records without usable endpoint addresses are not exported.

### Kubernetes Field Filling Rules

The collector periodically refreshes a Pod IP cache from Kubernetes Pods. Only
Running Pods with assigned Pod IPs are indexed.

Field filling follows these rules.

- A source IP match fills `src_*` attributes.
- A destination IP match fills `dst_*` attributes.
- Unmatched endpoints keep empty Kubernetes fields.
- Host-network Pods are covered when the observed endpoint IP matches a Pod IP.
- Stored documents are not rewritten; later Pod or owner changes do not modify
  historical documents.

Owner lookup follows the first owner reference until the highest readable owner.
If lookup fails, the collector preserves Pod fields and leaves owner fields
empty, or keeps the highest owner already resolved.

### OTLP and Elasticsearch Contract

Each flow record becomes one OTLP log record with event name `sflow.flow`,
resource attribute `service.name=unifabric-sflow`, and the flow observation
time as the record timestamp. Derived fields are denormalized at export time so
dashboards do not need query-time expressions:

- `scaled_bytes` and `scaled_packets` multiply the sampled values by the
  sampling rate (a zero sampling rate scales by 1).
- `sampler_ip`, `src_ip`, and `dst_ip` are string representations of the
  binary addresses.
- `protocol` is the IP protocol name (`TCP`, `UDP`, `ICMP`, `IPv6-ICMP`,
  `SCTP`, or `Protocol N`).
- `src_workload` and `dst_workload` concatenate the top-level owner as
  `namespace/kind/name`.

The Collector writes log records with the elasticsearch exporter in ECS mapping
mode, which flattens attributes to the document root. The chart applies an
index template and ILM policy named `unifabric-switch-sflow` before ingestion. The
template disables dynamic mapping, so every exported attribute has an explicit
field. Dashboard queries depend on these fields:

- `@timestamp`, `sampler_ip`, `bytes`, `packets`, `scaled_bytes`,
  `scaled_packets`, `sampling_rate`
- `src_pod_namespace`, `src_pod_name`, `src_node_name`, `dst_pod_namespace`,
  `dst_pod_name`, `dst_node_name`
- `src_pod_top_owner_*`, `dst_pod_top_owner_*`, `src_workload`, `dst_workload`
- `proto`, `protocol`, `src_ip`, `dst_ip`, `src_port`, `dst_port`

The ILM policy rolls the write index over daily or at 10gb per primary shard
and deletes backing indices three days after rollover by default. Operators can
adjust `sflow.elasticsearch.schema.ilm` according to their storage policy.

### Helm Configuration

The sFlow collector is controlled by the chart `sflow` values. Defaults keep it
disabled.

```yaml
sflow:
  enabled: false
  listen:
    bindAddress: ":6343"
    port: 6343
  service:
    enabled: true
    type: NodePort
    port: 6343
    nodePort: 0
  otel:
    endpoint: ""
    insecure: true
    timeout: 10s
  elasticsearch:
    index: unifabric-switch-sflow
    schema:
      ilm:
        rolloverMaxAge: 1d
        rolloverMaxPrimaryShardSize: 10gb
        deleteMinAge: 3d
    grafanaDatasource:
      enabled: true
  writer:
    batchSize: 2000
    flushInterval: 2s
    queueSize: 65536
```

`NodePort` is the default Service type because switch sFlow datagrams usually
arrive from outside the cluster. `nodePort: 0` lets Kubernetes allocate the UDP
node port; operators can set a fixed value when switches require a stable
target. `LoadBalancer` remains supported for environments with UDP external load
balancing.

An empty `sflow.otel.endpoint` targets the sFlow port of the in-release
`ebpfFlowEventCollector`. In that case the chart requires
`ebpfFlowEventCollector.elasticsearch.enabled=true`; leaving
`ebpfFlowEventCollector.elasticsearch.endpoints` empty additionally deploys the
bundled test Elasticsearch. Elasticsearch connection endpoints and credentials
are shared with the RDMA flow event pipeline through the
`ebpfFlowEventCollector.elasticsearch` values; sFlow data lands in its own data
stream.

When `sflow.enabled=false`, no sFlow collector resources are rendered. When
`sflow.enabled=true`, the chart renders the ConfigMap, Deployment, UDP Service,
ServiceAccount, ClusterRole, and ClusterRoleBinding, and extends the shared
OpenTelemetry Collector with the sFlow receiver, exporter, and schema. sFlow
dashboards are rendered when `grafanaDashboard.enabled=true` and
`sflow.enabled=true`.

### Overload Behavior

UDP receive is isolated from OTLP exports by a bounded in-process queue. When
exports slow down and the queue is full, the collector drops new normalized
records, increments `unifabric_sflow_records_dropped_total`, and logs a
structured warning. This bounds memory use and keeps the receive loop available
for current switch telemetry.

Export failures increment `unifabric_sflow_write_errors_total`. Failed batches
are not retried indefinitely because stale flow samples have limited
troubleshooting value, while the collector still needs to keep receiving current
traffic. The OpenTelemetry Collector adds its own bounded sending queue and
retry policy in front of Elasticsearch.

### RBAC and Permissions

The collector needs read access to Pods and common workload owner objects to
fill Pod / Workload fields.

```yaml
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["daemonsets", "deployments", "replicasets", "statefulsets"]
  verbs: ["get"]
- apiGroups: ["batch"]
  resources: ["cronjobs", "jobs"]
  verbs: ["get"]
- apiGroups: ["kubeflow.org"]
  resources: ["mpijobs", "pytorchjobs", "tfjobs", "xgboostjobs"]
  verbs: ["get"]
- apiGroups: ["ray.io"]
  resources: ["rayjobs"]
  verbs: ["get"]
- apiGroups: ["sparkoperator.k8s.io"]
  resources: ["sparkapplications"]
  verbs: ["get"]
```

### Observability

The collector exposes controller-runtime health checks on the configured health
bind address:

- `/healthz`
- `/readyz`

It also exposes Prometheus metrics on the configured metrics bind address.

| Metric | Type | Meaning |
| --- | --- | --- |
| `unifabric_sflow_datagrams_accepted_total` | Counter | sFlow datagrams accepted by the collector. |
| `unifabric_sflow_decode_errors_total` | Counter | Datagrams that failed decoding. |
| `unifabric_sflow_records_decoded_total` | Counter | Normalized records decoded from datagrams. |
| `unifabric_sflow_records_dropped_total` | Counter | Records dropped because the writer queue was full. |
| `unifabric_sflow_records_written_total` | Counter | Records successfully handed to the OTLP exporter. |
| `unifabric_sflow_write_errors_total` | Counter | OTLP export failures. |
| `unifabric_sflow_queue_depth` | Gauge | Records currently waiting to be exported. |

Logs should include:

- datagram decode failures with remote address and error
- writer queue full warnings
- OTLP export failures with batch record count and error

### Test Plan

#### Unit Tests

- Config parsing and validation tests for defaults, bind addresses, OTLP
  endpoint, timeout, batch size, queue size, and flush interval.
- Decoder tests for IPv4, IPv6, sampled headers, multi-record datagrams, and
  malformed input.
- Pod field filling tests for source match, destination match, unmatched
  endpoints, host-network Pods, and owner fallback.
- OTLP attribute mapping tests for derived fields such as `scaled_bytes`,
  `protocol`, and `src_workload`/`dst_workload`.
- Collector queue tests for batch flush, overload drops, and metrics updates.
- Command wiring tests for metrics / health server options and startup wiring.

#### Helm and Documentation Tests

- `helm template unifabric ./chart` verifies the default chart render does not
  contain sFlow collector resources.
- `helm template unifabric ./chart --set sflow.enabled=true --set ebpfFlowEventCollector.elasticsearch.enabled=true`
  verifies the sFlow collector, UDP Service, RBAC, Collector pipeline, and
  dashboards render.
- `make helm-docs` verifies `chart/README.md` stays synchronized with values.
- Check that `switch-sflow.json`, `workload-sflow.json`, and
  `workload-pair-sflow.json` only query fields mapped by the `unifabric-switch-sflow`
  index template.

### Acceptance Criteria

This capability should satisfy at least these conditions.

- The collector can receive valid sFlow datagrams and produce normalized records
  from supported samples.
- Source or destination endpoints that match Pod IPs can receive Pod, Node, and
  top-level workload owner fields.
- Base flow records can still be exported when Pod IP matching or owner lookup
  fails.
- The `unifabric-switch-sflow` index template fields support the switch, workload, and
  workload pair sFlow dashboard queries.
- When the writer queue is full, the collector avoids unbounded memory growth
  and exposes drops through metrics.
- The chart does not expose a new UDP port by default; related resources render
  only after `sflow.enabled=true`.
- `gosflow/` is not imported by production code and does not enter the image
  build context.

## Implementation History

- This implementation adds `cmd/sflow` and `pkg/sflow`, deployed by the Helm
  chart as an independent Deployment.
- The storage path originally wrote rows directly to a ClickHouse `flows_raw`
  table. It now exports OTLP log records to the shared OpenTelemetry Collector,
  which writes them into Elasticsearch, matching the RDMA flow event pipeline.
- User-provided `switch-sflow.json` and `workload-sflow.json` remain dashboard
  assets under `chart/files/dashboard-en/`, rewritten as Elasticsearch panels.

## Drawbacks

- The current design uses a single collector instance for UDP sFlow receive. If
  high availability or horizontal sharding is required, load distribution and
  duplicate sample handling need a follow-up design.
- Pod / Workload fields depend on the current Pod cache, so records may briefly
  miss Pod or owner fields.
- Failed OTLP batches are not retried indefinitely, so an extended Collector or
  Elasticsearch outage can lose sampled records.
- Elasticsearch count-distinct aggregations are approximate, so the dashboard
  distinct-count stats are estimates rather than exact counts.

## Alternatives

- Put sFlow processing into the existing Controller process. This would add UDP
  ingest and OTLP exports to the control-plane reconcile process, increasing
  the impact on existing Controller behavior.
- Write rows directly to ClickHouse from the collector. An earlier iteration
  did this; it required a ClickHouse driver, schema migrations, and a Lease to
  serialize them. Exporting over OTLP reuses the existing Collector-to-
  Elasticsearch pipeline and removes the storage-specific code.
- Write raw sFlow data first and fill Pod fields at query time. This would make
  dashboard queries depend on historical Pod state or an additional dimension
  table.
- Port the full reference `gosflow/` directory. This would add unnecessary
  sample-code maintenance cost, so production code only follows the needed
  behavior and does not import that directory.
