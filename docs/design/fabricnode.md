# FabricNode Design

中文版本：[fabricnode.zh.md](./fabricnode.zh.md)

## Summary

`FabricNode` is the node-local network state resource of Unifabric. The Agent on each
Kubernetes Node maintains one `FabricNode` and writes the discovered RDMA topology,
LLDP neighbor data, RDMA Pod metadata, node role, and node IP into `status`.
Controllers and external consumers read `FabricNode` instead of scraping host network
state directly. This keeps discovery local to each node while exposing one
cluster-level API. The current field reference is
[FabricNode reference](../reference/fabricnode.md).

## Motivation

Topology discovery and RDMA metrics both need per-node facts: which interfaces are
RDMA capable, which switch each interface connects to, and which Pods use RDMA. These
facts only exist on the host, in netlink, `lldpcli`, and the container runtime. Before
this design, no cluster API carried them. A controller that wanted node-side LLDP data
had to run privileged probes against every host itself, and the metrics collector
had no stable source for Pod and workload attribution.

The design centralizes host probing in one Agent per node and publishes the result as
a typed, cluster-scoped resource. The Controller consumes `FabricNode` as the node-side
input for topology computation (see [Scale-out topology design](./scaleout-topology.md)
and [Topology CRD design](./topology-crd.md)). The metrics collector consumes the same
data in memory (see [RDMA metrics design](./rdma-metrics.md)).

User stories:

- As a Controller developer, I want a typed per-node resource with NIC and LLDP data,
  so topology computation reads the API server instead of host state.
- As an observability engineer, I want the Agent to record which Pods use RDMA, so
  metrics carry Pod and workload owner labels.
- As a cluster operator, I want to select which interfaces belong to the scale-out and
  storage fabrics, so mixed-NIC hosts report the correct topology lists.

### Goals

- Publish one cluster-scoped `FabricNode` per Node, owned and written only by the
  local Agent.
- Discover RDMA-capable interfaces, their addresses, link state, and LLDP neighbors.
- Classify interfaces into scale-out and storage lists through configurable selectors.
- Record RDMA-enabled Pods with container IDs and top-level workload owner metadata.
- Report node role, node IP, and readiness conditions.

### Non-Goals

- `FabricNode` does not compute topology. The Controller derives domains and labels
  from it.
- `FabricNode` does not store metrics samples. The collector reads the in-memory
  snapshot.
- Users do not edit `FabricNode`. The resource has an empty `spec` and the Agent owns
  the status.
- Scale-up NIC reporting is out of scope in this release. The scale-up selector is
  validated and excludes interfaces from `scaleOutNics`, but writes no status field.

## Proposal

`FabricNode` is a cluster-scoped resource with an empty `spec`. The Agent is the only
writer. Each reconciliation scans host state, builds the desired status, and writes it
only when the observed state changes. The Agent also keeps a local deep copy as the
snapshot for the RDMA metrics collector, so a scrape never calls the API server.

```yaml
apiVersion: unifabric.io/v1beta1
kind: FabricNode
metadata:
  name: node-gpu-1
status:
  conditions:
    - type: Ready
      status: "True"
      reason: Ready
      message: All FabricNode conditions are ready
    - type: LLDPNeighborsReady
      status: "True"
      reason: LLDPNeighborsReady
      message: All selected RDMA interfaces have LLDP neighbors
  totalNics: 4
  nodeRole: GPU
  nodeIP: 192.168.200.6
  scaleOutNics:
    - name: eth1
      rdma: true
      rdmaDeviceName: mlx5_0
      state: up
      ipv4: 172.17.1.11/24
      ipv6: ""
      lldpNeighbor:
        hostname: gpu-leaf-1
        mgmtIP: 10.10.10.1
        mac: 00:11:22:33:44:55
        port: Ethernet1/1
        description: GPU leaf switch
    - name: eth2
      rdma: true
      rdmaDeviceName: mlx5_1
      state: up
      ipv4: 172.17.2.11/24
      ipv6: ""
      lldpNeighbor:
        hostname: gpu-leaf-2
        mgmtIP: 10.10.10.2
        mac: 00:11:22:33:44:66
        port: Ethernet1/1
        description: GPU leaf switch
  storageNics:
    - name: eth9
      rdma: true
      rdmaDeviceName: mlx5_8
      state: up
      ipv4: 172.18.9.11/24
      ipv6: ""
      lldpNeighbor:
        hostname: storage-leaf-1
        mgmtIP: 10.10.20.1
        mac: 00:11:22:33:55:55
        port: Ethernet9/1
        description: Storage leaf switch
    - name: eth10
      rdma: true
      rdmaDeviceName: mlx5_9
      state: up
      ipv4: 172.18.10.11/24
      ipv6: ""
      lldpNeighbor:
        hostname: storage-leaf-2
        mgmtIP: 10.10.20.2
        mac: 00:11:22:33:55:66
        port: Ethernet9/1
        description: Storage leaf switch
  rdmaPods:
    - namespace: training
      name: worker-0
      containerList:
        - containerd://8f3e2d3a2f5b6c7d
      hostRDMA: false
      topOwner:
        apiVersion: kubeflow.org/v1
        kind: PyTorchJob
        namespace: training
        name: resnet
```

