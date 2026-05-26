# Tasks: Switch-Driven Scale-Out Topology Discovery

**Input**: Design documents from `/specs/002-switchgroup-topology/`  
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Include focused unit tests and e2e validation only for this feature.

**Organization**: Tasks are grouped by phase and mapped to the single P1 user story so the switch-driven topology path can be implemented and validated as one independently testable increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no blocking dependency)
- **[Story]**: Which user story this task belongs to (`[US1]` for this feature)
- Every task includes exact file paths

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish the new switch-agent binary, package layout, and build support required by the feature.

- [x] T001 Create the switch-agent entrypoint in `cmd/switch-agent/main.go` and runtime package scaffold in `pkg/switchagent/agent.go`
- [x] T002 Add switch-agent build and image targets in `Makefile` and `image/switch-agent/Dockerfile`
- [x] T003 Add gRPC/protobuf dependencies and generation commands in `go.mod` and `Makefile`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Define the public contracts, config surface, and controller/chart wiring that all story work depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T004 [P] Define the `Switch` API resource in `pkg/api/v1beta1/switch_types.go`
- [x] T005 [P] Confirm topology groups remain an internal projection rather than a separate CRD
- [x] T006 Update API registration and generated CRD outputs in `pkg/api/v1beta1/crd_types.go`, `pkg/api/v1beta1/zz_generated.deepcopy.go`, and `chart/crds/unifabric.io_switches.yaml`
- [x] T007 Extend controller config for switch discovery, pinned mTLS, and group naming options in `pkg/config/controller.go`
- [x] T008 Add switch discovery values and controller ConfigMap wiring in `chart/values.yaml` and `chart/templates/ConfigMap.yaml`
- [x] T009 Add RBAC and pinned mTLS template scaffolding in `chart/templates/Role.yaml`, `chart/templates/Deployment.yaml`, `chart/templates/_helpers.tpl`, and `chart/templates/SwitchMTLSSecret.yaml`
- [x] T010 Wire the controller startup path so switch-driven discovery becomes the active scale-out topology path in `pkg/controller/controller.go`

**Checkpoint**: Foundation ready. `Switch`, config, chart values, and controller wiring are in place so story implementation can start.

---

## Phase 3: User Story 1 - Use Topology-Aware Scheduling in a GPU Cluster (Priority: P1) 🎯 MVP

**Goal**: Replace leaf-only scale-out discovery with a switch-driven path that ingests switch connectivity, computes leaf/spine/core topology, and writes stable scheduling labels.

**Independent Test**: In a cluster with known node-to-leaf and switch-to-switch connectivity, the controller can ingest switch snapshots, maintain `Switch` resources, and write stable leaf/spine/core labels back to participating GPU nodes.

### Tests for User Story 1 ⚠️

> **NOTE**: Write these tests first and make sure they fail before implementation.

- [x] T011 [P] [US1] Add topology engine unit tests for tier inference, deterministic grouping, and concat/hash naming in `pkg/topology/topology_test.go`
- [x] T012 [P] [US1] Add controller unit tests for switch status ingestion, internal topology projection, and node label cleanup in `pkg/controller/switchtopology/switchtopology_test.go`
- [x] T013 [US1] Add an e2e scenario for switch subscriptions and node label projection in `e2e/e2e_check.go` and `e2e/topology/`

### Implementation for User Story 1

- [x] T014 [US1] Define the SwitchReporter gRPC contract and generated stubs in `pkg/switchagent/switchreporter.proto`, `pkg/switchagent/switchreporter.pb.go`, and `pkg/switchagent/switchreporter_grpc.pb.go`
- [x] T015 [P] [US1] Implement switch-agent configuration loading and pinned mTLS material handling in `pkg/config/switchagent.go` and `pkg/switchagent/agent.go`
- [x] T016 [US1] Implement switch-agent LLDP snapshot collection and `WatchLLDPNeighbors` server behavior in `pkg/switchagent/lldp.go` and `pkg/switchagent/agent.go` (depends on T014, T015)
- [x] T017 [P] [US1] Refine `pkg/topology/topology.go` so it consumes `FabricNode` and `Switch` inputs and emits deterministic topology outputs in `pkg/topology/topology.go`
- [x] T018 [US1] Implement the controller-side gRPC client, retries, and `Switch.status` update flow in `pkg/controller/switchtopology/subscription.go` (depends on T014, T007)
- [x] T019 [US1] Implement the `SwitchTopologyDiscovery` reconcile loop in `pkg/controller/switchtopology/switchtopology.go` (depends on T017, T018)
- [x] T020 [US1] Implement internal topology group projection for tier, switch refs, optional node refs, and health in `pkg/controller/switchtopology/projection.go` (depends on T019)
- [x] T021 [US1] Implement node label projection and stale label cleanup using configurable label keys in `pkg/controller/switchtopology/projection.go` (depends on T019)
- [x] T022 [US1] Implement snapshot validation, generation ordering, and stale-topology health handling in `pkg/controller/switchtopology/subscription.go` (depends on T018)
- [x] T023 [US1] Implement switch count and LLDP parse observability in `pkg/controller/switchtopology/metrics.go` and `pkg/controller/switchtopology/switchtopology.go` (depends on T019)
- [x] T024 [US1] Integrate the new switch-agent runtime and controller package into the shipped binaries and images in `cmd/switch-agent/main.go`, `cmd/controller/main.go`, `pkg/controller/controller.go`, and `image/switch-agent/Dockerfile` (depends on T016, T019)
- [x] T025 [US1] Remove legacy leaf-group API, controller, CRD, docs, and test ownership in favor of `pkg/controller/switchtopology` (depends on T021)

