# Tasks: sFlow Processing and Workload Attribution

**Input**: Design documents from `/specs/003-sflow-processing/`
**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`
**Tests**: Included because the project constitution requires focused tests for Go behavior changes and Helm rendering validation for chart changes.
**Organization**: One MVP user story, implemented as an independently testable collector from sFlow UDP receive through workload-attributed ClickHouse rows and dashboards.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel with other marked tasks because it touches different files and does not depend on incomplete tasks.
- **[Story]**: User story label for story-phase tasks only.
- Every task includes exact file paths for execution.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish the committed project locations and build inputs for the sFlow collector.

- [X] T001 Add sFlow decoder and ClickHouse writer module dependencies in `go.mod` and `go.sum`.
- [X] T002 [P] Move the `flows_raw` initialization schema from `1.sql` into committed project documentation at `docs/sql/sflow-flows-raw.sql`.
- [X] T003 [P] Create the sFlow command entrypoint scaffold in `cmd/sflow/main.go`.
- [X] T004 [P] Create shared sFlow record and attribution types in `pkg/sflow/types.go`.
- [X] T005 Add sFlow binary and image build targets to `Makefile`.
- [X] T006 [P] Create the sFlow container build file in `image/sflow/Dockerfile`.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core configuration, metrics, and package structure that must exist before the user story can be implemented.

**Critical**: No user story implementation should begin until this phase is complete.

- [X] T007 Implement sFlow collector configuration structs, defaults, and validation in `pkg/config/sflow.go`.
- [X] T008 [P] Add unit tests for sFlow collector configuration defaults and validation in `pkg/config/sflow_test.go`.
- [X] T009 Implement collector lifecycle interfaces, queue settings, and dependency injection seams in `pkg/sflow/collector.go`.
- [X] T010 [P] Define collector health and Prometheus metric names in `pkg/sflow/metrics.go`.
- [X] T011 [P] Define Kubernetes owner lookup helpers and supported owner kinds in `pkg/sflow/owner.go`.

**Checkpoint**: Configuration, metrics, and package scaffolding compile; User Story 1 work can proceed.

---

## Phase 3: User Story 1 - Observe Switch sFlow by Kubernetes Workload (Priority: P1) MVP

**Goal**: Operators can receive switch sFlow, enrich valid flow records with Pod and workload ownership, write queryable rows, and use the provided switch/workload dashboards.

**Independent Test**: Send valid sFlow samples involving known Pods, then query `flows_raw` and verify sampler, traffic, source/destination Pod, node, owner, protocol, byte, packet, and sample fields are available for dashboard queries.

### Tests for User Story 1

- [X] T012 [P] [US1] Add decoder tests for IPv4, IPv6, sampled headers, multi-record datagrams, and malformed input in `pkg/sflow/decoder_test.go`.
- [X] T013 [P] [US1] Add attribution tests for source match, destination match, unmatched endpoints, host-network Pods, and owner fallback in `pkg/sflow/attribution_test.go`.
- [X] T014 [P] [US1] Add ClickHouse row mapping and batch flush tests in `pkg/sflow/clickhouse_test.go`.
- [X] T015 [P] [US1] Add collector queue, overload, and metric update tests in `pkg/sflow/collector_test.go`.
- [X] T016 [P] [US1] Add command wiring tests for config loading and startup validation in `cmd/sflow/main_test.go`.

### Implementation for User Story 1

- [X] T017 [US1] Implement sFlow datagram decoding and flow normalization in `pkg/sflow/decoder.go`.
- [X] T018 [US1] Implement Pod IP cache and source/destination endpoint attribution in `pkg/sflow/attribution.go`.
- [X] T019 [US1] Implement top-level Kubernetes owner resolution for attributed Pods in `pkg/sflow/owner.go`.
- [X] T020 [US1] Implement ClickHouse connection, row mapping, and batched writes to `flows_raw` in `pkg/sflow/clickhouse.go`.
- [X] T021 [US1] Implement UDP receive loop, bounded queue behavior, graceful shutdown, and overload handling in `pkg/sflow/collector.go`.
- [X] T022 [US1] Implement collector metrics registration and updates in `pkg/sflow/metrics.go`.
- [X] T023 [US1] Wire config loading, Kubernetes client creation, health probes, metrics endpoint, and collector startup in `cmd/sflow/main.go`.
- [X] T024 [US1] Add sFlow Helm values with comments and safe disabled-by-default defaults in `chart/values.yaml`.
- [X] T025 [US1] Add sFlow Helm naming, image, and service account helper templates in `chart/templates/_helpers.tpl`.
- [X] T026 [US1] Render the sFlow collector config and ClickHouse password source in `chart/templates/SFlowConfigMap.yaml`.
- [X] T027 [US1] Render the sFlow Deployment, container ports, probes, config mounts, security context, and resources in `chart/templates/SFlowDeployment.yaml`.
- [X] T028 [US1] Render the sFlow UDP Service in `chart/templates/SFlowService.yaml`.
- [X] T029 [US1] Render sFlow ServiceAccount, ClusterRole, and ClusterRoleBinding for Pod and owner reads in `chart/templates/SFlowRBAC.yaml`.
- [X] T030 [US1] Update Grafana dashboard rendering gates so `chart/files/switch-sflow.json` and `chart/files/workload-sflow.json` render when sFlow dashboards are enabled in `chart/templates/GrafanaDashboardRDMA.yaml`.
- [X] T031 [US1] Verify the switch and workload dashboards query only fields provided by `docs/sql/sflow-flows-raw.sql` in `chart/files/switch-sflow.json` and `chart/files/workload-sflow.json`.

**Checkpoint**: User Story 1 is functional and can be validated with `go test`, ClickHouse queries, and Helm rendering.

---

## Phase 4: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, generated chart docs, and final validation.

- [X] T032 [P] Document the sFlow collector design, attribution rules, and overload behavior in `docs/design/sflow.md`.
- [X] T033 [P] Document sFlow installation, switch configuration, ClickHouse setup, and dashboard verification in `docs/usage/sflow.md`.
- [X] T034 [P] Link the new sFlow usage and design docs from `docs/README.md` and `docs/README.zh.md`.
- [X] T035 Run `gofmt` on `cmd/sflow/main.go`, `pkg/config/sflow.go`, and `pkg/sflow/*.go`.
- [X] T036 Run `go test ./cmd/sflow ./pkg/sflow/... ./pkg/config` for `cmd/sflow`, `pkg/sflow`, and `pkg/config`.
- [X] T037 Run `make build` to validate `cmd/controller`, `cmd/agent`, and `cmd/sflow` build outputs from `Makefile`.
- [X] T038 Run `make helm-docs` and commit regenerated chart documentation in `chart/README.md`.
- [X] T039 Run `helm template unifabric ./chart` to validate rendered sFlow chart resources in `chart/templates/`.
- [X] T040 Confirm `gosflow/` remains uncommitted reference material and is not imported by production code in `go.mod`, `cmd/sflow/main.go`, or `pkg/sflow/collector.go`.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies.
- **Foundational (Phase 2)**: Depends on Setup completion.
- **User Story 1 (Phase 3)**: Depends on Foundational completion.
- **Polish (Phase 4)**: Depends on User Story 1 completion.

### User Story Dependencies

- **User Story 1 (P1)**: Starts after Foundational and has no dependency on other user stories because it is the only story in scope.

### Within User Story 1

- Tests T012-T016 should be written before implementation tasks T017-T023 where practical.
- Decoder T017 should be completed before collector integration T021.
- Attribution T018 and owner resolution T019 should be completed before row mapping T020.
- Go implementation T017-T023 should be complete before Helm tasks T024-T031 are fully validated.
- Helm values T024 should be complete before template tasks T025-T030.

### Parallel Opportunities

- T002, T003, T004, and T006 can run in parallel after T001 is understood.
- T008, T010, and T011 can run in parallel after T007 is defined.
- T012-T016 can run in parallel because each test file targets a different behavior surface.
- T024-T031 can be split after the sFlow config and runtime contract are stable.
- T032-T034 can run in parallel with final validation once the behavior and public values are stable.

---

## Parallel Example: User Story 1

```text
Task: "T012 [P] [US1] Add decoder tests for IPv4, IPv6, sampled headers, multi-record datagrams, and malformed input in pkg/sflow/decoder_test.go"
Task: "T013 [P] [US1] Add attribution tests for source match, destination match, unmatched endpoints, host-network Pods, and owner fallback in pkg/sflow/attribution_test.go"
Task: "T014 [P] [US1] Add ClickHouse row mapping and batch flush tests in pkg/sflow/clickhouse_test.go"
Task: "T015 [P] [US1] Add collector queue, overload, and metric update tests in pkg/sflow/collector_test.go"
Task: "T016 [P] [US1] Add command wiring tests for config loading and startup validation in cmd/sflow/main_test.go"
```

---

## Implementation Strategy

### MVP First

1. Complete Setup and Foundational phases.
2. Complete User Story 1 tests and Go implementation.
3. Add Helm packaging for the collector and dashboards.
4. Stop and validate the end-to-end path from sFlow datagram to `flows_raw` query and dashboard rendering.

### Incremental Delivery

1. Build a decoder and row model that passes local tests.
2. Add Pod/workload attribution and preserve unmatched records.
3. Add ClickHouse batching and overload visibility.
4. Add Helm deployment and public values.
5. Add docs and run final validation.

### Validation Gate

Before implementation is considered complete, run:

```bash
go test ./cmd/sflow ./pkg/sflow/... ./pkg/config
make build
make helm-docs
helm template unifabric ./chart
```

## Notes

- `gosflow/` is sample input only and must not become production source.
- The existing `chart/files/switch-sflow.json` and `chart/files/workload-sflow.json` are treated as user-provided dashboard assets and should only be changed for contract alignment.
- Keep the collector disabled by default in Helm to avoid changing existing installations.
