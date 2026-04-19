# Unifabric Constitution

## Core Principles

### I. Kubernetes-Native Topology Contracts
Unifabric MUST model discovered RDMA and fabric topology through Kubernetes APIs,
labels, and controller-managed status. CRDs, Node labels, and controller config
fields are public contracts: changes MUST preserve clear ownership, defaulting,
and reconciliation behavior. Label keys written to Nodes MUST be configurable
through Helm values and controller config, with defaults kept consistent across
code, chart values, rendered ConfigMaps, and documentation.

### II. Helm Chart Consistency
The Helm chart is the supported installation interface. Any change to
`chart/values.yaml`, chart templates, chart CRDs, or chart value comments MUST
keep `chart/README.md` generated from those sources. After modifying Helm chart
files, contributors MUST run `helm-docs` and include the regenerated
`chart/README.md` when it changes. Rendered manifests MUST be validated with
`helm template` for chart-affecting changes.

### III. Observable RDMA Behavior
Agent and controller behavior MUST remain observable through structured logs,
Kubernetes Events where useful, CR status, and metrics for RDMA devices,
interfaces, Pods, Nodes, and switches. New topology or metrics behavior MUST
define how operators can verify the discovered state and diagnose stale,
missing, or unhealthy topology information.

### IV. Testable Controller and Agent Changes
Changes to reconciliation logic, config defaulting, topology discovery, or
metrics classification MUST include focused automated tests when behavior can
be exercised without a live cluster. Chart changes MUST be covered by at least
Helm rendering validation. Tests MUST protect public behavior rather than only
implementation details.

### V. Minimal, Compatible Evolution
New abstractions, config fields, and generated assets MUST follow existing
project patterns and be scoped to the requested behavior. Backward-incompatible
changes to CRDs, Helm values, labels, metrics, or controller config MUST be
called out in specs and plans with migration guidance. Generated files MUST be
updated by the owning generator instead of hand-edited when a generator exists.

## Project Constraints

Unifabric targets Kubernetes clusters running Linux nodes with RDMA-capable
networking. The repository uses Go for controller and agent code, Helm for
packaging, Kubernetes CRDs for topology resources, and generated artifacts for
CRDs, deepcopy code, dashboards, and Helm values documentation where applicable.
Feature work MUST respect these boundaries unless a plan explicitly justifies a
change to the project architecture.

## Development Workflow

1. `make crd` if `pkg/api/v1beta1` changed
1. `make build` build bin
2. `make test-unit` for unit test check
3. `make helm-docs` if chart/* changed

## Governance

This constitution supersedes informal project practices. Specs, plans, tasks,
reviews, and implementation work MUST check compliance with these principles.
Amendments require updating this file, recording the version and date, and
propagating any changed rules to Spec Kit templates and runtime guidance.

Versioning follows semantic versioning:

- MAJOR for incompatible governance or principle removals/redefinitions.
- MINOR for new principles, new required workflow gates, or materially expanded
  guidance.
- PATCH for clarifications that do not change required behavior.

Compliance review is required before implementation planning completes and
again before final delivery. Any accepted violation MUST be documented with the
simpler alternative considered and the reason it was rejected.

**Version**: 1.0.0 | **Ratified**: 2026-04-28 | **Last Amended**: 2026-04-28
