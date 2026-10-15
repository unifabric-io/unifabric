# RDMA Metrics Design

中文版本：[rdma-metrics.zh.md](./rdma-metrics.zh.md)

## Summary

The Agent on each node collects RDMA metrics and exposes them through the
controller-runtime Prometheus metrics endpoint. The collector is node-local: it reads
host and Pod RDMA state from Linux sysfs, container namespaces, and ethtool, then
labels every sample with topology and workload metadata from the local `FabricNode`
snapshot. This document records the data sources, the scrape flow, the metric model,
and the design decisions behind them. How to enable scraping, dashboards, and alerts
is in [RDMA metrics usage](../usage/rdma-metrics.md).

## Motivation

RDMA counters live in per-device sysfs trees and in per-Pod network namespaces. A
cluster operator who wants to answer "which workload saturates this NIC" needs those
counters joined with Kubernetes metadata: the Pod, the top-level workload owner, and
the fabric the interface belongs to. Generic node exporters read host sysfs but know
nothing about RDMA Pods, SR-IOV parent interfaces, or the Unifabric scale-out and
storage classification.

The Agent already maintains all of the needed metadata in `FabricNode` (see
[FabricNode design](./fabricnode.md)). The design reuses that snapshot in memory, so
one collector produces host samples and Pod-attributed samples with one consistent
label set.

User stories:

- As an observability engineer, I want RDMA device, port, interface, and priority
  counters per node, so I can build dashboards and alerts on one stable label set.
- As a platform operator, I want samples attributed to Pods and top-level workload
  owners, so I can map traffic and errors to training jobs.
- As a network engineer, I want a `kind` label that separates scale-out and storage
  interfaces, so fabric-level views do not mix traffic classes.

### Goals

- Export node-level RDMA device, port, interface, and priority counters.
- Attribute RDMA metrics to RDMA-enabled Pods when possible.
- Add topology labels such as `kind=scaleOut` or `kind=storage`.
- Provide a stable Prometheus label set for dashboards and alerts.

### Non-Goals

- No cluster-level aggregation. Prometheus aggregates across nodes.
- No historical storage. The collector only serves the current scrape.
- No non-Linux support. Non-Linux builds emit warnings and skip RDMA collection.
- No guarantee of samples for Pods on unsupported container runtimes. Pod attribution
  requires containerd.

## Proposal

The collector runs inside the Agent and registers with the controller-runtime metrics
registry. Each Prometheus scrape triggers one collection pass. The pass reads only
local inputs:

- The `FabricNode` in-memory snapshot from `fabricnode.Interface.GetFabricNode()`.
- `FabricNode.status.rdmaPods` for RDMA Pod names, container IDs, host RDMA mode, and
  top-level owner labels.
- `/sys/class/infiniband` for RDMA devices, ports, `counters`, `hw_counters`, and
  traffic class.
- `/sys/class/net` for interface speed, MTU, oper state, netdev-to-PCI mapping, and
  PF/VF parent resolution.
- `/host/proc/<pid>/ns/mnt` and `/host/proc/<pid>/ns/net` for host and Pod namespace
  entry.
- `/host/run/containerd/io.containerd.runtime.v2.task/k8s.io/<container-id>/init.pid`
  for containerd Pod init PIDs.
- ethtool statistics for per-priority pause and discard counters.

Each `Collector.Collect()` call:

1. Fetches the current `FabricNode` snapshot.
2. Collects host samples first.
3. Keeps a copy of the host samples for host RDMA Pod attribution.
4. Collects Pod samples from `FabricNode.status.rdmaPods`.
5. Logs warnings at debug level.
6. Emits `unifabric_node_info` and all RDMA samples.

If there is no current `FabricNode`, the scrape returns an empty snapshot.

## Detailed Design

### Host metrics

Host scraping enters the host mount namespace when configured, then scans
`/sys/class/infiniband`. For each RDMA device, the collector:

1. Resolves the netdev name from `gid_attrs/ndevs` first, then falls back to PCI
   device mapping.
2. Resolves the PF interface for VF-backed devices.
3. Records device inventory with device name, provider, interface, parent interface,
   and ports.
4. Reads `hw_counters` and `counters` for each port.
5. Reads interface metrics: `port_speed_mbps`, `port_mtu`, and `port_oper_state`.
6. Reads the device traffic class as `rdma_device_tos`.
7. Reads ethtool priority counters for root interfaces only.

`port_xmit_data` and `port_rcv_data` values are multiplied by 4 while reading. This
matches the kernel counter unit conversion.

### Pod metrics

Pod scraping depends on `FabricNode.status.rdmaPods`.

For host RDMA Pods, the collector duplicates the host samples as Pod-scoped samples
and attaches the workload labels from the Pod entry. `rdma_device_tos` is not
duplicated for host RDMA Pods.

For non-host RDMA Pods, the collector:

