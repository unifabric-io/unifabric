# Implementation Plan: SwitchGroup Scale-Out Topology Discovery

**Branch**: `002-switchgroup-topology` | **Date**: 2026-05-24 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/002-switchgroup-topology/spec.md`

## Summary

Introduce a `Switch` and `SwitchGroup` based topology discovery path for the scale-out network. The implementation scope includes ingesting switch neighbor data, defining the `Switch` and `SwitchGroup` CRDs, computing topology from `FabricNode` and switch snapshots, writing back leaf, spine, and core Node labels, and converging the existing `ScaleOutLeafGroup` output path toward migration. The existing [docs/design/scaleout-topology.md](../../docs/design/scaleout-topology.md) serves as the design basis.

## Technical Context

**Language/Version**: Go 1.26.2  
**Primary Dependencies**: `sigs.k8s.io/controller-runtime`, `k8s.io/apimachinery`, `k8s.io/client-go`, Helm chart templates, Prometheus client library, planned `google.golang.org/grpc` support for switch subscriptions  
**Storage**: Kubernetes CRDs, CRD status fields, Kubernetes Node labels, Helm values; no separate database  
**Testing**: `make test-unit`, targeted `go test` for `./pkg/...` and `./cmd/...`, `make test-e2e`, `helm template unifabric ./chart`, `make helm-docs` when chart values or templates change  
**Target Platform**: Linux Kubernetes clusters with RDMA-capable GPU nodes and managed scale-out switches  
**Project Type**: Kubernetes controller/agent project packaged by Helm  
**Performance Goals**: After fresh node and switch inputs are available, one successful topology refresh should produce stable `SwitchGroup` outputs and Node labels for unaffected groups without churn  
**Constraints**: Scale-out only, no per-switch Kubernetes credentials, configurable Node label keys, deterministic group naming, generated CRDs via `make crd`, no long-term overlapping outputs with `ScaleOutLeafGroup`  
**Scale/Scope**: Add switch-side discovery inputs, 2 new CRDs, one controller path, chart/config updates, topology engine integration, unit tests, and e2e validation for a cluster-scale GPU fabric

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- Affected public contracts are known up front: `Switch` CRD, `SwitchGroup` CRD, `scaleOutDiscovery` Helm values, `topologyLabels.scaleOut*`, Node labels, topology metrics, controller configuration, generated CRDs, and operator-facing docs.
- Helm impact is expected. The implementation plan includes `helm template unifabric ./chart` validation and `make helm-docs` regeneration if `chart/values.yaml`, templates, or chart comments change.
- Go impact is expected. The implementation plan includes `gofmt` and focused tests around topology logic, CRD reconciliation, and controller wiring before any broad repository-wide build.
- Compatibility impact is explicit. `ScaleOutLeafGroup` becomes a deprecated output path for scale-out topology. Migration guidance must explain that the SwitchGroup path is the target model and that overlapping leaf outputs must be disabled when the new path is active.
- Constitution gate status before design: PASS.

## Project Structure

### Documentation (this feature)

```text
specs/002-switchgroup-topology/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── switch-reporter.md
│   └── topology-resources.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/
├── agent/
└── controller/

pkg/
├── api/
│   └── v1beta1/
│       ├── crd_types.go
│       ├── fabricnode_types.go
│       ├── scaleoutleafgroup_types.go
│       ├── switch_types.go                # planned
│       ├── switchgroup_types.go           # planned
│       └── zz_generated.deepcopy.go
├── controller/
│   ├── controller.go
│   ├── scaleoutgroup/
│   └── switchtopology/                    # planned
├── topology/
│   └── topology.go
└── utils/

chart/
├── values.yaml
├── crds/
│   ├── unifabric.io_fabricnodes.yaml
│   ├── unifabric.io_scaleoutleafgroups.yaml
│   ├── unifabric.io_switches.yaml         # planned
│   └── unifabric.io_switchgroups.yaml     # planned
└── templates/
    ├── ConfigMap.yaml
    ├── Deployment.yaml
    ├── Role.yaml
    └── _helpers.tpl

docs/
└── design/
    └── scaleout-topology.md
```

**Structure Decision**: Keep the existing single Go module plus Helm chart structure and do not introduce a new top-level project. Concentrate the implementation in `pkg/api/v1beta1`, `pkg/controller`, `pkg/topology`, and `chart/` so the current controller, agent, and CRD model can be extended with the smallest practical scope.

## Complexity Tracking

No constitution violations are expected for this feature.
