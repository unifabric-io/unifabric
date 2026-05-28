# sFlow Processing Usage Guide

This guide explains how to enable the Unifabric sFlow collector, configure
switch exporters, prepare ClickHouse, and verify the switch and workload
dashboards.

For the implementation model, see [sFlow processing design](../design/sflow.md).

## Prerequisites

- A reachable ClickHouse instance using the native protocol port.
- The `flows_raw` table created from `docs/sql/sflow-flows-raw.sql`.
- Switches that can export sFlow v5 datagrams to a Kubernetes Service address.
- Grafana dashboard import support if `grafanaDashboard.enabled=true`.

## Prepare ClickHouse

Create the table:

```bash
clickhouse-client --multiquery < docs/sql/sflow-flows-raw.sql
```

If the target database is not `default`, edit the SQL database name before
applying it and set `sflow.clickhouse.database` and `sflow.clickhouse.table` to
the matching destination.

## Enable the Collector

Create a values file:

```yaml
sflow:
  enabled: true
  service:
    type: NodePort
    port: 6343
    # Optional. Omit or set 0 to let Kubernetes allocate a nodePort.
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

`NodePort` is the default because switch sFlow traffic normally arrives from
outside the cluster. Set `sflow.service.nodePort` when switches need a stable
UDP node port; leave it as `0` to let Kubernetes allocate one. Use
`LoadBalancer` when the environment provides an external load balancer for UDP
traffic.

Render and install:

```bash
helm template unifabric ./chart -f sflow-values.yaml
helm upgrade --install unifabric ./chart -f sflow-values.yaml
```

## Configure Switches

Point each switch sFlow exporter at a reachable node address and the Service
nodePort. If `sflow.service.type=LoadBalancer`, point exporters at the load
balancer address and UDP `sflow.service.port`. The container port remains
`6343` by default.

Choose a sampling rate that matches switch capacity and expected traffic volume.
A lower numeric sampling rate produces more records and more ClickHouse writes.
If the collector reports queue drops, increase sampling rate, increase
`sflow.writer.queueSize`, or scale storage capacity.

## Verify the Collector

Check rendered resources:

```bash
kubectl -n unifabric-system get deploy,svc,cm -l app.kubernetes.io/component=unifabric-sflow
kubectl -n unifabric-system get clusterrole,clusterrolebinding -l app.kubernetes.io/component=unifabric-sflow
```

Check Pod health and logs:

```bash
kubectl -n unifabric-system get pods -l app.kubernetes.io/component=unifabric-sflow
kubectl -n unifabric-system logs deploy/unifabric-sflow -c sflow
```

Check the metrics endpoint from inside the cluster:

```bash
POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-sflow -o jsonpath='{.items[0].status.podIP}')
kubectl -n unifabric-system run sflow-metrics-check --rm -i --restart=Never --image=curlimages/curl -- \
  "http://${POD_IP}:8084/metrics"
```

Useful metrics:

- `unifabric_sflow_datagrams_accepted_total`: datagrams received from switches.
- `unifabric_sflow_decode_errors_total`: malformed or unsupported datagrams.
- `unifabric_sflow_records_decoded_total`: normalized records produced.
- `unifabric_sflow_records_written_total`: records written to ClickHouse.
- `unifabric_sflow_records_dropped_total`: records dropped because the writer
  queue was full.
- `unifabric_sflow_write_errors_total`: ClickHouse write failures.
- `unifabric_sflow_queue_depth`: current queued records.

## Verify Stored Records

Query recent rows:

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

Expected result: records involving active cluster Pods include Pod, Node, and
workload owner fields on the matched side. Traffic to external endpoints keeps
empty Kubernetes fields for the unmatched side.

## Verify Dashboards

When `grafanaDashboard.enabled=true` and `sflow.enabled=true`, the chart renders:

- `switch-sflow.json`
- `workload-sflow.json`

The switch dashboard starts from a switch sampler address and shows sampled
traffic, workloads, Pods, samples, protocols, and top talkers. The workload
dashboard starts from a workload owner and shows transmit and receive traffic by
switch and Pod.

If dashboards are empty, first confirm ClickHouse rows exist for the selected
time range, then check the dashboard data source and variables.

## Troubleshooting

If the collector is not receiving datagrams:

- Confirm the Service type and address are reachable from switches.
- Confirm switch exporters target UDP port `6343` or the configured
  `sflow.service.port`.
- Confirm Kubernetes NetworkPolicy or external firewalls allow UDP traffic.

If rows are written without Pod fields:

- Confirm the endpoint IPs in sampled records are Pod IPs, not only node or NAT
  addresses.
- Confirm the target Pods are running and have assigned Pod IPs.
- Confirm the collector RBAC can list Pods and get owner objects.
- Allow up to one Pod cache refresh interval after Pod creation.

If ClickHouse writes fail:

- Confirm `sflow.clickhouse.address` includes host and port.
- Confirm the table exists and matches `docs/sql/sflow-flows-raw.sql`.
- Confirm the configured username and Secret password are valid.
- Check `unifabric_sflow_write_errors_total` and collector logs.

If records are dropped:

- Check `unifabric_sflow_queue_depth` and
  `unifabric_sflow_records_dropped_total`.
- Increase switch sampling rate to reduce input volume.
- Increase `sflow.writer.queueSize` or `sflow.writer.batchSize`.
- Check ClickHouse insert latency and resource saturation.
