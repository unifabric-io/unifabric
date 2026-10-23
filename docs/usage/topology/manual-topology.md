# Manual Topology

中文版: [manual-topology.zh.md](./manual-topology.zh.md)

## Introduction

Manual topology is the discovery method for switches on which you cannot run
switch-agent. The reason can be a security policy, a switch that another team
manages, or a switch OS that provides no Docker and no Linux shell. The switch
only has to send LLDP.

In this method, the Unifabric Agent on each node still reads the LLDP neighbor of
each RDMA NIC. From this, the Controller discovers the leaf switches and the links
from the nodes to them. For the tiers above the leaf, you create one `Switch`
resource for each spine and core switch and declare the switches one tier below
it in the `unifabric.io/switch-neighbors` annotation. The Controller combines the
discovered leaf tier with the declared upper tiers and writes the same Node
labels and `Topology` resource as the other methods.

The method applies to the compute fabric and the storage fabric. For the result
and the limitations, see
[Network Topology Discovery](./README.md#method-4-manual-topology).

## Prerequisites

- Unifabric is deployed according to the [basic installation](../../getting-started.md),
  and `topoDiscovery.scaleOut.mode` or `topoDiscovery.storage.mode` is
  `unifabric-roce`.
- LLDP is enabled on the switch ports that face the nodes. The nodes must see
  their leaf switches. Make sure with the command below. The hostname column
  must not be empty:

  ```bash
  kubectl get fabricnode node1 -o jsonpath='{range .status.scaleOutNics[*]}{.name}{"\t"}{.lldpNeighbor.hostname}{"\t"}{.lldpNeighbor.port}{"\n"}{end}'
  ```

  ```text
  eth1	leaf1	Ethernet0
  eth2	leaf2	Ethernet0
  ```

- You know the physical links between the leaf, spine, and core switches.
- No `Switch` of the same role runs switch-agent. The annotation key selects
  manual topology for the whole role. If one `Switch` of the role has the key
  and another does not, the Controller ignores all annotations of that role.

## Configuration and Usage

The example uses the topology from
[Network Topology Discovery](./README.md#view-topology-and-node-labels): four
nodes, four leaf switches, and one spine switch `spine1` that connects to
`leaf1` to `leaf4`.

1. Do not create `Switch` resources for the leaf switches. The node LLDP already
   discovered them. The Controller uses the LLDP hostname of each leaf as its
   name.

2. Create one `Switch` resource for each switch at the spine tier and above.
   Declare the switches one tier below it in the annotation. The `role` is
   `ScaleOut` for the compute fabric or `Storage` for the storage fabric:

   ```bash
   kubectl apply -f - <<'EOF'
   apiVersion: unifabric.io/v1beta1
   kind: Switch
   metadata:
     name: spine1
     annotations:
       unifabric.io/switch-neighbors: '["leaf1", "leaf2", "leaf3", "leaf4"]'
   spec:
     role: ScaleOut
   EOF
   ```

   The annotation value is a quoted JSON array of strings. Each name must be the
   LLDP hostname of a leaf that the nodes discovered, or the name of another
   `Switch` resource with the same role. Do not use a Kubernetes Node name. The
   link is undirected, so you declare each link once.

3. If the network has a core tier, create one `Switch` resource for each core
   switch. Its neighbors are the spine switches that connect to it:

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

> **Warning:** The declared links do not update when the physical links change.
> After you add a switch or move a cable, update the annotation. Otherwise the
> Node labels do not match the real topology. Do not run switch-agent and set
> this annotation on `Switch` resources of the same role.

## Verification

Make sure that the `Switch` resources exist. `HEALTHY` and `NEIGHBORS` are empty
because no switch-agent reports to the Controller:

```bash
kubectl get switches -o wide
```

```text
NAME     MGMTIP   ROLE       HEALTHY   NEIGHBORS
spine1            ScaleOut
```

The final result is the Node labels and the `Topology` resource. To view them,
see [Network Topology Discovery](./README.md#view-topology-and-node-labels). In
the `Topology` resource, the leaf domains list the leaf hostnames from LLDP as
their members, and the spine domain lists `spine1`.

## Troubleshooting

### Node Labels Have Only Tier 1

- Make sure that a `Switch` resource exists for every switch at the spine tier
  and above.
- Make sure that every `Switch` with the same role has the
  `unifabric.io/switch-neighbors` annotation. One `Switch` without the key
  switches the whole role back to switch-agent discovery.
- Make sure that the annotation is a quoted JSON array of strings, and that the
  array contains only switch names, not Kubernetes Node names.
- Make sure that each neighbor name resolves to a leaf hostname that the node
  LLDP discovered, or to another `Switch` resource with the same role. An
  unresolved name stops the reconciliation and keeps the old labels. Read the
  Controller log:

  ```bash
  kubectl -n unifabric-system logs deployment/unifabric-controller --tail 200
  ```

- Make sure that no `Switch` resource has the same name as a leaf that the nodes
  discovered.
- If the physical network has only a leaf tier, tier 1 alone is the expected
  result.

## References

- [Network Topology Discovery](./README.md): the discovery methods and how to view
  the result.
- [switch-agent onboarding](./switch-agent-onboarding.md): automatic discovery
  with switch-agent.
- [Switch reference](../../reference/switch.md): the `spec`, `status`, and
  annotation of the `Switch` resource.
- [Scale-out topology design](../../design/scaleout-topology.md): how the
  Controller combines node LLDP with the neighbor declaration.
