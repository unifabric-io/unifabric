## 1. Purpose

RDMA metrics are collected by the Agent on each node and exposed through the controller-runtime Prometheus metrics endpoint.

The implementation is intentionally node-local: the Agent reads host and Pod RDMA state from Linux sysfs, container namespaces, and ethtool, then labels the samples with topology and workload metadata already stored in the local `FabricNode` snapshot.

The main goals are:

- Export node-level RDMA device, port, interface, and priority counters.
- Attribute RDMA metrics to RDMA-enabled Pods when possible.
- Add topology labels such as `kind=scaleOut` or `kind=storage`.
- Provide a stable Prometheus label set for dashboards and alerts.

## 2. Data Sources

The scraper uses these local inputs:

- `FabricNode` in-memory snapshot from `fabricnode.Interface.GetFabricNode()`.
- `FabricNode.status.rdmaPods` for RDMA Pod names, container IDs, host RDMA mode, and top-level owner labels.
- `/sys/class/infiniband` for RDMA devices, ports, `counters`, `hw_counters`, and traffic class.
- `/sys/class/net` for interface speed, MTU, oper state, netdev-to-PCI mapping, and PF/VF parent resolution.
- `/host/proc/<pid>/ns/mnt` and `/host/proc/<pid>/ns/net` for host and Pod namespace entry.
- `/host/run/containerd/io.containerd.runtime.v2.task/k8s.io/<container-id>/init.pid` for containerd Pod init PIDs.
- ethtool stats for per-priority pause and discard counters.

Only Linux implements host and Pod scraping. Non-Linux builds emit warnings and skip RDMA collection.

## 3. Scrape Flow

Each Prometheus scrape calls `Collector.Collect()`:

1. Fetch the current `FabricNode` snapshot.
2. Collect host samples first.
3. Keep a copy of host samples for host RDMA Pod attribution.
4. Collect Pod samples from `FabricNode.status.rdmaPods`.
5. Log warnings at debug level.
6. Emit `unifabric_node_info` and all RDMA samples.

If there is no current `FabricNode`, the scraper returns an empty snapshot.

### 3.1 Host Metrics

Host scraping enters the host mount namespace when configured, then scans `/sys/class/infiniband`.

For each RDMA device:

1. Resolve the netdev name from `gid_attrs/ndevs` first, then fall back to PCI device mapping.
2. Resolve the PF interface for VF-backed devices.
3. Record device inventory with device name, provider, interface, parent interface, and ports.
4. Read `hw_counters` and `counters` for each port.
5. Read interface metrics: `port_speed_mbps`, `port_mtu`, and `port_oper_state`.
6. Read device traffic class as `rdma_device_tos`.
7. Read ethtool priority counters for root interfaces only.

`port_xmit_data` and `port_rcv_data` values are multiplied by 4 while reading, matching the kernel counter unit conversion used by the code.

### 3.2 Pod Metrics

Pod scraping depends on `FabricNode.status.rdmaPods`.

For host RDMA Pods:

- The scraper duplicates host samples as Pod-scoped samples.
- Workload labels are attached from the Pod entry.
- `rdma_device_tos` is not duplicated for host RDMA Pods.

For non-host RDMA Pods:

1. Resolve each containerd container ID to its init PID.
2. Enter the Pod mount namespace.
3. Read the Pod-visible `/sys/class/infiniband` tree.
4. Collect RDMA counters, interface metrics, traffic class, and ethtool priority counters from the Pod namespace.
5. Stop after the first container that yields samples.

Unsupported container runtimes are skipped with a warning because `containerInitPID()` currently supports `containerd://` IDs only.

## 4. Metric Model

Every RDMA sample is emitted as a Prometheus gauge. Metric names are prefixed with `unifabric_` unless the sample name already has that prefix.

Examples:

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

The collector also emits:

- `unifabric_node_info{unifabric_node_name="<node>"} 1`

All RDMA samples share this label set:

| Label | Meaning |
| --- | --- |
| `node_name` | FabricNode name. |
| `device` | RDMA device, for example `mlx5_0` or `rxe_eth1`. |
| `ifname` | Interface associated with the RDMA device. |
| `parent_ifname` | PF interface for VF-backed devices; same as `ifname` for root devices. |
| `port` | RDMA port name when applicable. |
| `priority` | Traffic priority for ethtool priority counters. |
| `pod_name`, `pod_namespace` | Set for Pod-attributed samples. |
| `topowner_*` | Top-level workload owner from `FabricNode.status.rdmaPods`. |
| `host_rdma` | Whether the attributed Pod uses host RDMA mode. |
| `is_root` | Whether `ifname == parent_ifname`. |
| `kind` | `scaleOut`, `storage`, `scaleUp`, or empty. |
| `scope` | `host` or `pod`. |
| `source` | `hw_counters`, `counters`, `interface`, `device`, or `ethtool`. |

## 5. Interface Kind Labeling

`kind` is derived from `fabricNode` selectors:

1. Match `storageInterfaceSelector`.
2. Match `scaleUpInterfaceSelector`.
3. Match `scaleOutInterfaceSelector`.
4. If no scale-out selector is configured and the interface was not matched above, default to `scaleOut`.

Matching checks `parent_ifname` first, then `ifname`. Supported selector forms are:

- `interface=eth*,!eth9`
- `cidr=172.17.0.0/16`

## 6. Kubernetes Exposure

The Agent metrics endpoint is configured by `agent.config.metrics.bindAddress`, defaulting to `:8082`.

The Helm chart can render:

- Agent metrics `Service` when `agent.enabled` and `nodeMetrics.enabled` are true.
- Agent `ServiceMonitor` in the Helm release namespace when `agent.enabled`, `nodeMetrics.enabled`, and `nodeMetrics.serviceMonitor.enabled` are true.
- RDMA Grafana dashboards when `grafanaDashboard.enabled` and `nodeMetrics.enabled` are true.

Current implementation detail: the Go Agent always registers the RDMA collector when the Agent starts. `nodeMetrics.enabled` controls the Helm resources around scraping and dashboards, not collector registration inside the Agent.

## 7. Failure Handling

RDMA scraping is best-effort:

- Missing optional counter directories are ignored.
- Permission and non-existent errors in counter directories are not treated as fatal.
- Invalid or unreadable counter values are skipped.
- Namespace, PID, sysfs, and ethtool failures are recorded as warnings and logged at debug level.
- A scrape error prevents metric emission for that collection, but does not stop the Agent.

## 8. Operational Requirements

- Linux nodes with RDMA devices visible under `/sys/class/infiniband`.
- Agent access to host `/proc` and containerd runtime state as mounted by the chart.
- Privileged Agent container for namespace and host device access.
- Containerd runtime for non-host RDMA Pod attribution.
- `FabricNode.status.rdmaPods` must be up to date for Pod and workload labels.
