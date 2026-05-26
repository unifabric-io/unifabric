# Feature Specification: Switch-Driven Scale-Out Topology Discovery

**Feature Branch**: `002-switchgroup-topology`  
**Created**: 2026-05-24  
**Status**: Draft  
**Input**: User description: "Generate a 002 spec for switch-driven scale-out topology discovery by referencing docs/design/scaleout-topology.md。"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Use Topology-Aware Scheduling in a GPU Cluster (Priority: P1)

As a GPU cluster administrator, I need Unifabric to identify leaf, spine, and core topology domains for the scale-out network and publish stable topology outputs that schedulers can consume, so that I can place workloads with topology-aware scheduling instead of relying only on direct leaf information.

**Why this priority**: This is the user-facing value of the feature. Without stable topology outputs across multiple network layers, schedulers such as Kueue still only see partial fabric structure and cannot place workloads according to the full scale-out topology.

**Independent Test**: In a GPU cluster with known node-to-leaf and switch-to-switch connectivity, refresh topology discovery and verify that participating nodes receive stable leaf, spine, and core labels while corresponding `Switch` resources expose healthy LLDP snapshots for operators to inspect.

**Acceptance Scenarios**:

1. **Given** a GPU cluster with complete node and switch connectivity for a scale-out fabric, **When** topology discovery completes, **Then** the system publishes leaf, spine, and core Node labels for participating GPU nodes and exposes the underlying switch snapshots through `Switch.status`.
2. **Given** a change in switch or node connectivity that moves a node into a different topology domain, **When** topology discovery refreshes, **Then** the affected Node labels are updated and stale topology labels are removed.
3. **Given** a topology layer that contains switches but no directly attached GPU nodes, **When** topology discovery refreshes, **Then** that layer remains part of the internal topology projection and can still contribute labels to descendant GPU nodes.

### Edge Cases

- Topology data is complete enough to identify direct leaf connectivity but not enough to determine higher layers such as spine or core.
- A topology refresh causes an existing domain to split into multiple groups or multiple groups to merge into one.
- A topology layer contains switches but currently no directly attached GPU nodes.
- Operators choose readable label values in one cluster and hash-based label values in another cluster, but both expect stable results when the underlying topology does not change.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST combine node-local scale-out connectivity and switch-level connectivity into one topology computation for the scale-out network.
- **FR-002**: The system MUST identify leaf, spine, and core topology layers for participating GPU nodes when sufficient connectivity data is available.
- **FR-003**: The system MUST compute internal topology groups that represent each detected scale-out topology domain.
- **FR-004**: Each internal topology group MUST identify its topology layer, the switches that define that domain, and any participating GPU nodes that belong directly to that layer.
- **FR-005**: The system MUST write scale-out topology labels back to Kubernetes Nodes for leaf, spine, and core using configurable label keys.
- **FR-006**: The system MUST support stable label values in either readable-name mode or short-hash mode.
- **FR-007**: When topology inputs change, the system MUST update affected topology projections and remove stale node labels.
- **FR-008**: The system MUST make topology health visible so operators can distinguish complete topology results from stale or partial topology results.
- **FR-009**: The switch-driven topology path MUST replace the legacy leaf-group CR as the target model for scale-out topology expression.
- **FR-010**: The system MUST NOT produce scale-out topology outputs from the legacy leaf-group CR.
- **FR-011**: The system MUST preserve topology domains that exist at spine or core layers even when those layers do not have directly attached GPU nodes.

### Key Entities *(include if feature involves data)*

- **FabricNode Connectivity Record**: Node-local view of scale-out NIC connectivity used as the host-side input to topology discovery.
- **Switch Connectivity Snapshot**: The latest view of switch neighbor relationships used to infer switch-to-switch and switch-to-node topology.
- **Internal Topology Group**: A computed topology domain that represents one detected leaf, spine, or core grouping and supplies stable label values.
- **Node Topology Label Set**: The leaf, spine, and core labels written to Kubernetes Nodes so schedulers and operators can consume topology results directly.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In a cluster with complete topology inputs, 100% of participating GPU nodes receive leaf, spine, and core topology labels after a successful topology refresh.
- **SC-002**: When a node changes topology domains, stale scale-out labels are removed by the next successful topology refresh.
- **SC-003**: Operators can query `Switch` resources and Node labels to inspect switch health, LLDP snapshots, and scheduler-facing topology outputs.
- **SC-004**: Re-running topology discovery without any connectivity changes produces identical label values and identical topology group names for unaffected nodes and groups.

## Assumptions

- `FabricNode` remains the node-local source of scale-out connectivity for participating GPU nodes.
- Managed switches can provide connectivity data needed to determine switch-to-switch and switch-to-node relationships.
- Consumers such as Kueue read topology from Kubernetes Node labels rather than from raw switch neighbor data.
- This feature only covers scale-out network topology and does not extend to scale-up or storage topology discovery.
