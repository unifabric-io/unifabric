# Network Topology Discovery

中文版: [README.zh.md](./README.zh.md)

## Introduction

An AI cluster depends on three high-speed networks:

- The scale-out network carries GPU traffic between nodes.
- The scale-up network connects the GPUs inside one node.
- The storage network carries traffic to high-speed storage.

A group of training or inference AI workloads needs topology-aware placement on
these networks. If the scheduler spreads their Pods across random nodes of the
cluster, their traffic crosses the spine tier or the core tier. This adds network
latency, and the traffic of other jobs interferes. For example, in
prefill-decode disaggregated inference, the Prefill instances and the Decode
instances transfer the KV cache between them. If both are under the same leaf
switch, the transfer takes one hop. If they are in different spine domains, the
transfer takes two or more extra hops.

Kueue, Volcano, and KAI Scheduler all support topology-aware placement, but they
do not see the physical network. They read only the topology labels on the Nodes.
For the scheduler to work, somebody must record the position of each node in the
network as a label on the Node object. This meets two problems:

- The topology labels have no reliable source.

  The physical links are spread across the switches, the NICs, and the management
  platforms of many vendors. The tools of each vendor do not talk to each other,
  and none of them connect to Kubernetes. In a cluster with hundreds of nodes,
  manual labeling of each node is slow and easy to get wrong. After a switch is
  added or a cable is moved, the labels do not follow, and they no longer match
  the real topology.
- Operators have no global map of the network topology.

  No single place shows the full set of links between the nodes, the leaf
  switches, the spine switches, and the core switches. When a training job slows
  down or a link reports an error, operators first have to piece the picture
  together from the switches, the NICs, and the vendor platforms: which nodes
  share a leaf, and which jobs a link carries. Only then can they look for the
  fault. In a cluster with hundreds of nodes and tens of switches, this step
  alone takes a long time.

Network topology discovery detects the links between hosts and switches and
between switches in these three networks. It writes the result as two standard
Kubernetes resources, one for each problem:

- Tiered Node labels.

  Unifabric discovers the leaf, spine, and core domain of each node in real time
  and writes them as Node labels. The scheduler consumes them directly and places
  the Pods of one job in the same switch domain. When the physical topology
  changes, the labels update.
- The `Topology` resource.

  Each switch becomes a `Switch` resource, and each of the three networks becomes
  a `Topology` resource that shows its performance domains and the path of each
  node. Grafana renders them as one global topology map in real time. When a
  switch or a link fails, operators see on the map which nodes and which jobs it
  affects, and then go to the fault.

Whether the devices are SONiC, NVIDIA Spectrum, or InfiniBand, the result uses
the same label model and the same `Topology` resource. The scheduler
configuration and the operations procedures do not change with the vendor.

The scope of topology discovery is:

| Topology relation | Compute fabric (scale-out) | Storage fabric | NVLink fabric (scale-up) |
| --- | --- | --- | --- |
| Tier of each host (which leaf domain it is in) | Yes | Yes | Yes (which NVLink domain it is in) |
| Tier of each leaf, spine, and core switch | Yes | Yes | Not applicable |

In this document you learn:

- Which fabrics, discovery methods, and network devices topology discovery
  supports.
- How to complete topology discovery with each of the four discovery methods.
- How to view the result: Node labels, the `Topology` resource, and the Grafana
  topology graph.
- How schedulers such as Kueue, Volcano, and KAI Scheduler consume the labels.

## Supported Scope

Each fabric has its own discovery mode in the Helm values. The three fabrics are
independent. For example, the compute fabric can use InfiniBand discovery while
the storage fabric uses LLDP discovery.

| Fabric | Helm value | Discovery methods |
| --- | --- | --- |
| Compute (scale-out) | `topoDiscovery.scaleOut.mode` | `unifabric-roce`: LLDP discovery (Method 1) or manual topology (Method 4). `nv-topograph`: NetQ import (Method 2) or InfiniBand (Method 3). |
| Storage | `topoDiscovery.storage.mode` | `unifabric-roce`: LLDP discovery (Method 1) or manual topology (Method 4). |
| NVLink (scale-up) | `topoDiscovery.scaleUp.mode` | `nv-topograph`: NVIDIA Topograph reads the NVLink domain from the GPU Operator (Method 3) or from NVIDIA NMX through NetQ (Method 2). `manual`: an administrator writes the labels. |

The network devices that each method supports:

