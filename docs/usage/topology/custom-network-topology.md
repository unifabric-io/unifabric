# Custom Network Topology

中文版: [custom-network-topology.zh.md](./custom-network-topology.zh.md)

## Introduction

Custom network topology is the discovery method for switches that cannot run
switch-agent. The reason can be a security policy, a switch that another team
manages, or a switch OS that has no Docker and no Linux shell. The switch only
has to send LLDP on the ports that face the nodes.

In this method, the Unifabric Agent on each node still reads the LLDP neighbor of
each RDMA NIC. From this, the Controller discovers the leaf switches and the links
from the nodes to them. For the tiers above the leaf, you create one `Switch`
resource for each spine and core switch. In its `unifabric.io/switch-neighbors`
annotation, you list the switches in the tier below that connect directly to it.
The Controller combines the discovered leaf tier with the declared upper tiers.
It writes the same Node labels and `Topology` resource as the other methods.

| Item | Description |
| --- | --- |
| Applicable networks | Scale-out network, Storage network |
| Topology source | The LLDP neighbors on the node side, plus the neighbor declarations on the `Switch` resources |
| Discovered tiers | The leaf tier is discovered automatically. The spine and core tiers come from the declarations. |
| Switch side requirements | LLDP is enabled on the ports that face the nodes. Any switch OS is possible. No software runs on the switch. |
| Cluster side requirements | None. |
| Helm configuration | The same as [LLDP discovery in RoCE networks](./roce-lldp-discovery.md): `topoDiscovery.scaleOut.mode=unifabric-roce` or `topoDiscovery.storage.mode=unifabric-roce`. |

Limits:

- The declared links do not update when the physical links change. After you add
  a switch or move a cable, update the declarations.
- The leaf tier can come only from the LLDP on the node side. You cannot write a
  declaration for a leaf or for a node, and there is no other way to tell the
  Controller which leaf a node connects to. If the switch ports that face the
  nodes do not send LLDP, including when no switch can enable LLDP, the nodes
  cannot find their leaf and get no topology labels. Then you cannot use this
  method, and Unifabric has no other method for this case yet.
