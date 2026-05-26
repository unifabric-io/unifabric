# Feature Specification: Project Initialization Scope

**Feature Branch**: `001-init`  
**Created**: 2026-04-27  
**Status**: Draft  
**Input**: User description: "This project is an initial implementation and only includes FabricNode, switch-driven scale-out topology discovery, and RDMA metrics functionality."

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

**Why this priority**: Switch-driven topology consumes FabricNode and Switch data and provides the scheduling-facing topology labels.

**Independent Test**: Create or observe multiple GPU `FabricNode` resources and related `Switch` resources with matching scale-out connectivity and verify that the controller writes stable leaf, spine, and core node labels.

**Acceptance Scenarios**:

1. **Given** two GPU nodes connected to the same scale-out topology domain, **When** the controller reconciles `FabricNode` and `Switch` resources, **Then** both nodes receive the same relevant scale-out topology labels.
2. **Given** a node's switch connectivity changes, **When** reconciliation runs, **Then** the node receives the correct topology labels and stale labels are removed.

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
- Storage nodes must not participate in scale-out topology labeling.
- Missing or stale RDMA Pod container metadata must not prevent host RDMA metrics from being exported.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST maintain a `FabricNode` resource per participating node.
- **FR-002**: The system MUST discover RDMA-capable host interfaces and publish their interface name, RDMA device name, link state, IP addresses, and LLDP neighbor information.
- **FR-003**: The system MUST classify RDMA NICs into `scaleOutNics` and `storageNics` using configured topology interface selectors.
- **FR-004**: The system MUST record RDMA-enabled Pods on each non-storage node, including namespace, name, container IDs, host RDMA mode, and top-level owner metadata.
- **FR-005**: The system MUST combine `FabricNode.status.scaleOutNics` LLDP data with `Switch.status` LLDP snapshots to compute scale-out topology.
- **FR-006**: The system MUST skip storage nodes when computing scale-out topology labels.
- **FR-007**: The system MUST write scale-out leaf, spine, and core labels back to Kubernetes Nodes and remove stale labels when nodes leave a topology domain.
- **FR-008**: The system MUST export RDMA metrics for host RDMA devices and RDMA-enabled Pods.
- **FR-009**: RDMA metrics MUST include labels sufficient to identify node, RDMA device, interface, parent interface, NIC kind, Pod, owner, host RDMA mode, and root interface status where applicable.
- **FR-010**: The initial project scope MUST NOT include additional topology resources, scheduling policies, remediation workflows, or non-RDMA observability features.

### Key Entities

- **FabricNode**: Cluster-scoped node network state containing RDMA topology, LLDP neighbors, RDMA Pod metadata, node type, and node IP.
- **Switch**: Cluster-scoped switch endpoint and status resource that exposes switch-side LLDP snapshots for scale-out topology discovery.
- **Node Topology Labels**: Kubernetes Node labels that publish computed scale-out leaf, spine, and core topology domains.
- **RDMA Metrics**: Prometheus metrics exported by the Agent for RDMA counters, interface properties, priority counters, and Pod attribution.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A deployed Agent creates or updates one `FabricNode` for its node and keeps status current after topology changes.
- **SC-002**: Scale-out and storage RDMA NICs are classified according to configured selectors without overlapping storage NICs into scale-out topology.
- **SC-003**: Nodes in the same computed scale-out topology domains receive matching leaf, spine, and core labels.
- **SC-004**: Storage nodes are excluded from scale-out topology labels.
- **SC-005**: RDMA metrics can be scraped and queried by node, device, interface, NIC kind, and Pod attribution labels.

## Assumptions

- This is the initial project specification and intentionally documents only the implemented core scope.
- Kubernetes CRDs are the API boundary for topology state.
- Prometheus-compatible scraping is the metrics integration boundary.
- GPU, storage, and RDMA terminology follows the existing project API and Helm values.
