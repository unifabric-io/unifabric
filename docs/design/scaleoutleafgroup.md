## Current status

`ScaleOutLeafGroup` is now a legacy scale-out topology output. It still documents the historical leaf-only grouping algorithm, but the target model for scale-out topology is the switch-driven path that consumes `Switch` status and writes Node labels.

Use this document only when:

- the cluster still runs the leaf-only path with `scaleOutDiscovery.leafGroups.enabled=true`
- switch-driven discovery is not enabled yet

When switch-driven discovery is enabled, set `scaleOutDiscovery.switches.enabled=true` and `scaleOutDiscovery.leafGroups.enabled=false`. In that mode the Controller should stop writing overlapping `ScaleOutLeafGroup` outputs for the same nodes.

## Purpose

`ScaleOutLeafGroup` is the leaf switch grouping resource for the scale-out dimension. The Controller reads LLDP neighbor information for RDMA NICs from `FabricNode.status.scaleOutNics` and places compute nodes with the same set of leaf switches into the same `ScaleOutLeafGroup`.

The Controller also writes the grouping result back to Kubernetes Nodes as topology labels, allowing schedulers and external systems to make placement decisions based on leaf topology.

## Resource Example

`ScaleOutLeafGroup` is a cluster-scoped resource with an empty `spec`. The Controller automatically creates, updates, or deletes this resource based on `FabricNode` status.

```yaml
apiVersion: unifabric.io/v1beta1
kind: ScaleOutLeafGroup
metadata:
  name: 6f8a91c2
status:
  healthy: true
  healthyNodes: 2
  totalNodes: 2
  nodes:
    - name: node-gpu-1
      healthy: true
    - name: node-gpu-2
      healthy: true
  switches:
    - name: gpu-leaf-1
      mgmtIP: 10.10.10.1
    - name: gpu-leaf-2
      mgmtIP: 10.10.10.2
```

- `healthy`: True when every node in the group fully matches the group's leaf switch set.
- `healthyNodes`: Number of healthy nodes.
- `totalNodes`: Total number of nodes in the group.
- `nodes`: The list of nodes in the group and each node's health state within the group.
- `switches`: The set of leaf switches that defines the group.

The `ScaleOutLeafGroup` resource name is generated as a short hash from the sorted switch names. This gives the same set of leaf switches a stable group name while avoiding overly long Kubernetes resource names.

## Reconcile Flow

### Resource Watches

The Controller reconciles `ScaleOutLeafGroup` in `pkg/controller/scaleoutgroup`.

On startup, the Controller:

1. Registers the `ScaleOutLeafGroup` reconciler.
2. Watches create, delete, and status changes for all `FabricNode` resources.
3. Builds an index for `FabricNode.metadata.name`.
4. Performs a full sync during the first Reconcile, then incrementally syncs individual node changes.

During each Reconcile, the Controller:

1. Lists existing `ScaleOutLeafGroup` resources.
2. Gets the `FabricNode` that triggered the Reconcile.
3. Skips nodes whose `status.nodeRole` is `Storage`.
4. Extracts the set of leaf switches connected to the current node from `FabricNode.status.scaleOutNics`.
5. Creates, updates, migrates, or deletes groups based on the switch set.
6. Updates the leaf topology label on the Kubernetes Node.

If a `FabricNode` is deleted, the Controller removes the corresponding node from any existing group and cleans up the leaf topology label on that Kubernetes Node.

### Leaf Switch Extraction

The Controller only consumes `FabricNode.status.scaleOutNics`. `storageNics` is not used for scale-out leaf grouping.

When extracting leaf switches from `scaleOutNics`, only NICs that meet the following conditions are kept:

- The NIC state is `up`
- `lldpNeighbor.hostname` is not empty

After extraction, the Controller:

1. Deduplicates by switch name.
2. Keeps the switch name and management IP.
3. Sorts by switch name.

The sorted switch set is the topology fingerprint used to place the node into a scale-out group.

### Grouping Algorithm

When a node's leaf switch set changes, the Controller compares that set with existing groups.

Main scenarios:

1. The node currently does not belong to any group, and no matching group exists.
   - Create a new group from the node's switch set.
   - Add the node to the new group and mark it healthy.
   - Write the leaf topology label to the Kubernetes Node.

2. The node currently does not belong to any group, but a matching group exists.
   - Add the node to the existing group.
   - Mark the node healthy only when its switch set exactly matches the group's switch set.

3. The node already belongs to a group, but the current switch set no longer matches any group.
   - Remove the node from the old group.
   - Clean up the leaf topology label on the Kubernetes Node.
   - Delete the old group if it has no remaining nodes.

4. The node migrates from one group to another.
   - Remove the node from the old group.
   - Add the node to the target group.
   - Update the leaf topology label on the Kubernetes Node.

5. The node remains in the same group.
   - Refresh the node's health state.
   - Recalculate `healthy`, `healthyNodes`, and `totalNodes`.
   - Refresh the leaf topology label on the Kubernetes Node.

### Health Calculation

A node's health state within a group is determined by whether its leaf switch set matches exactly:

- Healthy node: The current switch set exactly matches the group's `status.switches`.
- Unhealthy node: The current switch set only partially matches the group.

The group's `status.healthy` is true only when every node in `status.nodes` is healthy.

This preserves visibility for nodes with partially degraded topology while preventing the whole group from being incorrectly marked healthy.

## Migration guidance

Clusters that move to the switch-driven path should migrate in this order.

1. Deploy switch-agent to the managed scale-out switches and make sure each switch has a matching `Switch` resource.
2. Enable `scaleOutDiscovery.switches.enabled=true` in the Controller configuration.
3. Disable `scaleOutDiscovery.leafGroups.enabled` so `ScaleOutLeafGroup` no longer owns the scale-out leaf label.
4. Verify that the replacement outputs are present through `Switch.status` and Kubernetes Node labels for leaf, spine, and core.

After that migration, `ScaleOutLeafGroup` remains only as a deprecated legacy resource and should not be treated as the source of truth for scale-out topology.

## Optional Configuration

Configure the leaf topology label key in Helm values.

```yaml
topologyLabels:
  scaleOutLeaf: unifabric.io/scale-out-leaf
```

By default, the label written back to Kubernetes Nodes has this form:

```yaml
unifabric.io/scale-out-leaf: <scale-out-leaf-group-name>
```

When a node no longer belongs to any usable group, the Controller cleans up this label to prevent stale topology information from affecting scheduling.

| Configuration | Default | Description |
| --- | --- | --- |
| `topologyLabels.scaleOutLeaf` | `unifabric.io/scale-out-leaf` | Scale-out leaf group label key written back to Kubernetes Nodes |
| `scaleOutDiscovery.leafGroups.enabled` | `true` | Whether to enable `ScaleOutLeafGroup` discovery and Node leaf label updates |

## Additional Notes

- `ScaleOutLeafGroup` depends on LLDP information in `FabricNode.status.scaleOutNics[*].lldpNeighbor`.
- Nodes without available leaf neighbors cannot create a new group.
- Empty groups are deleted automatically.
- The Controller needs permission to update Kubernetes Node labels.
- Consumers should check both `status.healthy` and `status.nodes[*].healthy` before using a group as a scheduling boundary.
- Current scale-out leaf grouping is based only on scale-out RDMA NICs, not storage topology NICs.