- The Controller checks only the format and the names in the declarations. It
  does not check that the declarations match the physical links. Overlapping or
  wrong declarations usually do not cause an error. They cause wrong labels. See
  [Verification](#verification) and [Troubleshooting](#troubleshooting).
- Multi-plane networks are not supported yet. See
  [Network topology discovery](./README.md#view-topology-and-node-labels).

For the discovery method to use for each network and each network device, see
[Network topology discovery](./README.md#choose-a-discovery-method-by-network).

## Tiers and Declaration Rules

The Controller calculates the tier of each switch from its distance to the nodes:

```text
                    core1                     tier 3 (core)
                  /       \
            spine1         spine2             tier 2 (spine)
           /      \       /      \
        leaf1   leaf2   leaf3   leaf4         tier 1 (leaf)
          |       |       |       |
        node1   node2   node3   node4         nodes
```

- A leaf connects directly to nodes. It is tier 1. The nodes discover the leaf
  switches through LLDP.
- A spine connects to leaf switches. It is tier 2.
- A core connects to spine switches. It is tier 3.

Only the links set the tier. The switch name and the `role` in the `Switch`
resource do not. `role` only selects the network: `ScaleOut` or `Storage`.

How to declare each type of switch:

| Switch | Create a `Switch` resource | Content of `unifabric.io/switch-neighbors` | Example from the figure |
| --- | --- | --- | --- |
| leaf | No | None | None |
| spine | Yes | The leaf switches that connect directly to it. Use the leaf hostnames that the node LLDP sees. | `spine1`: `["leaf1", "leaf2"]` |
| core | Yes | The spine switches that connect directly to it. Use the names of the spine `Switch` resources. | `core1`: `["spine1", "spine2"]` |

By the number of tiers in the network:

- Leaf tier only: do not create `Switch` resources. The nodes get only the tier 1
  label.
- Leaf and spine tiers: create a `Switch` resource for each spine, and list its
  leaf switches. The spine must have a declaration. Without it, the nodes get only
  the tier 1 label.
- Leaf, spine, and core tiers: create a `Switch` resource for each spine and each
  core. A spine lists its leaf switches. A core lists its spine switches.

Rules:

- Declare only links between two adjacent tiers. Do not declare links between
  switches of the same tier, for example spine to spine. If both spine switches
  already list leaf switches, the link is ignored. If one of them lists no leaf,
  it moves up one tier and becomes a core. Do not skip a tier, for example a core
  that lists leaf switches. If you do, that core is placed in the spine tier.
- A link has no direction. A link between a spine and a core has the same effect
  on the core or on the spine. Declare each link only once. It is recommended to
  always write it on the switch in the upper tier, as in the table above.
- In one network, each `Switch` resource must have the annotation. If a switch has
  no lower neighbor to declare, set the value to `'[]'`.

## Prerequisites

- Unifabric is deployed according to the [basic installation](../../getting-started.md),
  and `topoDiscovery.scaleOut.mode` or `topoDiscovery.storage.mode` is
  `unifabric-roce`.
- LLDP is enabled on the switch ports that face the nodes. The nodes must see
  their leaf switches. Use the command below to make sure of this. The hostname
  column must not be empty:

  ```bash
  kubectl get fabricnode node1 -o jsonpath='{range .status.scaleOutNics[*]}{.name}{"\t"}{.lldpNeighbor.hostname}{"\t"}{.lldpNeighbor.port}{"\n"}{end}'
  ```

  ```text
  eth1	leaf1	Ethernet0
  ```

- The operator must know the physical links between the leaf, spine, and core
  switches.
- No `Switch` of the same role runs switch-agent. The annotation key selects
  Custom network topology for the whole role. If one `Switch` of the role has the key
  and another does not, the Controller ignores all annotations of that role.

## Configuration and Usage

The example uses the three-tier network from
[Tiers and declaration rules](#tiers-and-declaration-rules).

1. Do not create `Switch` resources for the leaf switches. The node LLDP already
   discovers them. The Controller uses the LLDP hostname of each leaf as its
   name.

2. Create one `Switch` resource for each spine switch. In the annotation, list
   the leaf switches that connect directly to it. Set `role` to `ScaleOut` for
   the Scale-out network or to `Storage` for the Storage network:

   ```bash
   kubectl apply -f - <<'EOF'
   apiVersion: unifabric.io/v1beta1
   kind: Switch
   metadata:
     name: spine1
     annotations:
       unifabric.io/switch-neighbors: '["leaf1", "leaf2"]'
   spec:
     role: ScaleOut
   ---
   apiVersion: unifabric.io/v1beta1
   kind: Switch
   metadata:
     name: spine2
     annotations:
       unifabric.io/switch-neighbors: '["leaf3", "leaf4"]'
   spec:
     role: ScaleOut
   EOF
   ```

   The annotation value is a quoted JSON array of strings. Each name must be the
   LLDP hostname of a leaf that the nodes discovered, or the name of another
   `Switch` resource with the same role. Do not use a Kubernetes Node name.

3. If the network has a core tier, create one `Switch` resource for each core
   switch. In the annotation, list the spine switches that connect directly to
   it. If the network has only the leaf and spine tiers, skip this step:

   ```bash
   kubectl apply -f - <<'EOF'
   apiVersion: unifabric.io/v1beta1
   kind: Switch
   metadata:
     name: core1
     annotations:
       unifabric.io/switch-neighbors: '["spine1", "spine2"]'
   spec:
     role: ScaleOut
   EOF
   ```

> Warning: The declared links do not update when the physical links change.
> After you add a switch or move a cable, update the annotation. If you do not,
> the Node labels do not match the real topology.

> Do not run switch-agent and set this annotation on `Switch` resources of the
> same role.

## Verification

The Controller does not check that the declarations match the physical links.
After the configuration, use the steps below to compare the result with the real
topology item by item.

1. Make sure that the `Switch` resources exist. `HEALTHY` and `NEIGHBORS` are
   empty because no switch-agent reports to the Controller:

   ```bash
   kubectl get switches -o wide
   ```

   ```text
   NAME     MGMTIP   ROLE       HEALTHY   NEIGHBORS
   core1             ScaleOut
   spine1            ScaleOut
   spine2            ScaleOut
   ```

2. Make sure that the `Topology` resource has no Warning Event:

   ```bash
   kubectl describe topologies.unifabric.io scaleout
   ```

   If `Events` shows `AutoDiscoveryConflict` or `TopologyLabelConflict`, see
   [Troubleshooting](#troubleshooting).

3. View the full topology that the `Topology` resource builds:

   ```bash
   kubectl get topologies.unifabric.io scaleout -o yaml
   ```

   ```yaml
   status:
     domains:
       - name: tier3-group1
         switchMember:
           - core1
         tier: 3
       - name: tier2-group1
         parent: tier3-group1
         switchMember:
           - spine1
         tier: 2
       - name: tier2-group2
         parent: tier3-group1
         switchMember:
           - spine2
         tier: 2
       - name: tier1-group1
         parent: tier2-group1
         tier: 1
       - name: tier1-group2
         parent: tier2-group1
         tier: 1
       - name: tier1-group3
         parent: tier2-group2
         tier: 1
       - name: tier1-group4
         parent: tier2-group2
         tier: 1
     nodes:
       - name: node-group1
         nodes:
           - node1
         switchDomainPath:
           - tier3-group1
           - tier2-group1
           - tier1-group1
       # node-group2 to node-group4 are omitted
   ```

   Compare item by item:

   - The number of domains in each tier matches the real network. The example
     has 1 core domain, 2 spine domains, and 4 leaf domains.
   - The `switchMember` of each spine and core domain contains only the switches
     that must be in the same domain. If spine switches that must be separate are
     in one domain, the declarations usually overlap.
   - The `parent` of each domain points to the correct upper domain.
   - The length of the `switchDomainPath` of each node equals the number of tiers
     in the network. A shorter path means that no spine declares its leaf.

   Leaf switches have no `Switch` resource, so leaf domains have no
   `switchMember`. Use the Node labels and the LLDP neighbors in `FabricNode` to
   check the leaf domains:

   ```bash
   kubectl get nodes -L scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2,scale-out.unifabric.io/tier-3
   ```

   For the Node labels and the `Topology` resource, see
   [Network topology discovery](./README.md#view-topology-and-node-labels).

## Troubleshooting

### View Errors

The Controller can find only these errors:

- The annotation is not a JSON array of strings, or the array has an empty name.
- A neighbor name is neither a leaf hostname that the nodes discovered nor the
  name of a `Switch` resource with the same role.
- A `Switch` lists itself as a neighbor.
- The calculated topology has a cycle or a gap, or it conflicts with labels that
  are already written.

For these errors, the Controller stops the reconciliation, and the existing
labels stay. The errors are in the Events of the `Topology` resource and in the
Controller log:

```bash
kubectl describe topologies.unifabric.io scaleout
kubectl -n unifabric-system logs deployment/unifabric-controller --tail 200 | grep -i topology
```

### Topology Does Not Match the Real Network

The declaration mistakes below cause no error. They cause a wrong topology. After
you compare the result as in [Verification](#verification), check the related
declarations:

| Symptom | Common cause |
| --- | --- |
| Spine switches that must be separate are in one domain | More than one spine declares the same leaf, but these spine switches do not share that leaf. |
| A node is in the wrong spine domain | Its leaf is declared under the wrong spine. |
| Some nodes have only the tier 1 label | No spine declares their leaf. |
| A core is in the same tier as the spine switches, and there is no tier 3 | The core lists leaf switches directly. |
| A spine is in tier 3, and some nodes have one more tier | That spine lists no leaf and lists only another spine. |
| A switch is in no domain | It has no path to any leaf, for example its annotation is empty and no other switch lists it. |

After you correct the annotation, the Controller does not change the labels that
it already wrote. The written labels are locked. If the new result conflicts with
them, the Controller reports `AutoDiscoveryConflict` and keeps the old labels.
Delete the `Topology` resource. The Controller then removes the labels that it
wrote and calculates again:

```bash
kubectl delete topologies.unifabric.io scaleout
```

After the Controller removes the labels, it writes labels from the current
declarations and creates the `scaleout` resource again. During the recalculation,
the scheduler cannot read topology labels for this network. For the Storage
network, delete the `storage` resource.

### Node Labels Have Only Tier 1

- Make sure that a `Switch` resource exists for each switch at the spine tier
  and above.
- Make sure that each `Switch` of the same role has the
  `unifabric.io/switch-neighbors` annotation. One `Switch` without the key
  changes the whole role back to switch-agent discovery.
- Make sure that no `Switch` resource exists for a leaf switch, and that no
  `Switch` resource has the same name as a leaf that the nodes discovered.
- Make sure that the annotation is a quoted JSON array of strings. The array
  must contain only switch names, not Kubernetes Node names.
- Make sure that some spine declares each leaf.
- Make sure that the Controller reports no error, as in [View errors](#view-errors).
- If the physical network has only a leaf tier, tier 1 alone is the expected
  result.

## References

- [Network topology discovery](./README.md): the discovery methods and how to
  view the result.
- [LLDP discovery in RoCE networks](./roce-lldp-discovery.md): automatic discovery with switch-agent.
- [Switch reference](../../reference/switch.md): the `spec`, `status`, and
  annotation of the `Switch` resource.
- [Scale-out topology design](../../design/scaleout-topology.md): how the
  Controller combines the node LLDP with the neighbor declarations.
- [Topology CRD design](../../design/topology-crd.md): locked labels and the reset
  by deleting a `Topology`.
