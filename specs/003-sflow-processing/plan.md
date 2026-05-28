# Implementation Plan: sFlow Processing and Workload Attribution

**Branch**: `003-sflow-processing` | **Date**: 2026-05-28 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/003-sflow-processing/spec.md`

## Summary

Add a Kubernetes-aware sFlow collector that receives switch-pushed sFlow datagrams, decodes flow samples, enriches source and destination endpoints with Pod, node, and top-level workload owner metadata, and writes enriched records to the `flows_raw` analytical table consumed by the existing switch and workload dashboards. The reference `gosflow/` directory is treated as non-committed sample code only; production code is implemented in the project tree.

## Technical Context

**Language/Version**: Go 1.26.2
**Primary Dependencies**: `github.com/netsampler/goflow2/v2` for sFlow decoding, `github.com/ClickHouse/clickhouse-go/v2` for ClickHouse writes, controller-runtime/client-go for Kubernetes Pod and owner metadata, Prometheus client for collector health metrics
**Storage**: ClickHouse `flows_raw` table seeded from the schema currently captured in `1.sql` and later committed as project documentation or chart asset
**Testing**: `go test` for decoder, attribution, writer, config, and command wiring; `helm template unifabric ./chart` for rendered manifests
**Target Platform**: Linux container running in Kubernetes
**Project Type**: Kubernetes service component packaged by the existing Helm chart
**Performance Goals**: Make at least 95% of valid records available for dashboard queries within 30 seconds during expected sustained ingest; tolerate short bursts without blocking UDP receive
**Constraints**: Preserve existing controller and agent behavior; keep `gosflow/` out of committed source; keep attribution best-effort and leave fields empty when confidence is insufficient; avoid mutating historical attribution
**Scale/Scope**: One collector instance can receive from multiple switches through one configurable UDP endpoint; HA or sharded ingestion is outside this feature unless added later by configuration

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- Affected public contracts: new Helm values under `sflow`, new sFlow UDP Service, new sFlow Deployment, new collector config file, new ClickHouse table contract, new dashboards already present under `chart/files/`, and new docs.
- No CRD or Node label changes are planned.
- Helm chart changes require `make helm-docs` and `helm template unifabric ./chart`.
- Go changes require `gofmt`, focused `go test ./cmd/sflow ./pkg/sflow/... ./pkg/config`, and `make build`.
- Compatibility impact: existing controller, agent, RDMA metrics, CRDs, labels, and dashboards remain enabled as before. The sFlow collector is opt-in by Helm value so existing installs are not forced to expose a new UDP port.

**Initial Gate Result**: PASS. The design adds a scoped component and public Helm/config contracts without changing existing topology resources.

## Project Structure

### Documentation (this feature)

```text
specs/003-sflow-processing/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── sflow-collector.md
│   ├── flows-raw.md
│   └── helm-values.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/
└── sflow/
    └── main.go

pkg/
├── config/
│   ├── sflow.go
│   └── sflow_test.go
└── sflow/
    ├── attribution.go
    ├── attribution_test.go
    ├── clickhouse.go
    ├── clickhouse_test.go
    ├── collector.go
    ├── decoder.go
    ├── decoder_test.go
    ├── metrics.go
    ├── owner.go
    └── types.go

image/
└── sflow/
    └── Dockerfile

chart/
├── files/
│   ├── switch-sflow.json
│   └── workload-sflow.json
├── templates/
│   ├── SFlowConfigMap.yaml
│   ├── SFlowDeployment.yaml
│   ├── SFlowRBAC.yaml
│   └── SFlowService.yaml
└── values.yaml

docs/
├── design/sflow.md
├── sql/sflow-flows-raw.sql
└── usage/sflow.md
```

**Structure Decision**: Use a new `cmd/sflow` binary and `pkg/sflow` package. This keeps UDP ingestion and ClickHouse writes separate from the existing controller reconciliation loop and node-local agent while sharing the repository's Go, Helm, logging, and config patterns.

## Complexity Tracking

No constitution violations require justification.

## Post-Design Constitution Check

- Public contracts are captured in `contracts/`.
- Helm and docs regeneration are included in `tasks.md`.
- Focused Go tests and build validation are included in `tasks.md`.
- Backward compatibility is preserved by making the collector disabled by default unless explicitly enabled through Helm.
