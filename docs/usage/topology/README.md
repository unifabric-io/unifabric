# Network Topology Discovery

中文版: [README.zh.md](./README.zh.md)

## Introduction

Unifabric network topology discovery detects the links between hosts and switches, and between switches, in an AI cluster.
It writes the result as Kubernetes resources.
The scheduler uses them for topology-aware scheduling, and operators use them to see the global topology.

Topology discovery supports these network types:

- Scale-out network: It carries GPU traffic between nodes and uses a leaf-spine architecture.
- Storage network: It carries traffic from the nodes to high-speed storage.
- Scale-up network: It is the high-speed interconnect between GPUs, for example NVIDIA NVLink.

Topology discovery covers this scope:

- The topology tier of each host: the leaf switch that the host connects to, and the spine domain that it is in.
- The topology tier of the leaf and spine switches: the uplinks and downlinks between the switches.

Topology discovery writes the result as two types of resources:

- Tiered Node labels. They record the switch domain of each node, and the scheduler consumes them directly. When the physical topology changes, the labels update automatically.
- `Switch` and `Topology` resources. Each switch becomes a `Switch` resource, and each network becomes a `Topology` resource. Grafana can render them as a real-time global topology map.

The network devices can be SONiC switches, NVIDIA Spectrum switches, or InfiniBand.
For all of them, the result uses the same label model and the same `Topology` resource.

In this document you learn:

- Which discovery method each network uses on each network device, and whether Unifabric supports it now.
- How each discovery method works, and what it requires from the network devices and the cluster.
- How to view the result: Node labels, the `Topology` resource, and the Grafana topology map.
- How schedulers such as Kueue, Volcano, and KAI Scheduler consume these labels.

## Background

One node has a limited number of GPUs and limited GPU memory, usually 8 GPUs.
Most training jobs and inference jobs cannot complete in one node.
Their Pods must run on many nodes:

- Training jobs: The parameters, gradients, and optimizer states of a large model are much larger than the GPU memory of one node. A training job uses data parallelism, tensor parallelism, pipeline parallelism, and other methods. These methods split the model and the data across tens to thousands of GPUs.
- Inference jobs: The weights of a large model alone can be larger than the GPU memory of one node, so the model must run across nodes. In prefill-decode (PD) disaggregation, the Prefill stage is compute-intensive and the Decode stage is memory-bandwidth-intensive. If both run on the same node, they compete for GPU resources. Each one then gets higher latency and lower throughput. For this reason, the Prefill instances and the Decode instances usually run on different nodes and scale independently.

The GPU traffic between these Pods goes through the Scale-out network.
If two Pods connect to the same leaf switch, their traffic goes through only that switch.
If they connect to different leaf switches, their traffic goes through the spine.
The path is longer, the latency is higher, and the traffic of other jobs interferes on the upper links.
For this reason, such jobs need topology-aware scheduling.
The scheduler puts the Pods of one job in the same leaf domain or spine domain when possible.

Kueue, Volcano, and KAI Scheduler all support topology-aware scheduling, but they do not see the physical network.
They read only the topology labels on the Nodes.
For the scheduler to work, a mechanism must record the network position of each node as a label on the Node object.

This causes two problems:

- The topology labels have no reliable source.

  The physical links are spread across the switches, the NICs, and the network management platforms of many vendors.
  The tools of each vendor do not work together, and they do not connect to Kubernetes.
  In a cluster with hundreds of nodes, manual labeling of each node is slow and easy to get wrong.
  After you add switches or move cables, the labels do not change, and they no longer match the real topology.

- Operators have no global map of the network topology.

  No single place shows all links between the nodes, the leaf switches, and the spine switches.
  A training job can slow down, or a link can report errors.
  Then operators must first collect the data from the switches, the NICs, and the vendor platforms.
  They find which nodes connect to the same leaf, and which jobs each link carries.
  Only then can they look for the fault.
  In a cluster with hundreds of nodes and tens of switches, this step alone takes a long time.