**Checkpoint**: User Story 1 is functional. The cluster can ingest switch snapshots, compute internal topology groups, and write stable scale-out labels without conflicting leaf outputs.

---

## Phase 4: Polish & Cross-Cutting Concerns

**Purpose**: Refresh generated assets, update docs, and run feature-scoped validation.

- [x] T026 Update rollout and migration documentation in `docs/design/scaleout-topology.md` and `specs/002-switchgroup-topology/quickstart.md`
- [x] T027 Run `make crd` to refresh `pkg/api/v1beta1/zz_generated.deepcopy.go` and `chart/crds/unifabric.io_switches.yaml`
- [x] T028 Run `make helm-docs` to regenerate `chart/README.md` from `chart/values.yaml` and `chart/README.md.gotmpl`
- [x] T029 Run `helm template unifabric ./chart` against `chart/values.yaml` and `chart/templates/`
- [x] T030 Run focused unit validation with `go test` for `pkg/topology/topology.go`, `pkg/controller/switchtopology/`, and `pkg/switchagent/`
- [ ] T031 Run end-to-end validation with `make test-e2e` using `e2e/e2e_check.go` and `e2e/topology/`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies; can start immediately.
- **Foundational (Phase 2)**: Depends on Setup completion; blocks all story work.
- **User Story 1 (Phase 3)**: Depends on Foundational completion.
- **Polish (Phase 4)**: Depends on User Story 1 completion.

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Phase 2 and is the MVP for this feature.

### Within User Story 1

- Unit tests and the e2e scenario are written before implementation.
- The gRPC contract (`T014`) must exist before switch-agent and controller subscription work (`T016`, `T018`).
- Topology reconciliation (`T019`) depends on both topology engine updates (`T017`) and switch status ingestion (`T018`).
- Status projection, label projection, observability, and legacy removal (`T020`-`T025`) follow the main reconcile loop.

### Parallel Opportunities

- `T004` and `T005` can run in parallel because they define different CRD type files.
- `T011` and `T012` can run in parallel because they cover different packages.
- `T015` and `T017` can run in parallel after the foundational phase because they touch different packages.

---

## Parallel Example: User Story 1

```bash
# Launch the two unit-test tasks together:
Task: "T011 Add topology engine unit tests in pkg/topology/topology_test.go"
Task: "T012 Add controller unit tests in pkg/controller/switchtopology/switchtopology_test.go"

# After the gRPC contract and foundational wiring are ready, these can proceed together:
Task: "T015 Implement switch-agent configuration loading in pkg/config/switchagent.go and pkg/switchagent/agent.go"
Task: "T017 Refine pkg/topology/topology.go for topology-ready outputs"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup.
2. Complete Phase 2: Foundational.
3. Complete Phase 3: User Story 1.
4. **STOP and VALIDATE**: Run the unit tests and e2e scenario for User Story 1.
5. Demo the switch-driven path before moving to polish tasks.

### Incremental Delivery

1. Finish Setup + Foundational so the repository can build the new feature path.
2. Add the switch subscription contract, topology engine integration, and controller reconcile loop.
3. Add internal topology projection and Node label cleanup.
4. Finish generated assets, docs, and chart validation.

### Parallel Team Strategy

With multiple developers:

1. One developer handles CRDs/config/chart wiring through Phase 2.
2. After Phase 2 completes:
   - Developer A: switch-agent contract and runtime (`T014`-`T016`)
   - Developer B: topology engine and reconcile loop (`T017`-`T021`)
   - Developer C: observability, legacy removal, and validation (`T022`-`T031`)

---

## Notes

- `[P]` tasks touch different files and can be parallelized safely.
- All test tasks are limited to unit and e2e coverage, matching the requested scope.
- Generated files should be refreshed through their owning generators rather than edited by hand.
- Legacy leaf-group output is removed in favor of `pkg/controller/switchtopology`.
