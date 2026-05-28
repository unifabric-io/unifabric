# Quickstart: sFlow Processing and Workload Attribution

## 1. Prepare ClickHouse Access

Provide a ClickHouse endpoint that the sFlow collector can reach. The collector manages its own schema at startup: it creates the target database, records applied schema versions in `__unifabric_sflow_schema_migrations`, creates `flows_raw` when missing, and reconciles the table TTL. Migration versions use the `YYYYMMDDNNNN` format, for example `202606040001`.

## 2. Enable the sFlow collector

Create a values file:

```yaml
sflow:
  enabled: true
  listen:
    port: 6343
  service:
    type: NodePort
    port: 6343
    nodePort: 0
  clickhouse:
    address: clickhouse-clickhouse.clickhouse.svc.cluster.local:9000
    database: default
    username: default
    passwordSecret:
      name: clickhouse-auth
      key: password
    schema:
      retentionDays: 3
  writer:
    batchSize: 2000
    flushInterval: 2s
    queueSize: 65536
```

Render before applying:

```bash
helm template unifabric ./chart -f sflow-values.yaml
```

Install or upgrade:

```bash
helm upgrade --install unifabric ./chart -f sflow-values.yaml
```

## 3. Configure switches

Point each switch sFlow exporter at the rendered sFlow Service address and UDP port. Keep the switch sampling rate appropriate for the expected traffic volume.

## 4. Verify collector health

Check the collector Pod:

```bash
kubectl -n <namespace> get pods -l app.kubernetes.io/component=unifabric-sflow
kubectl -n <namespace> logs deploy/unifabric-sflow
```

Check metrics for accepted datagrams, decoded records, decode errors, queue saturation, and write failures.

## 5. Verify stored records

Run a query for recent records:

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

Expected result: records involving active cluster Pods include Pod, node, and workload owner fields for the matched source and/or destination side. Records involving external endpoints keep empty Kubernetes fields for the unmatched side.

## 6. Verify dashboards

Open the rendered `Unifabric Switch sFlow` and `Unifabric Workload sFlow` dashboards.

- The switch dashboard should show observed traffic, workloads, Pods, flow samples, top workloads, top Pods, and protocols.
- The workload dashboard should show transmit and receive traffic by switch and top Pods for the selected workload.

## 7. Validation commands

```bash
gofmt -w cmd/sflow pkg/sflow pkg/config/sflow.go
go test ./cmd/sflow ./pkg/sflow/... ./pkg/config
make build
make helm-docs
helm template unifabric ./chart
```