Network topology discovery solves these two problems:

- Tiered Node labels give the scheduler an accurate, real-time topology.

- The `Topology` resource and the Grafana topology map show operators the affected nodes and jobs when a fault occurs.

## Prerequisites

- You deployed Unifabric according to the [basic installation](../../getting-started.md).
- In `kubectl get fabricnodes`, every node shows `READY` as `True`, and the NIC classification is correct. The verification section of the basic installation covers this step.
- Only for topology visualization: Grafana Operator and its CRDs are installed in the cluster.

For the other requirements of each discovery method on the network devices and the cluster, see the related section in [Discovery methods](#discovery-methods).

## Choose a discovery method by network

The three networks are independent.
Each network has its own discovery mode in the Helm values.
One cluster can use different discovery methods together.
For example, the Scale-out network uses InfiniBand network discovery, and the Storage network uses LLDP discovery in RoCE networks.

> Note: If you set both the Scale-out network and the Scale-up network to `nv-topograph`, they share one NVIDIA Topograph instance and one provider (`nvidiaTopograph.provider.name`).

Find your network device in the table for each network to get the discovery method.
Then read the requirements and the configuration in [Discovery methods](#discovery-methods).

The values in the "Support status" column have these meanings:

| Support status | Meaning |
| --- | --- |
| Supported | Unifabric implements it. |
| Not verified | Unifabric implements it, but it was not tested on this network device. |
| Not supported yet | A topology data source exists and it is technically possible, but Unifabric does not implement it yet. |
| Not supported | No topology data source exists, so automatic discovery is not possible. |
| Not applicable | No topology discovery is necessary. |

### Scale-out network

| Network device | Discovery method | Helm configuration | Support status |
| --- | --- | --- | --- |
| Ethernet RoCE switch that runs SONiC and can run switch-agent | [LLDP discovery in RoCE networks](./roce-lldp-discovery.md) | `topoDiscovery.scaleOut.mode=unifabric-roce` (default) | Supported |
| NVIDIA Spectrum-X switch that NetQ manages | [NVIDIA NetQ discovery in RoCE networks](./roce-netq-discovery.md) | `topoDiscovery.scaleOut.mode=nv-topograph`, `nvidiaTopograph.provider.name=netq` | Supported |
| NVIDIA Spectrum-X switch without NetQ | [LLDP discovery in RoCE networks](./roce-lldp-discovery.md) | `topoDiscovery.scaleOut.mode=unifabric-roce` | Not verified |
| NVIDIA InfiniBand switch | [InfiniBand network discovery](./infiniband-discovery.md) | `topoDiscovery.scaleOut.mode=nv-topograph`, `nvidiaTopograph.provider.name=infiniband-k8s` (default) | Supported |
| Ethernet switch that sends LLDP on the ports to the nodes, but cannot run switch-agent | [Custom network topology](./custom-network-topology.md) | `topoDiscovery.scaleOut.mode=unifabric-roce` | Supported |
| Ethernet switch that does not send LLDP on the ports to the nodes | None | None | Not supported |

### Storage network

| Network device | Discovery method | Helm configuration | Support status |
| --- | --- | --- | --- |
| Ethernet RoCE switch that runs SONiC and can run switch-agent | [LLDP discovery in RoCE networks](./roce-lldp-discovery.md) | `topoDiscovery.storage.mode=unifabric-roce` (default), and a selector for the storage NICs | Supported |
| NVIDIA Spectrum-X switch that NetQ manages | Unifabric cannot yet identify the Storage network in the topology from NVIDIA NetQ discovery in RoCE networks. Use [LLDP discovery in RoCE networks](./roce-lldp-discovery.md) (not verified) or [Custom network topology](./custom-network-topology.md) instead. | `topoDiscovery.storage.mode=unifabric-roce` | Not supported yet |
| NVIDIA Spectrum-X switch without NetQ | [LLDP discovery in RoCE networks](./roce-lldp-discovery.md) | `topoDiscovery.storage.mode=unifabric-roce` | Not verified |
| NVIDIA InfiniBand switch | Unifabric cannot yet identify the Storage network in the topology from [InfiniBand network discovery](./infiniband-discovery.md). InfiniBand NICs receive no LLDP, so the nodes cannot find their leaf. For this reason, LLDP discovery in RoCE networks and Custom network topology do not work. | None | Not supported yet |
| Ethernet switch that sends LLDP on the ports to the nodes, but cannot run switch-agent | [Custom network topology](./custom-network-topology.md) | `topoDiscovery.storage.mode=unifabric-roce` | Supported |
| Ethernet switch that does not send LLDP on the ports to the nodes | None | None | Not supported |

### Scale-up network

The Scale-up network is the high-speed interconnect between GPUs, for example NVIDIA NVLink.
Its hardware is the GPU interconnect itself.
It does not depend on the switches of the Scale-out network and the Storage network.

Only a multi-node NVLink system needs Scale-up topology discovery.
For example, GB200 NVL72 uses NVLink Switches to connect the 72 GPUs in one rack into one NVLink domain.
This domain spans many Kubernetes nodes.
In a single-node NVLink system, such as HGX with 8 GPUs, one node is one NVLink domain, and no discovery is necessary.

| GPU interconnect | Discovery method | Helm configuration | Support status |
| --- | --- | --- | --- |
| Single-node NVLink, for example HGX with 8 GPUs | Not necessary | `topoDiscovery.scaleUp.mode=manual` (default) | Not applicable |
| Multi-node NVLink, and NetQ manages the NVLink domain | [NVLink domain discovery through NVIDIA NetQ](./roce-netq-discovery.md) | `topoDiscovery.scaleUp.mode=nv-topograph`, `nvidiaTopograph.provider.name=netq` | Supported |
| Multi-node NVLink, and the Scale-out network is InfiniBand | [InfiniBand network discovery](./infiniband-discovery.md). It reads the `nvidia.com/gpu.clique` label of the GPU Operator. | `topoDiscovery.scaleUp.mode=nv-topograph`, `nvidiaTopograph.provider.name=infiniband-k8s` | Supported |
| Multi-node NVLink without NetQ, and the Scale-out network is not InfiniBand | An administrator writes the labels | `topoDiscovery.scaleUp.mode=manual` | Not supported yet |
| GPU interconnect that is not from NVIDIA | An administrator writes the labels | `topoDiscovery.scaleUp.mode=manual` | Not supported yet |

## Discovery methods

The methods differ only in the topology source.
The result is the same, and you view it in [Verification](#verification).
For the principle, the requirements, the limits, and the configuration steps of each method, see its document.

| Discovery method | Applicable networks | Principle |
| --- | --- | --- |
| [LLDP discovery in RoCE networks](./roce-lldp-discovery.md) | Scale-out, Storage | The nodes read the LLDP neighbors of their NICs. The switch-agent on each switch reads the LLDP neighbors of the switch. The Controller combines both to calculate the tiers. |
| [Custom network topology](./custom-network-topology.md) | Scale-out, Storage | Node-side LLDP discovers the leaf. Neighbor declarations on `Switch` resources give the spine and the tiers above it. No software runs on the switches. |
| [NVIDIA NetQ discovery in RoCE networks](./roce-netq-discovery.md) | Scale-out, Storage, Scale-up | NVIDIA Topograph reads the fabric topology and the NVLink domains through the NetQ API. |
| [InfiniBand network discovery](./infiniband-discovery.md) | Scale-out, Storage | NVIDIA Topograph runs `ibnetdiscover` on a GPU node to read the topology of the IB subnet. |

## Verification

### View Topology and Node labels

This section uses one cluster as the example.
The three networks have these links:

- The Scale-out network is SONiC RoCE. `node1` and `node2` connect to both `leaf1` and `leaf2`. `node3` and `node4` connect to both `leaf3` and `leaf4`. All four leaf switches connect to `spine1`.
- The Storage network has only one leaf tier. `node1` and `node2` connect to `storage-leaf1`. `node3` and `node4` connect to `storage-leaf2`.
- The Scale-up network has two NVLink domains. `node1` and `node2` are in one domain. `node3` and `node4` are in the other domain.

![Example topology of the three networks](../../images/topology-example-all-fabrics.png)

The NVLink domain names in the figure are short forms.
The full format is `ClusterUUID.CliqueId`.
See the example of the `scaleup` resource below.

After discovery completes, view the labels of the Scale-out network:

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

`node1` and `node2` have the same `tier-1` value, so they are in the same leaf domain.
All four nodes have the same `tier-2` value, so they share one spine domain.
If a node has `tier-1` but no `tier-2`, Unifabric did not discover the tiers above the leaf.
See [Troubleshooting](#troubleshooting).

The Storage network and the Scale-up network use the same model, and each has its own label prefix.
In the example, the Storage network has only one leaf tier:

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

For the Scale-up network, run `kubectl get nodes -L scale-up.unifabric.io/tier-1`.
The value is the NVLink domain ID.
Topograph gets it from the GPU Operator clique label or from NetQ.
Nodes with the same value share one NVLink domain.

The label model of the three networks:

| Node label | Meaning |
| --- | --- |
| `scale-out.unifabric.io/tier-1` | The leaf switch domain of the Scale-out network that the node connects to directly |
| `scale-out.unifabric.io/tier-2` | The spine domain of the Scale-out network above the leaf |
| `scale-out.unifabric.io/tier-3` | The core domain of the Scale-out network above the spine |
| `storage.unifabric.io/tier-N` | The matching tier of the Storage network. The rules are the same as for the Scale-out network. |
| `scale-up.unifabric.io/tier-1` | The NVLink domain that the node is in |

A higher tier number means a tier closer to the top of the network.
When two nodes have the same value for a label, they are in the same performance domain.
LLDP discovery in RoCE networks names the domains `tier<N>-group<M>`.
Topograph uses its own names, for example `S-fc6a1c03006636c0` for an InfiniBand switch.

The Helm value `topoDiscovery.<fabric>.nodeLabel.keyTemplate` defines the label key.
The defaults are `scale-out.unifabric.io/tier-{{ .Tier }}`, `storage.unifabric.io/tier-{{ .Tier }}`, and `scale-up.unifabric.io/tier-{{ .Tier }}`.
You can change the template, but it must contain exactly one `{{ .Tier }}`.

> Note: If you change the key template, change the label keys in the scheduler configuration too. If you do not, the scheduler reads no topology information.

Each network has one `Topology` resource: `scaleout`, `storage`, and `scaleup`.
It summarizes the hierarchy of the performance domains and the domain path of each node:

> Note: Multi-plane Scale-out networks, for example with 2, 4, or 8 planes, are not supported yet.
> All planes map to one `scaleout` resource and one set of `scale-out.unifabric.io/tier-N` labels.
> If a node connects to more than one plane, the switches of all planes are merged into the same domains, and the result does not show the topology of each plane.
> Support for multi-plane networks is planned for a later release.

```bash
kubectl get topologies.unifabric.io
```

```text
NAME       AGE
scaleout   30d
scaleup    30d
storage    30d
```

In `status.domains`, `switchMember` lists the switches in a domain.
`parent` points to the domain one tier above.
The domain at the highest tier has no `parent`.
In `status.nodes`, each group of nodes shares one domain path `switchDomainPath`, from top to bottom.
`name` is the stable identifier of the group.
For the full field description, see the [Topology reference](../../reference/topology.md).

The Scale-out network. The example has two tiers, leaf and spine:

```bash
kubectl get topologies.unifabric.io scaleout -o yaml
```

```yaml
apiVersion: unifabric.io/v1beta1
kind: Topology
metadata:
  name: scaleout
status:
  domains:
    - name: tier2-group1
      switchMember:
        - spine1
      tier: 2
    - name: tier1-group1
      parent: tier2-group1
      switchMember:
        - leaf1
        - leaf2
      tier: 1
    - name: tier1-group2
      parent: tier2-group1
      switchMember:
        - leaf3
        - leaf4
      tier: 1
  nodes:
    - name: node-group1
      nodes:
        - node1
        - node2
      switchDomainPath:
        - tier2-group1
        - tier1-group1
    - name: node-group2
      nodes:
        - node3
        - node4
      switchDomainPath:
        - tier2-group1
        - tier1-group2
```

The Storage network. The example has only one leaf tier.
Tier 1 is the highest tier, so the domains have no `parent`:

```bash
kubectl get topologies.unifabric.io storage -o yaml
```

```yaml
apiVersion: unifabric.io/v1beta1
kind: Topology
metadata:
  name: storage
status:
  domains:
    - name: tier1-group1
      switchMember:
        - storage-leaf1
      tier: 1
    - name: tier1-group2
      switchMember:
        - storage-leaf2
      tier: 1
  nodes:
    - name: node-group1
      nodes:
        - node1
        - node2
      switchDomainPath:
        - tier1-group1
    - name: node-group2
      nodes:
        - node3
        - node4
      switchDomainPath:
        - tier1-group2
```

The Scale-up network. It has only tier 1, which is the NVLink domain.
The domain name is the value of the `scale-up.unifabric.io/tier-1` label.
When Topograph writes it, the format is `ClusterUUID.CliqueId`.
The Scale-up network has no `Switch` resources, so it has no `switchMember`:

```bash
kubectl get topologies.unifabric.io scaleup -o yaml
```

```yaml
apiVersion: unifabric.io/v1beta1
kind: Topology
metadata:
  name: scaleup
status:
  domains:
    - name: 7b968a6d-c8aa-45e1-9e07-e1e51be99c31.1
      tier: 1
    - name: 7b968a6d-c8aa-45e1-9e07-e1e51be99c31.2
      tier: 1
  nodes:
    - name: node-group1
      nodes:
        - node1
        - node2
      switchDomainPath:
        - 7b968a6d-c8aa-45e1-9e07-e1e51be99c31.1
    - name: node-group2
      nodes:
        - node3
        - node4
      switchDomainPath:
        - 7b968a6d-c8aa-45e1-9e07-e1e51be99c31.2
```

#### Use the Node labels in a scheduler

A topology-aware scheduler arranges the Node labels into tiers from top to bottom.
It then places the Pods of one job tier by tier.
The table maps the Unifabric labels to the configuration entry of each scheduler.
For the installation and configuration of the scheduler itself, follow its official documentation:

| Scheduler | Configuration entry | Unifabric labels to fill in |
| --- | --- | --- |
| Kueue Topology Aware Scheduling | `spec.levels` of the `Topology` resource | From top to bottom: `scale-out.unifabric.io/tier-3`, `tier-2`, `tier-1`, then `kubernetes.io/hostname` |
| Volcano network topology aware scheduling | The `HyperNode` resource | Create one HyperNode for each tier. The member selector refers to the matching label. |
| KAI Scheduler topology aware scheduling | `spec.levels` of the `Topology` resource | The same as Kueue |

References: [Kueue TAS](https://kueue.sigs.k8s.io/docs/concepts/topology_aware_scheduling/),
[Volcano network topology](https://volcano.sh/en/docs/network_topology_aware_scheduling/),
[KAI Scheduler](https://github.com/NVIDIA/KAI-Scheduler).

### View topology visualization

The Grafana topology dashboard renders each `Topology` resource as an interactive graph.
The graph matches the Node labels above.
Each tier 1 domain is a leaf group.
Each tier 2 domain is a spine group above the leaf groups.
Each node is under the leaf group that it belongs to.

![Topology visualization dashboard](../../images/topology-visualization.png)

For the dashboard installation, the connection to an external Grafana, and the details of the graph, see [Topology visualization](./topology-visualization.md).

## Troubleshooting

### A Node has no topology labels

First make sure that the prerequisites are met:

1. For LLDP discovery in RoCE networks and Custom network topology, make sure that the NICs in the `FabricNode` received LLDP neighbors:

   ```bash
   kubectl get fabricnode node1 -o jsonpath='{range .status.scaleOutNics[*]}{.name}{"\t"}{.lldpNeighbor.hostname}{"\t"}{.lldpNeighbor.port}{"\n"}{end}'
   ```

   ```text
   eth1	leaf1	Ethernet0
   eth2	leaf2	Ethernet0
   ```

   If the hostname column is empty, make sure that LLDP is enabled on the switch.
   Also make sure that the switch port of the node NIC sends LLDP frames.
   The `LLDPNeighborsReady` condition of the `FabricNode` also reports the reason `LLDPNeighborMissing`.

2. For NVIDIA NetQ discovery in RoCE networks and InfiniBand network discovery, make sure that `mode` is `nv-topograph` and that the Topograph Deployment runs:

   ```bash
   kubectl -n unifabric-system get deployment unifabric-nvidia-topograph
   kubectl -n unifabric-system logs deployment/unifabric-nvidia-topograph --tail 100
   ```

   For NVIDIA NetQ discovery in RoCE networks, examine the NetQ credentials and the API address.
   Make sure that the Topograph Pod can connect to the NetQ API.
   For InfiniBand network discovery, make sure that the log shows that `ibnetdiscover` runs correctly.

3. Read the Controller log:

   ```bash
   kubectl -n unifabric-system logs deployment/unifabric-controller --tail 200
   ```

### Node labels have only tier 1

Node-side LLDP discovers only the leaf tier.
This symptom means that the information above the leaf tier did not get to the Controller:

- LLDP discovery in RoCE networks: Make sure that every switch has a `Switch` resource. Also make sure that `HEALTHY` is `true` in `kubectl get switch -o wide`. If it is `false`, make sure that the node that runs the Controller can connect to port `8090` of the switch. Also make sure that the mTLS certificates match. See [LLDP discovery in RoCE networks](./roce-lldp-discovery.md#troubleshooting).
- Custom network topology: Make sure that each switch at the spine tier and above has a `Switch` resource. Also make sure that each one has the `unifabric.io/switch-neighbors` annotation. See [Custom network topology](./custom-network-topology.md#troubleshooting).
- If the physical network has only one leaf tier, tier 1 alone is the expected result.

### Switch members are empty in topology visualization

This is the expected behavior for NVIDIA NetQ discovery in RoCE networks and InfiniBand network discovery.
See [Create Switch instances](./topograph-switch-resources.md).

## References

- [Topology discovery concepts](../../concepts/topology-discovery.md): logical topology, discovery methods, and discovery results.
- [LLDP discovery in RoCE networks](./roce-lldp-discovery.md), [Custom network topology](./custom-network-topology.md), [NVIDIA NetQ discovery in RoCE networks](./roce-netq-discovery.md), [InfiniBand network discovery](./infiniband-discovery.md): the principle, the requirements, and the configuration of each discovery method.
- [Create Switch instances](./topograph-switch-resources.md): the optional step for NVIDIA NetQ discovery in RoCE networks and InfiniBand network discovery.
- [Topology visualization](./topology-visualization.md): the installation and the reading of the Grafana dashboard.
- [Topology reference](../../reference/topology.md), [FabricNode reference](../../reference/fabricnode.md), [Switch reference](../../reference/switch.md).
- [Scale-out topology discovery design](../../design/scaleout-topology.md), [Topology CRD design](../../design/topology-crd.md).
- [Helm values reference](../../../chart/README.md): all `topoDiscovery.*` parameters.
