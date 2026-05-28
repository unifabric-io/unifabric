# Feature Specification: sFlow Processing and Workload Attribution

**Feature Branch**: `003-sflow-processing`
**Created**: 2026-05-28
**Status**: Draft
**Input**: User description: "Implement sFlow processing: provide a receiving endpoint for switch-pushed sFlow data, add Pod and Workload information to the sFlow records, and write the resulting data to ClickHouse. The dashboards have already been added at chart/files/switch-sflow.json and chart/files/workload-sflow.json."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Observe Switch sFlow by Kubernetes Workload (Priority: P1)

As a cluster operator, I need switch-exported sFlow traffic to be collected, enriched with Kubernetes Pod and workload ownership information, and made available for switch and workload traffic dashboards so I can identify which workloads are responsible for observed network traffic.

**Why this priority**: This is the complete user value path for the feature: raw switch flow samples are not actionable until they can be attributed to Pods, nodes, and top-level workloads and queried by the provided dashboards.

**Independent Test**: Configure a switch to send sFlow samples for traffic involving known cluster Pods, then verify that the resulting records can be queried by switch, source workload, destination workload, source Pod, destination Pod, source node, destination node, protocol, byte count, packet count, and sample count.

**Acceptance Scenarios**:

1. **Given** the system is configured to accept sFlow data and a switch sends valid flow samples for traffic between two known Pods, **When** the samples are processed, **Then** the stored records include flow timing, switch sampler identity, source and destination addresses, protocol and port information, sampled bytes and packets, and source and destination Pod, node, and workload owner fields.
2. **Given** a valid sFlow sample references an address that cannot be matched to any current Pod, **When** the sample is processed, **Then** the stored record keeps the original flow fields and leaves the unmatched Kubernetes attribution fields empty without dropping the record.
3. **Given** multiple switches send sFlow samples for the same selected time range, **When** an operator views switch and workload traffic dashboards, **Then** the dashboards can show observed traffic, observed workloads, observed Pods, flow samples, top source and destination workloads, top source and destination Pods, and top protocols.
4. **Given** the system receives malformed or unsupported sFlow data, **When** the data is processed, **Then** invalid samples are rejected or skipped without interrupting valid sFlow ingestion from the same or other switches.

### Edge Cases

- sFlow datagrams may contain multiple flow records; each valid record must be represented independently for later analysis.
- Switches may report IPv4 or IPv6 traffic, and either address family may appear as source, destination, or sampler identity.
- Samples may omit transport ports, autonomous system data, or packet lengths; missing optional values must not prevent useful flow records from being stored.
- Pod ownership can change as workloads roll out; records must reflect the best attribution available at processing time and must not mutate historical records after they are stored.
- A Pod IP may be stale, reused, host-networked, or recently deleted; attribution must avoid assigning traffic to an unrelated current Pod when there is insufficient confidence.
- Destination traffic may be outside the cluster; source-side attribution must still be preserved when available.
- The receiving path may experience short bursts; the system must prioritize continuing ingestion and provide a clear indication when records cannot be accepted or stored.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST expose a configurable network receiving endpoint for switch-pushed sFlow data.
- **FR-002**: The system MUST accept sFlow datagrams from multiple switches without requiring a separate receiver per switch.
- **FR-003**: The system MUST decode valid sFlow flow samples into normalized flow records containing receive time, flow time, sequence number, sampling rate, sampler identity, source and destination addresses, protocol, source and destination ports, byte count, packet count, and available routing metadata.
- **FR-004**: The system MUST support IPv4 and IPv6 addresses for sampler, source, and destination fields.
- **FR-005**: The system MUST enrich each normalized flow record with Kubernetes attribution for both source and destination when a match is available, including Pod name, Pod namespace, node name, top-level workload kind, top-level workload name, and top-level workload namespace.
- **FR-006**: The system MUST leave attribution fields empty for unmatched sides of a flow while preserving and storing the original flow information.
- **FR-007**: The system MUST make enriched records available to the existing switch and workload traffic dashboards using the fields needed for switch selection, workload selection, source and destination Pod ranking, source and destination workload ranking, protocol ranking, traffic totals, packet totals, and sample counts.
- **FR-008**: The system MUST store flow records in a queryable analytical history with enough time precision and retention to support recent operational troubleshooting.
- **FR-009**: The system MUST apply the sFlow sampling rate when records are queried or summarized so observed bytes and packets can be estimated from sampled values.
- **FR-010**: The system MUST continue processing valid records when a datagram contains malformed, unsupported, or partially parseable records.
- **FR-011**: The system MUST report ingestion and storage health in a way operators can use to detect decode failures, rejected records, storage write failures, and overload conditions.
- **FR-012**: The system MUST allow operators to configure the receiving endpoint, the destination analytical store connection, write batching behavior, and flow retention settings.
- **FR-013**: The system MUST NOT require the example sFlow collector source directory to be part of the committed project scope.

### Key Entities

- **sFlow Datagram**: A switch-sent network telemetry packet containing one or more sampled traffic records and metadata about the exporting switch.
- **Normalized Flow Record**: A single decoded traffic observation with sampler, timing, address, protocol, port, byte, packet, sequence, and sampling-rate fields.
- **Kubernetes Endpoint Attribution**: The best available mapping from a flow endpoint address to Pod namespace, Pod name, node name, and top-level workload owner.
- **Workload Owner**: The top-level Kubernetes workload identity used to group traffic across Pods belonging to the same application or job.
- **Switch Traffic View**: Operator-facing view of traffic grouped by switch, workload, Pod, protocol, and time range.
- **Workload Traffic View**: Operator-facing view of traffic for a selected workload grouped by switch, Pod, direction, and time range.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In a controlled test with valid sFlow samples for known Pods, at least 95% of records whose endpoint addresses match active Pods include the correct Pod, node, and top-level workload attribution for the matched side.
- **SC-002**: Operators can select a switch and see observed traffic, observed workloads, observed Pods, flow sample count, top workloads, top Pods, and top protocols for a selected recent time range.
- **SC-003**: Operators can select a workload and see transmit and receive traffic by switch plus top Pods for a selected recent time range.
- **SC-004**: Invalid or unsupported sFlow samples do not stop ingestion of valid samples; valid samples received after malformed input are still stored and queryable.
- **SC-005**: During a sustained expected ingest rate, newly received valid records become available for dashboard queries within 30 seconds for at least 95% of records.
- **SC-006**: Unmatched flow endpoints are stored with empty attribution fields and remain queryable by sampler, address, protocol, and time.

## Assumptions

- Switches are already configured or can be configured by operators to push sFlow data to the receiving endpoint.
- Existing switch and workload dashboards define the required query surface for this feature.
- The analytical history schema already includes the flow and Kubernetes attribution fields needed by the dashboards.
- Kubernetes Pod IP and owner metadata are available to the system at processing time for attribution.
- Historical flow records preserve attribution as it was known when each record was processed.
- The example collector source directory is reference material only and is outside the committed project scope.
