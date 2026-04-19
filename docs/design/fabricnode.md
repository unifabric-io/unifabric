1. ## Purpose

`FabricNode` is the node-local network state resource. Each Agent maintains one `FabricNode` resource for its Kubernetes Node and writes the discovered RDMA topology, LLDP neighbor data, RDMA Pod metadata on the node, node type, and node IP into `status`.

Controllers and external consumers read `FabricNode` instead of scraping host network state directly. This keeps topology discovery local to each node while exposing a cluster-level API.

1. ## Resource Example

`FabricNode` is a cluster-scoped resource with an empty `spec`. The resource is fully owned and maintained by the Agent through `status`.

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
  totalNics: 3
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

- `conditions`: Kubernetes-style conditions for node status. `LLDPNeighborsReady` reports whether selected RDMA interfaces in the `up` state have LLDP neighbor data. `Ready` is the aggregate status and is true only when all other FabricNode conditions are true.
- `totalNics`: The number of topology NICs used by the current scale-out and storage topology lists.
- `scaleOutNics`: RDMA interfaces selected for the scale-out topology.
- `storageNics`: RDMA interfaces selected for the storage topology.
- `rdmaPods`: RDMA-enabled Pods running on this node, including container IDs and top-level owner metadata.
- `nodeRole`: The node role reported by the Agent, currently `GPU` or `Storage`.
- `nodeIP`: The node IP detected by opening a UDP socket to the configured probe address.

Each NIC entry contains:

- Interface name
- RDMA device name
- IPv4 and IPv6 CIDRs
- Link state
- LLDP neighbor hostname, management IP, MAC, port, and description

1. ## Reconcile Flow

1. ### Resource Creation

The Agent reconciles `FabricNode` in `pkg/agent/fabricnode`.

On startup, the Agent:

1. Resolves its node name from configuration, `NODE_NAME`, or `/etc/hostname`.
2. Creates a controller-runtime manager and limits the Pod cache to the current node.
3. Starts a periodic Reconcile loop.
4. Watches local Pods and triggers Reconcile when Pod state changes.

During each Reconcile, the Agent:

1. Creates the `FabricNode` if it does not exist.
2. Updates topology status after `initialScanDelay`.
3. Updates RDMA Pod status for non-storage Agents.
4. Updates node type and node IP.
5. Writes status only when the observed state changes.
6. Saves a local deep copy for the metrics collector.

Storage nodes delete their own `FabricNode` when the Agent exits, preventing stopped storage nodes from leaving stale reported state in the cluster.

1. ### NIC Network Topology Discovery

Topology discovery scans host network interfaces through netlink:

1. List links with `netlink.LinkList`.
2. Keep only links with type `device`.
3. Skip SR-IOV VFs.
4. Keep only interfaces that can be mapped to RDMA devices through `rdmamap`.
5. Read IPv4 and IPv6 addresses from netlink.
6. Read LLDP neighbors through `lldpcli`.
7. Build `NicInfo`.
8. Apply topology selectors and update the matching status lists.

The expected behavior is that `LLDPNeighborsReady` is false if any selected RDMA interface in the `up` state is missing LLDP neighbor data. Nodes with no selected RDMA interfaces are valid; they report `totalNics: 0` and do not fail the LLDP neighbor condition just because no RDMA interfaces were selected.

1. ### RDMA Pod Status

The Agent records Pods that use RDMA so metrics can be labeled with Pod and workload owner information.

A Pod is considered RDMA-enabled when either of the following is true:

- It uses an allocated SR-IOV RDMA device; or
- It runs in host RDMA mode.

For each matching Pod, the Agent records:

- Namespace and name
- Container IDs
- Whether the Pod uses host RDMA mode
- Top-level owner reference

The RDMA metrics collector reads the latest in-memory `FabricNode` snapshot, uses `scaleOutNics` and `storageNics` to label interfaces with a `kind` value, and uses `rdmaPods` to attribute host and container namespace counters to the corresponding objects.

1. ## Optional Configuration

Configure these settings under `nodeTopologyDiscovery` in Helm values.

Supported formats:

- `interface=eth*,!eth9`
- `cidr=172.17.0.0/16`

Selector behavior:

- `scaleOutInterfaceSelector`: Selects interfaces for `status.scaleOutNics`. If empty, all RDMA interfaces not matched as storage or scale-up are included in `scaleOutNics`.
- `storageInterfaceSelector`: Selects interfaces for `status.storageNics`. Interfaces matched as storage are excluded from `scaleOutNics`.
- `scaleUpInterfaceSelector`: Reserved scale-up selector. It is validated and excluded from `scaleOutNics`, but does not write a FabricNode status field in this release.

| Configuration | Written field | Description |
| --- | --- | --- |
| `scaleOutInterfaceSelector` | `status.scaleOutNics` | Includes all non-storage, non-scale-up RDMA interfaces when empty |
| `storageInterfaceSelector` | `status.storageNics` | Excludes matched interfaces from `scaleOutNics` |
| `scaleUpInterfaceSelector` | N/A | Reserved for future scale-up discovery; excludes matched interfaces from `scaleOutNics` |

## Additional Notes

- Connected switches must have LLDP enabled, and the local host must be able to obtain LLDP information through `lldpcli`.