The main status groups are:

- `conditions`: Kubernetes-style conditions. `LLDPNeighborsReady` reports whether all
  selected RDMA interfaces in the `up` state have LLDP neighbor data. `Ready` is the
  aggregate and is true only when all other conditions are true.
- `totalNics`: the number of NICs in the scale-out and storage topology lists.
- `scaleOutNics` and `storageNics`: RDMA interfaces selected for each fabric. Each
  entry carries the interface name, RDMA device name, IPv4 and IPv6 CIDRs, link state,
  and the LLDP neighbor hostname, management IP, MAC, port, and description.
- `rdmaPods`: RDMA-enabled Pods on this node, with container IDs, host RDMA mode, and
  the top-level owner reference.
- `nodeRole`: `GPU` or `Storage`, as reported by the Agent.
- `nodeIP`: the node IP detected by opening a UDP socket to the configured probe
  address.

## Detailed Design

### Reconcile flow

The Agent reconciles `FabricNode` in `pkg/agent/fabricnode`. On startup, the Agent:

1. Resolves its node name from configuration, `NODE_NAME`, or `/etc/hostname`.
2. Creates a controller-runtime manager and limits the Pod cache to the current node.
3. Starts a periodic Reconcile loop.
4. Watches local Pods and triggers Reconcile when Pod state changes.

During each Reconcile, the Agent:

1. Creates the `FabricNode` if it does not exist.
2. Updates topology status after `initialScanDelay`.
3. Updates RDMA Pod status for non-storage Agents.
4. Updates node role and node IP.
5. Writes status only when the observed state changes.
6. Saves a local deep copy for the metrics collector.

Storage nodes delete their own `FabricNode` when the Agent exits. A stopped storage
node therefore leaves no stale reported state in the cluster.

### NIC topology discovery

Topology discovery scans host network interfaces through netlink:

1. List links with `netlink.LinkList`.
2. Keep only links with type `device`.
3. Skip SR-IOV VFs.
4. Keep only interfaces that map to RDMA devices through `rdmamap`.
5. Read IPv4 and IPv6 addresses from netlink.
6. Read LLDP neighbors through `lldpcli`.
7. Build `NicInfo`.
8. Apply the topology selectors and update the matching status lists.

`LLDPNeighborsReady` is false when any selected RDMA interface in the `up` state has
no LLDP neighbor data. A node with no selected RDMA interfaces is valid: it reports
`totalNics: 0` and does not fail the condition. Connected switches must have LLDP
enabled, and the host must be able to read LLDP data through `lldpcli`.

### RDMA Pod status

The Agent records Pods that use RDMA so metrics can carry Pod and workload owner
labels. A Pod is RDMA-enabled when either condition is true:

- It uses an allocated SR-IOV RDMA device.
- It runs in host RDMA mode.

For each matching Pod, the Agent records the namespace and name, the container IDs,
the host RDMA flag, and the top-level owner reference. The RDMA metrics collector
reads the latest in-memory snapshot, uses `scaleOutNics` and `storageNics` to label
interfaces with a `kind` value, and uses `rdmaPods` to attribute host and container
namespace counters to the correct objects.

### Interface selectors

The `fabricNode` section of the Helm values configures the selectors. Supported
selector forms are:

- `interface=eth*,!eth9`
- `cidr=172.17.0.0/16`

| Configuration | Written field | Behavior |
| --- | --- | --- |
| `scaleOutInterfaceSelector` | `status.scaleOutNics` | When empty, includes all RDMA interfaces not matched as storage or scale-up. |
| `storageInterfaceSelector` | `status.storageNics` | Matched interfaces are excluded from `scaleOutNics`. |
| `scaleUpInterfaceSelector` | none | Reserved. Validated and excluded from `scaleOutNics`, but writes no status field in this release. |

### Upgrade compatibility

The Agent is the only writer, and the write is a full desired-state status update that
skips identical states. A new Agent version rebuilds the same status from host state,
so no migration step is needed. Consumers that tolerate absent optional fields keep
working across versions.

## Alternatives

The alternatives below are reconstructed from the decisions this design argues
against. They were not recorded at decision time.

- Controllers probe host state directly. Rejected because every consumer then
  needs privileged host access on every node, and the probes are duplicated. One Agent
  per node centralizes privilege and publishes a typed API.
- A namespaced resource or a `spec` that users edit. Rejected because the data is
  per-Node observed state with exactly one natural writer. An empty `spec` and
  Agent-owned `status` remove write conflicts.
- Keep stale `FabricNode` objects for stopped storage nodes. Rejected because a stale
  storage entry misleads topology computation. Storage Agents delete their own
  resource on exit.
- A cluster-wide Pod cache in the Agent. Rejected because the Agent only needs Pods on
  its own node. Limiting the cache bounds memory and API load.

## Implementation History

- 2026-04-19: Initial implementation with RDMA topology and metrics support.
- 2026-07-17: Updated together with topology status aggregation and automatic label
  discovery.
- 2026-10-13: Document restructured into the design template. The Alternatives section
  is a post-hoc reconstruction.
