# Feature Specification: Project Initialization Scope

**Feature Branch**: `001-init`  
**Created**: 2026-04-27  
**Status**: Draft  
**Input**: User description: "This project is an initial implementation and only includes FabricNode, ScaleOutLeafGroup, and RDMA metrics functionality."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Discover Node RDMA Topology (Priority: P1)

As a cluster operator, I need each participating node to publish its RDMA NIC topology and LLDP neighbor state so other controllers and users can understand the RDMA fabric from Kubernetes resources.

**Why this priority**: FabricNode is the base resource used by both topology grouping and metrics attribution.

**Independent Test**: Deploy the Agent on a node with RDMA interfaces and verify that a `FabricNode` resource exists with populated node type, node IP, GPU NICs, storage NICs, LLDP neighbor data, and RDMA Pod metadata where applicable.

**Acceptance Scenarios**:

1. **Given** a node with RDMA interfaces and valid LLDP neighbors, **When** the Agent reconciles the node, **Then** the corresponding `FabricNode.status` contains discovered RDMA NIC information and reports a healthy topology.
2. **Given** a node with configured GPU and storage interface selectors, **When** topology discovery runs, **Then** matching interfaces are assigned to `scaleOutNics` and `storageNics` according to selector rules.

---

### User Story 2 - Group GPU Nodes by Scale-Out Leaf Topology (Priority: P2)

As a scheduler or platform component, I need GPU nodes with the same scale-out leaf switch set to be grouped so placement decisions can consider RDMA leaf topology.

**Why this priority**: ScaleOutLeafGroup consumes FabricNode data and provides the scheduling-facing topology grouping.

**Independent Test**: Create or observe multiple GPU `FabricNode` resources with matching `scaleOutNics[*].lldpNeighbor` switch sets and verify that they are placed into a shared `ScaleOutLeafGroup` with node labels updated.

**Acceptance Scenarios**:

1. **Given** two GPU nodes connected to the same leaf switch set, **When** the controller reconciles their `FabricNode` resources, **Then** both nodes appear in the same `ScaleOutLeafGroup`.
2. **Given** a node's leaf switch set changes, **When** reconciliation runs, **Then** the node is moved to the correct group and its Kubernetes Node topology label is updated.

---

### User Story 3 - Collect RDMA Metrics (Priority: P3)

As an operator, I need RDMA device, port, priority, and Pod-attributed metrics to be exported so I can monitor node and workload RDMA behavior.

**Why this priority**: Metrics make the discovered RDMA environment observable after topology resources exist.

**Independent Test**: Scrape the Agent metrics endpoint and verify that RDMA counters include node, device, interface, parent interface, NIC kind, Pod, owner, host RDMA, and root interface labels where applicable.

**Acceptance Scenarios**:

1. **Given** a node with RDMA devices, **When** metrics are scraped, **Then** RDMA port counters and interface properties are exported with node and interface labels.
2. **Given** an RDMA-enabled Pod, **When** metrics are scraped, **Then** the exported metrics include Pod and top-level owner labels for attribution.

### Edge Cases

- Nodes without RDMA interfaces must not publish misleading topology or healthy RDMA state.
- RDMA interfaces without LLDP neighbors must be visible and reflected in topology health.
- Storage nodes must not participate in scale-out leaf grouping.
- Missing or stale RDMA Pod container metadata must not prevent host RDMA metrics from being exported.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST maintain a `FabricNode` resource per participating node.
- **FR-002**: The system MUST discover RDMA-capable host interfaces and publish their interface name, RDMA device name, link state, IP addresses, and LLDP neighbor information.
- **FR-003**: The system MUST classify RDMA NICs into `scaleOutNics` and `storageNics` using configured topology interface selectors.
- **FR-004**: The system MUST record RDMA-enabled Pods on each non-storage node, including namespace, name, container IDs, host RDMA mode, and top-level owner metadata.
- **FR-005**: The system MUST create and maintain `ScaleOutLeafGroup` resources from `FabricNode.status.scaleOutNics` LLDP neighbor data.
- **FR-006**: The system MUST skip storage nodes when computing scale-out leaf groups.
- **FR-007**: The system MUST write the scale-out leaf group label back to Kubernetes Nodes and remove stale labels when nodes leave a group.
- **FR-008**: The system MUST export RDMA metrics for host RDMA devices and RDMA-enabled Pods.
- **FR-009**: RDMA metrics MUST include labels sufficient to identify node, RDMA device, interface, parent interface, NIC kind, Pod, owner, host RDMA mode, and root interface status where applicable.
- **FR-010**: The initial project scope MUST NOT include additional topology resources, scheduling policies, remediation workflows, or non-RDMA observability features.

### Key Entities

- **FabricNode**: Cluster-scoped node network state containing RDMA topology, LLDP neighbors, RDMA Pod metadata, node type, and node IP.
- **ScaleOutLeafGroup**: Cluster-scoped grouping of GPU nodes that share the same scale-out leaf switch set.
- **RDMA Metrics**: Prometheus metrics exported by the Agent for RDMA counters, interface properties, priority counters, and Pod attribution.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A deployed Agent creates or updates one `FabricNode` for its node and keeps status current after topology changes.
- **SC-002**: Scale-out and storage RDMA NICs are classified according to configured selectors without overlapping storage NICs into scale-out topology.
- **SC-003**: Nodes with identical GPU leaf switch sets are grouped into the same `ScaleOutLeafGroup`.
- **SC-004**: Storage nodes are excluded from `ScaleOutLeafGroup` membership.
- **SC-005**: RDMA metrics can be scraped and queried by node, device, interface, NIC kind, and Pod attribution labels.

## Assumptions

- This is the initial project specification and intentionally documents only the implemented core scope.
- Kubernetes CRDs are the API boundary for topology state.
- Prometheus-compatible scraping is the metrics integration boundary.
- GPU, storage, and RDMA terminology follows the existing project API and Helm values.
