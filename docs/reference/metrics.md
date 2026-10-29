# Metrics Reference

中文版: [metrics.zh.md](./metrics.zh.md)

## Overview

This document lists the AI high-speed network traffic observability metrics that the Unifabric Agent exports, and the fields of RDMA send events.
The Agent exports all metrics through the `unifabric-agent-metrics` Service.
For how to use these metrics, see [AI high-speed network traffic observability](../usage/traffic-observability/README.md).

## Host and Pod RDMA NIC metrics

All these metrics are Gauges.
Counter-type metrics keep the total value at scrape time. Use `rate()` or `increase()` to get a rate.

| Metric | Type | Labels | Meaning |
| --- | --- | --- | --- |
| `unifabric_node_info` | Gauge | `unifabric_node_name` | Always 1. The Agent runs on this node. |
| `unifabric_<counter>` | Gauge | [NIC sample labels](#nic-sample-labels) | One counter in `/sys/class/infiniband/<device>/ports/<port>/counters/`, for example `unifabric_port_xmit_data`, `unifabric_port_rcv_data`, and `unifabric_unicast_xmit_packets`. `source` is `counters`. `port_xmit_data` and `port_rcv_data` are converted to bytes. |
| `unifabric_<hw_counter>` | Gauge | [NIC sample labels](#nic-sample-labels) | One counter in `/sys/class/infiniband/<device>/ports/<port>/hw_counters/`, for example `unifabric_np_ecn_marked_roce_packets`, `unifabric_np_cnp_sent`, `unifabric_out_of_sequence`, and `unifabric_local_ack_timeout_err`. `source` is `hw_counters`. The NIC driver supplies these counters, so they differ between NICs. |
| `unifabric_port_speed_mbps` | Gauge | [NIC sample labels](#nic-sample-labels) | The NIC speed, in Mbit/s. `source` is `interface`. |
| `unifabric_port_mtu` | Gauge | [NIC sample labels](#nic-sample-labels) | The NIC MTU. `source` is `interface`. |
| `unifabric_port_oper_state` | Gauge | [NIC sample labels](#nic-sample-labels) | 1 when the NIC state is `up`, otherwise 0. `source` is `interface`. |
| `unifabric_rdma_device_tos` | Gauge | [NIC sample labels](#nic-sample-labels) | The traffic class of the RDMA device. `source` is `device`. Pods in host mode do not have this metric. |
| `unifabric_rx_pause`, `unifabric_tx_pause` | Gauge | [NIC sample labels](#nic-sample-labels) | The pause frames received and sent on one priority. `priority` is the priority. `source` is `ethtool`. On the host, only PFs are collected. |
| `unifabric_rx_discards`, `unifabric_tx_discards` | Gauge | [NIC sample labels](#nic-sample-labels) | The packets discarded on one priority in the receive and send directions. `priority` is the priority. `source` is `ethtool`. On the host, only PFs are collected. |

### NIC sample labels

All NIC metrics except `unifabric_node_info` have these labels. A label that does not apply is empty.

| Label | Meaning |
| --- | --- |
| `node_name` | The node name. |
| `device` | The RDMA device, for example `mlx5_0`. |
| `ifname` | The NIC of the RDMA device. |
| `parent_ifname` | The PF NIC of a VF. For a PF, it is the same as `ifname`. |
| `port` | The RDMA port number. |
| `priority` | The priority of an ethtool priority counter. |
| `pod_namespace`, `pod_name` | The Pod of the sample. Empty when `scope` is `host`. |
| `topowner_api_version`, `topowner_kind`, `topowner_namespace`, `topowner_name` | The top-level workload of the Pod, for example a `PyTorchJob`. |
| `host_rdma` | Whether the Pod uses RDMA in host mode. |
| `is_root` | Whether `ifname` is the same as `parent_ifname`. |
| `kind` | The network of the NIC: `scaleOut`, `storage`, `scaleUp`, or empty. |
| `scope` | `host` for a host NIC sample, `pod` for a Pod sample. |
| `source` | The data source: `counters`, `hw_counters`, `interface`, `device`, or `ethtool`. |

## RDMA tracing metrics

When `agent.ebpfFlow.enabled` is set, the Agent exports these metrics.

| Metric | Type | Labels | Meaning |
| --- | --- | --- | --- |
| `rdma_flow_bytes_total` | Counter | [Flow labels](#flow-labels) | The total bytes that one flow sent. |
| `rdma_flow_wrs_total` | Counter | [Flow labels](#flow-labels) | The total WRs that one flow sent. |
| `rdma_flow_qps` | Gauge | [Flow labels](#flow-labels) | The number of active QPs of one flow now. |
| `rdma_flow_sends_total` | Counter | [Flow labels](#flow-labels) | The total number of sends of one flow. Exported only when `agent.ebpfFlow.flowEvents.enabled` is set. |
| `rdma_flow_send_bytes` | Histogram | [Flow labels](#flow-labels) | The distribution of bytes per send. Exported only when `agent.ebpfFlow.flowEvents.enabled` is set. |
| `rdma_flow_agent_qp_infos` | Gauge | None | The entries in the `qp_infos` BPF map. |
| `rdma_flow_agent_qp_stats` | Gauge | None | The entries in the `qp_stats` BPF map. |
| `rdma_flow_agent_qp_matched` | Gauge | None | The `qp_stats` entries that have a destination in `qp_infos`. |
| `rdma_flow_agent_qp_unmatched` | Gauge | None | The `qp_stats` entries that have no destination in `qp_infos`. |
| `rdma_flow_agent_qp_unresolved` | Gauge | None | The `qp_stats` entries skipped because the source or the destination cannot be resolved to a Pod. |
| `rdma_flow_agent_flows` | Gauge | None | The number of distinct source Pod and destination Pod pairs in the last report. |
| `rdma_flow_agent_send_events_received` | Gauge | None | The send events read from the ring buffer since the Agent started. |
| `rdma_flow_agent_send_events_sampled` | Gauge | None | The send events selected for storage by sampling since the Agent started. |
| `rdma_flow_agent_send_events_dropped` | Gauge | `sink` | The events dropped because the queue was full or the connection was broken. |
| `rdma_flow_agent_send_queue_length` | Gauge | `sink` | The events that wait to be written. |
| `rdma_flow_agent_send_rows_inserted_total` | Counter | `sink` | The events written. |
| `rdma_flow_agent_send_rows_failed_total` | Counter | `sink` | The events whose write failed. |
| `rdma_flow_agent_send_insert_seconds` | Histogram | `sink` | The time of each batch write. |
| `rdma_flow_agent_send_sink_connected` | Gauge | `sink` | 1 when the connection between the Agent and the Event Collector works. |

The value of the `sink` label is `otlp`.

### Flow labels

| Label | Meaning |
| --- | --- |
| `src_pod_namespace`, `src_pod_name` | The Pod that submits the WR. |
| `src_pod_top_owner_kind`, `src_pod_top_owner_namespace`, `src_pod_top_owner_name` | The top-level workload of the source Pod. |
| `dst_pod_namespace`, `dst_pod_name` | The peer Pod. |
| `dst_pod_top_owner_kind`, `dst_pod_top_owner_namespace`, `dst_pod_top_owner_name` | The top-level workload of the peer Pod. |
| `src_device` | The source RDMA device, for example `mlx5_0`. |
| `dst_device` | The destination RDMA device. Empty when it is not known. |

## RDMA send event fields

When `agent.ebpfFlow.flowEvents.enabled` is set, each send becomes one event in a ClickHouse table or an Elasticsearch data stream.

| Field | Type | Meaning |
| --- | --- | --- |
| `Timestamp` | Time, in nanoseconds | The send time. |
| `node` | String | The node name. |
| `tgid` | Integer | The ID of the sending process. |
| `qpn` | Integer | The QP number. |
| `bytes` | Integer | The bytes of one submission. |
| `wrs` | Integer | The WRs of one submission. |
| `count` | Integer | The number of submissions that this record merges. 1 when there is no merging. |
| `kind` | String | The submit method: `post_send`, `wr_set_sge`, `wr_set_sge_list`, `wr_set_inline_data`, or `wr_set_inline_data_list`. `unknown_type_<N>` when it is not known. |
| `src_device`, `dst_device` | String | The source RDMA device and the destination RDMA device. |
| `src_pod_namespace`, `src_pod_name`, `src_pod_top_owner_kind`, `src_pod_top_owner_namespace`, `src_pod_top_owner_name` | String | The source Pod and its top-level workload. |
| `dst_pod_namespace`, `dst_pod_name`, `dst_pod_top_owner_kind`, `dst_pod_top_owner_namespace`, `dst_pod_top_owner_name` | String | The destination Pod and its top-level workload. |