1. Resolves each containerd container ID to its init PID.
2. Enters the Pod mount namespace.
3. Reads the Pod-visible `/sys/class/infiniband` tree.
4. Collects RDMA counters, interface metrics, traffic class, and ethtool priority
   counters from the Pod namespace.
5. Stops after the first container that yields samples.

Unsupported container runtimes are skipped with a warning because `containerInitPID()`
currently supports `containerd://` IDs only.

### Metric model

Every RDMA sample is a Prometheus gauge. Metric names receive the `unifabric_` prefix
unless the sample name already has it. Examples:

- `unifabric_port_rcv_data`
- `unifabric_port_xmit_packets`
- `unifabric_port_speed_mbps`
- `unifabric_port_mtu`
- `unifabric_port_oper_state`
- `unifabric_rdma_device_tos`
- `unifabric_rx_pause`
- `unifabric_tx_pause`
- `unifabric_rx_discards`
- `unifabric_tx_discards`

The collector also emits `unifabric_node_info{unifabric_node_name="<node>"} 1`.

All RDMA samples share this label set:

| Label | Meaning |
| --- | --- |
| `node_name` | FabricNode name. |
| `device` | RDMA device, for example `mlx5_0` or `rxe_eth1`. |
| `ifname` | Interface associated with the RDMA device. |
| `parent_ifname` | PF interface for VF-backed devices. Same as `ifname` for root devices. |
| `port` | RDMA port name when applicable. |
| `priority` | Traffic priority for ethtool priority counters. |
| `pod_name`, `pod_namespace` | Set for Pod-attributed samples. |
| `topowner_*` | Top-level workload owner from `FabricNode.status.rdmaPods`. |
| `host_rdma` | Whether the attributed Pod uses host RDMA mode. |
| `is_root` | Whether `ifname == parent_ifname`. |
| `kind` | `scaleOut`, `storage`, `scaleUp`, or empty. |
| `scope` | `host` or `pod`. |
| `source` | `hw_counters`, `counters`, `interface`, `device`, or `ethtool`. |

### Interface kind labeling

`kind` is derived from the `fabricNode` selectors, in this order:

1. Match `storageInterfaceSelector`.
2. Match `scaleUpInterfaceSelector`.
3. Match `scaleOutInterfaceSelector`.
4. If no scale-out selector is configured and the interface was not matched above,
   default to `scaleOut`.

Matching checks `parent_ifname` first, then `ifname`. Supported selector forms are
`interface=eth*,!eth9` and `cidr=172.17.0.0/16`.

### Kubernetes exposure

The Agent metrics endpoint is configured by `agent.config.metrics.bindAddress`, with
default `:8082`. The Helm chart can render an Agent metrics `Service`, a
`ServiceMonitor`, and RDMA Grafana dashboards, gated by `nodeMetrics` and
`grafanaDashboard` values. One implementation detail: the Go Agent always registers
the RDMA collector when the Agent starts. `nodeMetrics.enabled` controls the Helm
resources around scraping and dashboards, not collector registration inside the
Agent. The enable steps and verification commands are in
[RDMA metrics usage](../usage/rdma-metrics.md).

### Failure handling

RDMA scraping is best-effort:

- Missing optional counter directories are ignored.
- Permission and non-existent errors in counter directories are not fatal.
- Invalid or unreadable counter values are skipped.
- Namespace, PID, sysfs, and ethtool failures are recorded as warnings and logged at
  debug level.
- A scrape error prevents metric emission for that collection but does not stop the
  Agent.

### Operational requirements

- Linux nodes with RDMA devices visible under `/sys/class/infiniband`.
- Agent access to host `/proc` and containerd runtime state as mounted by the chart.
- A privileged Agent container for namespace and host device access.
- A containerd runtime for non-host RDMA Pod attribution.
- An up-to-date `FabricNode.status.rdmaPods` for Pod and workload labels.

## Alternatives

The alternatives below are reconstructed from the decisions this design argues
against. They were not recorded at decision time.

- A separate exporter DaemonSet. Rejected because the Agent already holds the
  `FabricNode` snapshot, the privileges, and the host mounts. A second privileged
  DaemonSet duplicates all three.
- Pod attribution through the Kubernetes API at scrape time. Rejected because a
  scrape must not depend on API server latency. The in-memory `FabricNode` snapshot
  makes scrapes local and cheap.
- A runtime-generic attribution layer over CRI. Not implemented. The init-PID lookup
  is containerd-specific, and other runtimes are skipped with a warning. This is a
  known limitation, not a rejected option.
- Emitting sysfs counters as Prometheus counter types. Rejected because the kernel
  values can reset with devices and namespaces that the collector does not control.
  Gauges expose the raw value and leave rate calculation to PromQL.

## Implementation History

- 2026-04-19: Initial implementation with RDMA topology and metrics support.
- 2026-07-17: Updated together with topology status aggregation and automatic label
  discovery.
- 2026-10-13: Document restructured into the design template. The Alternatives section
  is a post-hoc reconstruction.
