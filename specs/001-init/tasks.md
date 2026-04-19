# Tasks: Project Initialization Scope

**Input**: `specs/001-init/spec.md`  
**Status**: Completed  
**Scope**: Initial project functionality only: FabricNode, ScaleOutLeafGroup, and RDMA metrics.

## Phase 1: FabricNode

- [x] T001 [US1] Define the `FabricNode` API resource for node-local RDMA topology state in `pkg/api/v1beta1/fabricnode_types.go`.
- [x] T002 [US1] Generate and include CRD output for `FabricNode` in `chart/crds/unifabric.io_fabricnodes.yaml`.
- [x] T003 [US1] Implement node-local RDMA interface discovery in `pkg/agent/fabricnode/fabricnode.go`.
- [x] T004 [US1] Classify RDMA interfaces into GPU and storage topology lists using configured selectors.
- [x] T005 [US1] Populate LLDP neighbor, RDMA device, link state, IP address, node type, and node IP status fields.
- [x] T006 [US1] Track RDMA-enabled Pods and top-level owner metadata for metrics attribution.

## Phase 2: ScaleOutLeafGroup

- [x] T007 [US2] Define the `ScaleOutLeafGroup` API resource for scale-out leaf topology grouping.
- [x] T008 [US2] Generate and include CRD output for `ScaleOutLeafGroup`.
- [x] T009 [US2] Implement grouping based on `FabricNode.status.scaleOutNics[*].lldpNeighbor`.
- [x] T010 [US2] Exclude storage nodes from scale-out leaf grouping.
- [x] T011 [US2] Maintain group health, healthy node count, total node count, node membership, and switch set status.
- [x] T012 [US2] Write and clean up Kubernetes Node leaf topology labels.

## Phase 3: RDMA Metrics

- [x] T013 [US3] Implement host RDMA device and port counter collection in `pkg/agent/rdmametrics/metrics.go`.
- [x] T014 [US3] Export RDMA counters with node, device, interface, parent interface, port, host RDMA, root interface, and NIC kind labels.
- [x] T015 [US3] Attribute RDMA metrics to RDMA-enabled Pods using `FabricNode.status.rdmaPods`.
- [x] T016 [US3] Export priority counters from ethtool where available.
- [x] T017 [US3] Include RDMA metrics dashboards and ServiceMonitor chart resources for Prometheus/Grafana integration.

## Phase 4: Documentation

- [x] T018 [P] Document FabricNode design in `docs/design/fabricnode.md`.
- [x] T019 [P] Document ScaleOutLeafGroup design in `docs/design/scaleoutleafgroup.md`.
- [x] T020 [P] Document the initial project scope in `specs/001-init/spec.md`.

## Completion Notes

- This task list records the already completed initial implementation.
- No additional topology resources, scheduling policies, remediation workflows, or non-RDMA observability features are included in this initial scope.
