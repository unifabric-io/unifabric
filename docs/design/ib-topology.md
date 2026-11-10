# InfiniBand Topology Discovery (unifabric-ib)

## Overview

This document describes a built-in InfiniBand topology discovery mode named `unifabric-ib` for `topoDiscovery.scaleOut.mode` and `topoDiscovery.storage.mode`. The Unifabric Agent discovers the IB switch fabric with `ibnetdiscover` and writes the parsed switches and their connections into the existing `Switch` CRs. The Unifabric Controller classifies each IB subnet as scale-out or storage based on NIC roles, reuses the existing path to compute leaf, spine, and core tiers, and writes the results into Node labels and the `Topology` CRD (aggregated from Node labels by the topology status controller for visualization); these flows stay unchanged.

The discovery and parsing logic follows the NVIDIA Topograph InfiniBand provider, including the multi-port probing introduced in [dsx-ai-factory/topograph#560](https://github.com/dsx-ai-factory/topograph/pull/560). Unifabric extends that design in one key way: Topograph selects a single compute fabric and publishes one set of fabric labels, while `unifabric-ib` classifies subnets into both scale-out and storage roles and publishes both label sets in one pass.

## Motivation

Unifabric currently supports two sources for scale-out topology labels: `unifabric-roce` (built-in LLDP discovery through the switch agent) and `external` (an external system such as NVIDIA Topograph writes the labels). Storage topology only supports `unifabric-roce`. Neither existing path covers InfiniBand clusters:

1. `unifabric-roce` depends on Ethernet LLDP: the switch agent collects LLDP neighbors and the discovery path (`pkg/agent/fabricnode`, `pkg/controller/switchtopology`) computes the topology from them. InfiniBand fabrics do not provide LLDP neighbor information, so this path has no input on IB clusters.
2. The `external` mode can delegate discovery to Topograph, but Topograph's scale-out detection supports only a single network type. In clusters where compute and storage use separate IB subnets (for example, each host has one compute IB NIC and one storage IB NIC), the storage fabric topology may not be fully discovered. For such scenarios, the options are extending Topograph's detection or implementing discovery in Unifabric itself.

This design therefore adds a built-in discovery mode, `unifabric-ib`, which gives IB clusters the same complete flow that RoCE clusters already have: automatic discovery, Node labels for topology-aware scheduling, and `Topology` CRDs for visualization, covering both the compute and the storage fabric.

### Goals

- Add `unifabric-ib` as a mode for `topoDiscovery.scaleOut.mode` and `topoDiscovery.storage.mode`, independently selectable per section.
- Discover the full IB switch tree (leaf, spine, core, any number of tiers) from `ibnetdiscover` output.
- Reuse the classification already produced by the existing NIC selectors to deterministically map different IB subnets to the scale-out and storage roles, without introducing new classification configuration.
- Reuse the existing label templates (`scale-out.unifabric.io/tier-{N}`, `storage.unifabric.io/tier-{N}`) and the existing `Topology` aggregation flow unchanged.

### Non-Goals

- Scale-up (NVLink) discovery. `topoDiscovery.scaleUp.mode` stays `external`: on MNNVL systems, the NVLink domain information is written into Node labels (`nvidia.com/gpu.clique`) by GPU Feature Discovery of the GPU Operator; see the [GPU Feature Discovery documentation](https://github.com/NVIDIA/k8s-device-plugin/blob/main/docs/gpu-feature-discovery/README.md).

## Proposal

The proposal consists of four parts.

1. **NIC reporting.** The Agent reuses the existing NIC reporting logic: IB NICs enter `scaleOutNics`/`storageNics` in `FabricNode.status` like any other RDMA NIC (including the existing `state` field; inactive ports are reported as usual). Only two optional fields are added to `NicInfo`: `linkLayer` reuses the existing `LinkLayer` enum from `RDMAEndpoint` (`InfiniBand`/`Ethernet`, taken from sysfs `link_layer`), and the IB discovery path only processes NICs with `InfiniBand`; `gids` reports the non-zero entries of the port GID table in ascending table-index order (colon-separated sysfs form, consistent with the GID string format of `RDMAEndpoint`), so the first element is `gids/0`:

   ```yaml
   apiVersion: unifabric.io/v1beta1
   kind: FabricNode
   metadata:
     # Same name as the Kubernetes Node, cluster scoped.
     name: node-a
   status:
     nodeRole: GPU
     nodeIP: 10.0.0.11
     totalNics: 2
     scaleOutNics:
       - name: ib0
         rdmaDeviceName: mlx5_0
         rdma: true
         state: up
         ipv4: ""
         ipv6: ""
         # New optional field: link layer type, enum InfiniBand/Ethernet.
         linkLayer: InfiniBand
         # New optional field: non-zero entries of the port GID table in
         # ascending table-index order; the first element is gids/0. The
         # controller takes the first GID: the upper 64 bits are the subnet
         # prefix, used to associate the subnet with the NIC role; the lower
         # 64 bits are the port GUID, which maps directly to H-<guid> in the
         # ibnetdiscover output.
         # Why gids/0: per IBTA InfiniBand Architecture Specification
         # Volume 1 §4.1.1, GID table index 0 is the port GID
         # ({SubnetPrefix[63:0], GUID[63:0]}) and is invariant; it cannot be
         # deleted or arbitrarily rewritten.
         gids:
           - "fe80:0000:0000:0000:a088:c203:0033:f66a"
     storageNics:
       - name: ib1
         rdmaDeviceName: mlx5_1
         rdma: true
         state: up
         ipv4: ""
         ipv6: ""
         linkLayer: InfiniBand
         gids:
           - "fe80:0000:0000:0001:a088:c203:0033:f66b"
   ```

2. **Subnet scanning and writing `Switch` CRs.** Performed by the elected reporter node; the parsed results are written directly into existing CRDs, with no new reporting structure.

   - Election: `ibnetdiscover` sees the entire subnet from any member port, so each subnet needs only one successful collection. The purpose of the election is to avoid a scan storm: every `ibnetdiscover` run sweeps all switches and ports in the subnet. Without election, if every node scanned on every NIC, a 122-node cluster with 8 IB NICs per node would produce nearly a thousand full-fabric scans per collection cycle, repeatedly querying the same devices and putting needless pressure on the subnet management plane (opensm and the SMAs of each device). The review discussion of topograph#560 pointed out the same class of problem; the concurrency limit in that PR only bounds per-node concurrency and does not reduce the total number of scans. With election, each subnet is scanned once per cycle, and the management-plane load stays constant as the cluster grows. The reporter node is elected through `coordination.k8s.io/Lease`: one Lease object per subnet, placed in the unifabric system namespace, with the name derived from the subnet prefix (the prefix contains `:` and must be converted to a valid object name, e.g. `ib-reporter-fe80-0000-0000-0000`). Agents on nodes that have an active port in that subnet compete for the Lease using the client-go leaderelection package, with `holderIdentity` set to the node name. Only the Agent holding the Lease performs the scan, following the existing Agent resync period with added jitter. After renewal timeout the Lease can be taken over by another candidate node, so failover needs no Controller involvement; a node attached to multiple subnets holds or competes for multiple Leases independently.
   - Scanning: the Agent lists active IB ports from `/sys/class/infiniband/*/ports/*/state` (state contains `ACTIVE`; CA names are validated against `^[A-Za-z0-9_.-]+$` and port numbers against `^[0-9]+$`), groups them by the subnet prefix of `gids/0`, and runs `ibnetdiscover --Ca <ca> --Port <port>` on one port per subnet. `ibnetdiscover` scans only the subnet of the specified port in one run; probing per port instead of only the default port ensures both the storage and the compute subnet are scanned, so a storage subnet on the default port cannot hide the compute subnet (the core fix of topograph#560).
   - Parsing: follows the line formats of Topograph `pkg/ib`. `Switch N "S-<guid>" # "<description>" ...` opens a switch block; the switch name is extracted from descriptions such as `MF0;IB-ComputeLeaf-01:MQM8700/U1` (the segment between `;` and `:`), and descriptions without ` ;:` separators are used verbatim. Inside a switch block, `[port] "S-<guid>"[port] # "<description>"` and `[port] "H-<guid>"[port] # "<description>"` lines record switch-to-switch and switch-to-host-NIC connections respectively; `Ca N "H-<guid>" # "node-a mlx5_0"` maps a host NIC GUID to a node name (the first whitespace-separated field).
   - Writing: one `Switch` CR per IB switch. The ibnetdiscover connectivity is isomorphic to LLDP neighbors, so `SwitchNeighbor` is reused directly (`RemoteSystemType: KubernetesNode|Switch` + `RemoteSystemName` + `LinkCount`). `Switch` already supports switches without `mgmtIP` participating in topology; IB switches need no switch-agent access, which is exactly this shape. NIC GUIDs, ports, LIDs, and link speeds are dropped at write time; only connections whose node names match cluster Nodes are kept (the same rule as Topograph: unknown hosts do not enter leaf groups). One CR per switch scales naturally with the fabric and avoids single-object size limits; `Switch` objects labeled `unifabric.io/discovered-by: unifabric-ib` that no longer appear in collection results are garbage-collected by the controller on a TTL. Example:

   ```yaml
   apiVersion: unifabric.io/v1beta1
   kind: Switch
   metadata:
     # Parsed switch name; falls back to the GUID when no name can be parsed.
     name: ib-computeleaf-01
     labels:
       # Marks the source, distinguishes from LLDP-attached switches, and
       # lets the controller filter and garbage-collect.
       unifabric.io/discovered-by: unifabric-ib
     annotations:
       # Switch GUID and owning subnet prefix, used for cross-collection
       # merging and deduplication.
       unifabric.io/ib-switch-guid: "S-0000000000000011"
       unifabric.io/ib-subnet-prefix: "fe80:0000:0000:0000"
   # IB-sourced switches do not set spec.role; it stays empty. The network
   # role is subnet-level information, classified by the
   # IBTopologyDiscoveryController from NIC roles and used directly in tier
   # computation.
   spec: {}
   status:
     # Reuses the existing neighbor structure: switch-to-switch and
     # switch-to-node connections.
     # Note: the field names carry an LLDP prefix, but IB-sourced neighbors
     # do not come from LLDP. Whether to refactor lldpNeighbors to neighbors
     # and lldpNeighborCount to neighborCount (keeping the old fields for a
     # compatibility transition) is to be discussed in the meeting.
     lldpNeighbors:
       - remoteSystemType: Switch
         remoteSystemName: ib-computespine-01
         linkCount: 1
       - remoteSystemType: KubernetesNode
         remoteSystemName: node-a
         linkCount: 1
       - remoteSystemType: KubernetesNode
         remoteSystemName: node-b
         linkCount: 1
     lldpNeighborCount: 3
   ```

3. **Classification.** A new `IBTopologyDiscoveryController` in the Controller watches IB-sourced `Switch` and `FabricNode` objects and recomputes when collection results change, classifying each subnet as scale-out or storage. Classification is subnet-level information and is not written into `Switch.spec.role`; IB-sourced switches keep an empty role.

   - Merging and deduplication: collections of the same subnet from different reporter nodes are deduplicated by subnet prefix and switch GUID with a consistency check; if two collections give conflicting leaf groups for the same node, the reconcile fails by reporting a condition instead of writing ambiguous labels (the same rule as topograph#560).
   - Classification rule: the subnet role directly inherits the role of its member NICs. The member host ports of each subnet are already classified into `scaleOutNics` or `storageNics` in `FabricNode.status` by the NIC selectors; the first element of `gids` (that is, `gids/0`) associates the subnet with the NIC role (the upper 64 bits, the subnet prefix, determine subnet membership; the lower 64 bits, the port GUID, map exactly to `H-<guid>` in `ibnetdiscover`). Subnets whose member NICs appear in `storageNics` are classified as storage, and those in `scaleOutNics` as scale-out. No new classification configuration is added.
   - Error handling: a subnet whose member NICs appear in both `scaleOutNics` and `storageNics`, or a `unifabric-ib` section that matches no subnet, is reported as an error through a condition.

4. **Tier computation and labels.** After classification, the `Switch` neighbor relations plus `FabricNode` provide the same input as the existing `SwitchTopologyDiscoveryController`; tier computation runs per subnet, and Node label writing follows the existing flow and ownership rules (`manageScaleOut`/`manageStorage`), ensuring `unifabric-ib` and `unifabric-roce` never fight over the same label set. `topologystatus` already lists Nodes by labels regardless of who wrote them, so the `Topology` CRD and visualization need no changes.

### Configuration

```yaml
topoDiscovery:
  scaleOut:
    mode: unifabric-ib
    nodeLabel:
      keyTemplate: "scale-out.unifabric.io/tier-{{ .Tier }}"

  storage:
    mode: unifabric-ib
    nodeLabel:
      keyTemplate: "storage.unifabric.io/tier-{{ .Tier }}"
```

- `unifabric-ib` introduces no new values configuration: compute and storage NICs are distinguished by the existing NIC selectors (`fabricNode.scaleOutInterfaceSelector`/`storageInterfaceSelector`), and subnet classification directly inherits the NIC roles.
- The two sections can use `unifabric-ib` independently; on hosts with an IB compute fabric and an Ethernet storage fabric, `scaleOut: unifabric-ib` with `storage: unifabric-roce` is a valid combination.
- Nodes need `ibnetdiscover` (standard `infiniband-diags` package); the Agent image bundles the tool, so no host installation is required.

### User Stories

#### Story 1: Compute and storage IB fabrics in one cluster

As an administrator of a GPU cluster running separate compute and storage InfiniBand subnets, I set both modes to `unifabric-ib`; compute and storage NICs are already distinguished by the existing NIC selectors, so no new configuration is needed. Unifabric labels every node with its position in both fabrics, Kueue schedules on the scale-out labels, the storage fabric is visible as a `Topology` CRD, and no external system is deployed.

### Constraints and Considerations

- Discovery runs `ibnetdiscover` with bounded concurrency and only on elected reporter nodes, so the SMP load on the fabric stays constant as the cluster grows.
- Discovery results reuse the two existing CRDs, `Switch` and `FabricNode`, with no new reporting structure; one `Switch` CR per switch scales naturally with the fabric.
- Classification directly inherits NIC roles, which requires the NIC selectors to distinguish compute NICs from storage NICs; a single IB subnet carrying both compute and storage traffic cannot be split into two roles.
- Parsing stays behavior-compatible with Topograph `pkg/ib`; upstream test data (for example `tests/output/ibnetdiscover/compute-storage.out`) can be reused as test vectors.

### Risks and Mitigations

- **Conflicting collections.** When two links report different leaf groups for the same node, the reconcile fails with a condition and no incorrect labels are written (rule taken from topograph#560).
- **Reporter node failure.** When the current reporter stops reporting, the election picks a new one; IB-sourced `Switch` objects not refreshed by collection for too long are garbage-collected on a TTL, and their labels are drained by the existing label writer.
- **Link degradation or flapping.** Ports leaving the ACTIVE state drop out of enumeration; the last consistent collection is kept within the TTL to avoid label flapping.

## Implementation Steps

Ordered by dependency into Agent, Controller, and testing parts.

1. **Agent: NIC reporting extension.** Add the two optional fields `linkLayer` and `gids` to `NicInfo`; the collection logic joins the existing NIC reporting path, reading `link_layer` and the GID table from sysfs. Original work.
2. **Agent: IB port enumeration.** Enumerate active ports from `/sys/class/infiniband/*/ports/*/state` and group them by `gids/0` prefix. Original work.
3. **Agent: per-subnet Lease election.** One `coordination.k8s.io/Lease` per subnet, competed for with client-go leaderelection; the holder performs the scan. Original work (Topograph has a centralized server architecture and no such mechanism).
4. **Agent: `ibnetdiscover` execution and parsing.** The Lease-holding Agent runs `ibnetdiscover --Ca --Port` once per subnet and parses switch blocks, connection lines, and `Ca` lines. The parser implementation references Topograph `pkg/ib` (line formats, switch name extraction, and node name mapping rules stay behavior-compatible).
5. **Agent: writing `Switch` CRs.** Map the parsed results into one `Switch` CR per switch (`SwitchNeighbor` reuse, `discovered-by` label, GUID and subnet prefix annotations). Original work.
6. **Controller: `IBTopologyDiscoveryController`.** Watch IB-sourced `Switch` and `FabricNode`, perform merging, deduplication, consistency checks, and subnet classification. Original work.
7. **Controller: tier computation, labels, and TTL garbage collection.** Reuse the existing `SwitchTopologyDiscoveryController` flow and the `manageScaleOut`/`manageStorage` ownership rules; add TTL garbage collection for IB-sourced `Switch` objects. Original work (reusing existing code in this repository).
8. **Testing.** The `ibnetdiscover` test data used by parser unit tests and e2e replay (such as `compute-storage.out`) comes from Topograph `tests/output/ibnetdiscover/`.
9. **License and attribution.** For the result-parsing code (step 4) and test data (step 8) that reference or come from [github.com/dsx-ai-factory/topograph](https://github.com/dsx-ai-factory/topograph) (Apache-2.0): keep the upstream copyright notice (`Copyright NVIDIA CORPORATION & AFFILIATES`) in file headers, and add an attribution pointing to that repository and its LICENSE (repository URL and original file path) alongside the SPDX identifier (`Apache-2.0`); never remove or modify notices required by the upstream LICENSE.

## Test Plan

- Parser unit tests using test data from Topograph, including the dual-subnet `compute-storage` scenario.
- Controller tests: merge conflicts, subnet classification, role-conflict and no-match error conditions, and label ownership with `unifabric-roce`.
- One e2e flow: replay saved `ibnetdiscover` collections through the Agent parsing path on fake nodes and verify both the scale-out and the storage `Topology` CRDs, following the approach of the existing nvidia-topograph integration test scenario.