| Method | Switches | Required on the switch | Required from the cluster |
| --- | --- | --- | --- |
| Method 1: LLDP discovery with switch-agent | SONiC switches and NVIDIA Spectrum switches that run Cumulus Linux. Other Ethernet switches that run Docker or a Linux binary. | LLDP enabled. Docker or a Linux shell for switch-agent. | The Controller reaches TCP port `8090` on the switch management IP. |
| Method 2: NetQ import | NVIDIA Spectrum-X switches that NetQ manages. | NetQ manages the fabric. | The Topograph Pod reaches the NetQ API. |
| Method 3: InfiniBand | NVIDIA Quantum InfiniBand switches. | A Subnet Manager runs. | `ibnetdiscover` runs on a GPU node. |
| Method 4: Manual topology | Any Ethernet switch that sends LLDP. The switch OS does not matter. | LLDP enabled. Nothing else. | None. |

If a switch sends no LLDP, the nodes see no neighbor. Unifabric then cannot
discover any switch, and the Node has no topology labels for that fabric.

## Prerequisites

- Unifabric is deployed according to the [basic installation](../../getting-started.md).
  The `topoDiscovery.*.mode` values for your fabrics are set according to its
  "Other Network Environments" section.
- In `kubectl get fabricnodes`, every node shows `READY` as `True`, and the NIC
  classification is correct. The verification section of the basic installation
  covers this step.
- The network path from the cluster to the devices is open. Method 1 needs the
  node that runs `unifabric-controller` to reach the switch management IPs,
  usually on the out-of-band management network. Method 2 needs the node that
  runs the Topograph Pod to reach the NetQ API. Make sure that the firewall of
  the switch and of the cluster allows this path.
- Only for topology visualization: Grafana Operator and its CRDs are installed in
  the cluster.

## Discovery Methods

Select the method for each fabric from the tables in
[Supported Scope](#supported-scope). The methods differ only in where the
topology comes from. The result is the same, and you examine it in
[Verification](#verification).

### Method 1: LLDP Discovery with switch-agent

LLDP is a protocol with which a device announces its identity and its port to the
device at the other end of a link. The Unifabric Agent on each node reads the LLDP
neighbor of each RDMA NIC. This gives the links from the nodes to the leaf
switches. The Unifabric switch-agent on each switch reads the LLDP neighbors of
the switch. This gives the links between the switches. The Controller combines
both and computes the leaf, spine, and core tiers.

| Item | Description |
| --- | --- |
| Fabrics | Compute, storage |
| Topology source | LLDP neighbors on the nodes and on the switches |
| Tiers discovered | Node, leaf, spine, core. All tiers. |
| Helm configuration | `topoDiscovery.scaleOut.mode=unifabric-roce` (the default) or `topoDiscovery.storage.mode=unifabric-roce`. The storage fabric also needs a NIC selector. See the [basic installation](../../getting-started.md#installation). |

Use this method when you can run a container or a binary on the switches. SONiC
and Cumulus Linux both support this. If a security policy forbids it, or the
switch OS has no Docker and no Linux shell, use Method 4 instead.

Limitations:

- This method has no source for the NVLink fabric. Keep
  `topoDiscovery.scaleUp.mode` at `manual`.

#### Configuration

Export the mTLS certificates, run switch-agent on every switch, and create one
`Switch` resource per switch. See
[switch-agent onboarding](./switch-agent-onboarding.md).

### Method 2: NetQ Import

Spectrum-X is the NVIDIA Ethernet RoCE network platform for AI clusters. NVIDIA
NetQ manages its fabric and already knows the full topology. Unifabric imports the
topology from NetQ through NVIDIA Topograph. Topograph is an open-source NVIDIA
service that reads the network topology from vendor data sources and writes Node
labels.

| Item | Description |
| --- | --- |
| Fabrics | Compute. NVLink when NVIDIA NMX is available through NetQ. |
| Topology source | NVIDIA Topograph reads the fabric topology through the NetQ API. For the NVLink fabric it reads the NVLink domains from NMX. |
| Tiers discovered | The fabric data in NetQ decides. Usually leaf, spine, and all tiers above. |
| Helm configuration | `topoDiscovery.scaleOut.mode=nv-topograph`, `nvidiaTopograph.provider.name=netq`, and the NetQ credentials. Add `topoDiscovery.scaleUp.mode=nv-topograph` for the NVLink fabric. See the [basic installation](../../getting-started.md#spectrum-x-fabric). |

Limitations:

- This method creates no `Switch` resources. The `Topology` resource and the Node
  labels are complete, but in topology visualization the switch member card of
  each performance domain is empty.
- This method does not cover the storage fabric. If the storage fabric runs on
  Spectrum switches, use Method 1 or Method 4 for it.

> **Note:** Spectrum switches run Cumulus Linux, which supports LLDP and Docker.
> If NetQ is not available, you can use Method 1 on Spectrum switches instead.

#### Configuration

The Node labels and the `Topology` resource appear after the basic installation.
No more steps are needed. If you want topology visualization to show the
switches, see
[Add Switch Resources for Topograph Scenarios](./topograph-switch-resources.md).

### Method 3: InfiniBand

InfiniBand is a high-speed interconnect that is independent of Ethernet. A Subnet
Manager keeps the topology of the whole fabric. Unifabric uses the
`infiniband-k8s` provider of NVIDIA Topograph. Topograph runs `ibnetdiscover` in
a Pod on a GPU node to read the full fabric topology. For the NVLink fabric, it
reads the `nvidia.com/gpu.clique` label that the GPU Operator writes on each
node.

| Item | Description |
| --- | --- |
| Fabrics | Compute, NVLink |
| Topology source | `ibnetdiscover` for the compute fabric. The GPU Operator clique label for the NVLink fabric. |
| Tiers discovered | Compute: leaf, spine, core. NVLink: tier 1, the NVLink domain. |
| Helm configuration | `topoDiscovery.scaleOut.mode=nv-topograph` and `topoDiscovery.scaleUp.mode=nv-topograph`. See the [basic installation](../../getting-started.md#infiniband-fabric). |

The compute label value is a name derived from the GUID of the InfiniBand switch,
for example `S-fc6a1c03006636c0`.

Limitations:

- As in Method 2, this method creates no `Switch` resources, and the switch member
  cards in topology visualization are empty.
- This method does not cover the storage fabric. If the storage fabric is RoCE,
  use Method 1 or Method 4 for it.
- The `infiniband-k8s` provider needs `ibnetdiscover` to succeed. It cannot
  produce the NVLink domains alone on a cluster without InfiniBand.

#### Configuration

The same as Method 2: no more steps are needed. If you want topology
visualization to show the switches, see
[Add Switch Resources for Topograph Scenarios](./topograph-switch-resources.md).

### Method 4: Manual Topology

In this method, you deploy nothing on the switches. The Unifabric Agent on each
node still reads the LLDP neighbor of each RDMA NIC, so the links from the nodes
to the leaf switches are discovered automatically. For the links above the leaf,
you create a `Switch` resource for each spine and core switch and declare its
neighbors in an annotation. The Controller combines the discovered leaf tier with
the declared upper tiers.

| Item | Description |
| --- | --- |
| Fabrics | Compute, storage |
| Topology source | LLDP neighbors on the nodes, plus neighbor declarations on `Switch` resources |
| Tiers discovered | Leaf automatically. Spine and core from the declaration. |
| Helm configuration | The same as Method 1: `topoDiscovery.scaleOut.mode=unifabric-roce` or `topoDiscovery.storage.mode=unifabric-roce`. |

Use this method when a security policy forbids software on the switches, when
another team manages the switches, or when the switch OS cannot run switch-agent.
The switch only has to send LLDP.

Limitations:

- The declared links do not update when the physical links change. After you add
  a switch or move a cable, update the declaration.
- The leaf tier still comes from LLDP. If the switches send no LLDP, this method
  does not work.
- This method has no source for the NVLink fabric.

#### Configuration

Create the `Switch` resources for the spine and core switches with the
`unifabric.io/switch-neighbors` annotation. See
[Manual topology](./manual-topology.md).

## Verification

### View Topology and Node Labels

This section uses a cluster as the example. The compute fabric is SONiC RoCE.
`node1` and `node2` connect to both `leaf1` and `leaf2`. `node3` and `node4`
connect to both `leaf3` and `leaf4`. The four leaf switches connect to `spine1`:

![Example SONiC RoCE topology](../../images/sonic-roce-topology-example.jpg)

After discovery completes, view the compute fabric labels:

```bash
kubectl get nodes \
  -L scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2
```

```text
NAME    STATUS   ROLES    AGE   VERSION   SCALE-OUT.UNIFABRIC.IO/TIER-1   SCALE-OUT.UNIFABRIC.IO/TIER-2
node1   Ready    <none>   30d   v1.32.0   tier1-group1                    tier2-group1
node2   Ready    <none>   30d   v1.32.0   tier1-group1                    tier2-group1
node3   Ready    <none>   30d   v1.32.0   tier1-group2                    tier2-group1
node4   Ready    <none>   30d   v1.32.0   tier1-group2                    tier2-group1
```

`node1` and `node2` have the same `tier-1` value, so they are in the same leaf
domain. All four nodes have the same `tier-2` value, so they share one spine
domain. If a node has `tier-1` but no `tier-2`, the tiers above the leaf were not
discovered. See [Troubleshooting](#troubleshooting).

The storage fabric and the NVLink fabric use the same model with their own label
prefixes. In the example, the storage fabric has one leaf tier:

```bash
kubectl get nodes -L storage.unifabric.io/tier-1
```

```text
NAME    STATUS   ROLES    AGE   VERSION   STORAGE.UNIFABRIC.IO/TIER-1
node1   Ready    <none>   30d   v1.32.0   tier1-group1
node2   Ready    <none>   30d   v1.32.0   tier1-group1
node3   Ready    <none>   30d   v1.32.0   tier1-group2
node4   Ready    <none>   30d   v1.32.0   tier1-group2
```

For the NVLink fabric, run `kubectl get nodes -L scale-up.unifabric.io/tier-1`.
The value is the NVLink domain ID that Topograph derives from the GPU Operator
clique label or from NMX. Nodes with the same value share one NVLink domain.

The label model for all three fabrics:

| Node label | Meaning |
| --- | --- |
| `scale-out.unifabric.io/tier-1` | The compute leaf switch domain that the node connects to |
| `scale-out.unifabric.io/tier-2` | The compute spine domain above the leaf |
| `scale-out.unifabric.io/tier-3` | The compute core domain above the spine |
| `storage.unifabric.io/tier-N` | The matching tier of the storage fabric. The rules are the same as for compute. |
| `scale-up.unifabric.io/tier-1` | The NVLink domain inside the node |

A higher tier number means a tier closer to the top of the network. When two nodes
have the same value for a label, they are in the same performance domain. LLDP
discovery names the domains `tier<N>-group<M>`. Topograph uses its own names, for
example `S-fc6a1c03006636c0` for an InfiniBand switch.

The Helm value `topoDiscovery.<fabric>.nodeLabel.keyTemplate` defines the label
key. The defaults are `scale-out.unifabric.io/tier-{{ .Tier }}`,
`storage.unifabric.io/tier-{{ .Tier }}`, and
`scale-up.unifabric.io/tier-{{ .Tier }}`. You can change the template, but it
must contain exactly one `{{ .Tier }}`.

> **Note:** If you change the key template, change the label keys in the scheduler
> configuration too. Otherwise the scheduler reads no topology information.

Each fabric has one `Topology` resource: `scaleout`, `storage`, and `scaleup`. It
summarizes the hierarchy of the performance domains and the domain path of each
node:

```bash
kubectl get topologies.unifabric.io
kubectl get topologies.unifabric.io scaleout -o yaml
```

```text
NAME       AGE
scaleout   30d
scaleup    30d
storage    30d
```

```yaml
apiVersion: unifabric.io/v1beta1
kind: Topology
metadata:
  name: scaleout
status:
  domains:
    - members:
        - spine1
      name: tier2-group1
      tier: 2
    - members:
        - leaf1
        - leaf2
      name: tier1-group1
      parent: tier2-group1
      tier: 1
    - members:
        - leaf3
        - leaf4
      name: tier1-group2
      parent: tier2-group1
      tier: 1
  nodes:
    - domainPath:
        - tier2-group1
        - tier1-group1
      nodes:
        - node1
        - node2
    - domainPath:
        - tier2-group1
        - tier1-group2
      nodes:
        - node3
        - node4
```

In `status.domains`, `members` lists the switches in a domain, and `parent` points
to the domain one tier above. In `status.nodes`, each group of nodes shares one
domain path from top to bottom. The `storage` resource has the same shape. The
`scaleup` resource has only tier 1 domains and no switch members. For the full
field description, see the [Topology reference](../../reference/topology.md).

#### Use the Node Labels in a Scheduler

A topology-aware scheduler arranges the Node labels into tiers from top to bottom.
It then places the Pods of one job tier by tier. The table maps the Unifabric
labels to the configuration entry of each scheduler. For the installation and
configuration of the scheduler itself, follow its official documentation:

| Scheduler | Configuration entry | Unifabric labels to fill in |
| --- | --- | --- |
| Kueue Topology Aware Scheduling | `spec.levels` of the `Topology` resource | From top to bottom: `scale-out.unifabric.io/tier-3`, `tier-2`, `tier-1`, then `kubernetes.io/hostname` |
| Volcano network topology aware scheduling | The `HyperNode` resource | Create one HyperNode per tier. The member selector references the matching label. |
| KAI Scheduler topology aware scheduling | `spec.levels` of the `Topology` resource | The same as Kueue |

References: [Kueue TAS](https://kueue.sigs.k8s.io/docs/concepts/topology_aware_scheduling/),
[Volcano network topology](https://volcano.sh/en/docs/network_topology_aware_scheduling/),
[KAI Scheduler](https://github.com/NVIDIA/KAI-Scheduler).

### View Topology Visualization

The Grafana topology dashboard renders each `Topology` resource as an interactive
graph. The graph matches the Node labels above. Each tier 1 domain is a leaf group.
Each tier 2 domain is a spine group above the leaf groups. Each node hangs under
its leaf group.

![Topology visualization dashboard](../../images/topology-visualization.png)

For the dashboard installation, the connection to an external Grafana, and the
details of the graph, see
[Topology Visualization](./topology-visualization.md).

## Troubleshooting

### A Node Has No Topology Labels

First make sure that the prerequisites are met:

1. For Method 1 and Method 4, make sure that the NICs in the `FabricNode`
   received LLDP neighbors:

   ```bash
   kubectl get fabricnode node1 -o jsonpath='{range .status.scaleOutNics[*]}{.name}{"\t"}{.lldpNeighbor.hostname}{"\t"}{.lldpNeighbor.port}{"\n"}{end}'
   ```

   ```text
   eth1	leaf1	Ethernet0
   eth2	leaf2	Ethernet0
   ```

   If the hostname column is empty, make sure that LLDP is enabled on the switch
   and that the switch port of the node NIC sends LLDP frames. The
   `LLDPNeighborsReady` condition of the `FabricNode` also reports the reason
   `LLDPNeighborMissing`.

2. For Method 2 and Method 3, make sure that `mode` is `nv-topograph` and that
   the Topograph Deployment runs:

   ```bash
   kubectl -n unifabric-system get deployment unifabric-nvidia-topograph
   kubectl -n unifabric-system logs deployment/unifabric-nvidia-topograph --tail 100
   ```

   For Method 2, examine the NetQ credentials and the API URL, and make sure that
   the Topograph Pod reaches the NetQ API. For Method 3, make sure that the log
   shows that `ibnetdiscover` runs.

3. Read the Controller log:

   ```bash
   kubectl -n unifabric-system logs deployment/unifabric-controller --tail 200
   ```

### Node Labels Have Only Tier 1

LLDP on the nodes discovers only the leaf tier. This symptom means that the
information above the leaf tier did not reach the Controller:

- Method 1: make sure that every switch has a `Switch` resource and that
  `HEALTHY` is `true` in `kubectl get switch -o wide`. If it is `false`, make
  sure that the node that runs the Controller can reach port `8090` of the switch
  and that the mTLS certificates match. See
  [switch-agent onboarding](./switch-agent-onboarding.md#troubleshooting).
- Method 4: make sure that a `Switch` resource exists for each switch at the
  spine tier and above, and that each one has the
  `unifabric.io/switch-neighbors` annotation. See
  [Manual topology](./manual-topology.md#troubleshooting).
- If the physical network has only a leaf tier, tier 1 alone is the expected
  result.

### Switch Members Are Empty in Topology Visualization

This is the expected behavior for Method 2 and Method 3. See
[Add Switch Resources for Topograph Scenarios](./topograph-switch-resources.md).

## References

- [Topology discovery concepts](../../concepts/topology-discovery.md): logical
  topology, discovery methods, and discovery results.
- [switch-agent onboarding](./switch-agent-onboarding.md): Method 1.
- [Manual topology](./manual-topology.md): Method 4.
- [Add Switch Resources for Topograph Scenarios](./topograph-switch-resources.md):
  the optional step for Method 2 and Method 3.
- [Topology Visualization](./topology-visualization.md): installation and
  reading of the Grafana dashboard.
- [Topology reference](../../reference/topology.md),
  [FabricNode reference](../../reference/fabricnode.md),
  [Switch reference](../../reference/switch.md).
- [Scale-out topology discovery design](../../design/scaleout-topology.md),
  [Topology CRD design](../../design/topology-crd.md).
- [Helm values reference](../../../chart/README.md): all `topoDiscovery.*`
  parameters.
