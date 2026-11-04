# eBPF RDMA Flow Attribution Design

Chinese: [rdma-flow.zh.md](./rdma-flow.zh.md)

## 1. Purpose

Host RDMA counters show how much a device or Pod has sent, but not which peer
received the traffic. The Agent's `ebpfFlow` component attributes RDMA send
traffic to Pod-to-Pod flows without modifying applications, helping operators
identify the communication peers and message sizes of training or inference
workloads and optionally inspect each send submission.

The component observes only **CPU-initiated RDMA**: the CPU must submit
`post_send` or extended verbs work requests (WRs) through a `libibverbs`
provider. A buffer in GPU memory does not affect observability as long as
the CPU submits the WRs.

**GPU-initiated RDMA** uses GPU kernels to submit WQEs directly to NIC queues,
as in GDAKI, GPUDirect Async, and IBGDA. These paths bypass the component's
uprobes and are not observable.

The component is disabled by default and enabled per Helm release through
`agent.ebpfFlow.enabled`. The code lives in `pkg/agent/ebpfflow` and runs as
a controller-runtime `Runnable` inside the existing Agent DaemonSet.

### 1.1 Communication Library Compatibility

Compatibility depends on the final network submission path, not just the
communication library name or version. Support details and the versions they
apply to are maintained in the "Inference frameworks" and "Communication
libraries" sections of the [usage guide](../usage/traffic-observability/rdma-flow.md#scope).
After upgrading a communication library, check the actual backend configuration
and logs to confirm the submission path.

## 2. Data Plane

The component observes only the `libibverbs` path. Libraries that bypass
`libibverbs` to interact directly with the kernel or device are not observable.

- **Control plane.** A uprobe on `ibv_modify_qp` in every `libibverbs`
  instance found on the node fires when a queue pair enters RTR with an
  address vector. The BPF program stores the destination GID or LID, the
  destination QPN, the port, the device name and the process name in the
  `qp_infos` map keyed by `(tgid, qpn)`, and emits a `QP_RTR` event.
- **Callback discovery.** Probes on public functions such as
  `ibv_create_qp`, `ibv_reg_mr` and `ibv_query_qp` read the provider
  function pointers of live verbs objects and report the `post_send`,
  `wr_start`, `wr_complete` and related addresses through the
  `callback_events` ring buffer. User space translates them to file offsets
  inside the provider library and attaches address-only uprobes.
- **Data plane.** The provider uprobes count payload bytes and work requests
  per `(tgid, qpn)` in the `qp_stats` map for both the legacy `post_send`
  path and the extended verbs setters. When `flowEvents.enabled` is `true`,
  they also write one 32-byte record per submission to the `send_events`
  ring buffer; when `false`, the BPF configuration map disables generation
  of those records.
- **Persistence.** `qp_infos` and `qp_stats` are pinned under
  `/sys/fs/bpf/unifabric` so a restarted Agent keeps destinations and totals.
  A map layout change causes the maps to be recreated. Discovered callback offsets are cached per
  library content identity under `/var/lib/unifabric/ebpf-flow` so restarts
  re-attach without waiting for new verbs objects.

Libraries and processes are discovered through a `sched_process_exec`
tracepoint and a periodic scan of `/proc/*/maps`, which needs `hostPID`.
Versioned `libibverbs` symbols are resolved through the ELF dynamic symbol
table because `cilium/ebpf` picks the compatibility alias otherwise.

## 3. Attribution

Every report interval (5 seconds by default), the component joins `qp_stats`
with `qp_infos` and resolves the source and destination of each QP.

- **Source.** Map `tgid` to a Pod UID through its cgroup path, then look up
  the Pod in the Pod index.
- **Destination.** Resolve in the following order:
  1. Look up a RoCE GID that is an IPv4-mapped or IPv6 address in the
     Pod IP index, which includes addresses from CNI annotations such as Multus
     `network-status` and Calico `podIPs`.
  2. A link-local GID carries an EUI-64 encoded MAC that is matched against
     MAC addresses found in the same annotations.
  3. The GID is looked up in the `RDMAEndpoint` index. When several Pods on
     the node share the GID, the destination QPN picks the one that reported
     that QPN. Otherwise, use the GID's unique holder. GIDs that remain
     ambiguous are left unresolved.
  4. InfiniBand queue pairs without a GRH carry a LID. The local port's
     subnet prefix, read from GID index 0, forms the key
     `<subnetPrefix>/<lid>` looked up in the `RDMAEndpoint` index, again
     refined by QPN.
- **Owner.** Follow `ownerReferences` using the known workload type table
  to find the top-level owner, and cache the result by UID. Stop traversing
  when an unknown type is encountered.

hostNetwork Pods share the node IP and are excluded from the IP index; they
can only be resolved through `RDMAEndpoint`. If either end of a flow has not
been resolved to a Pod, the component skips that reporting cycle without
advancing the baseline. Once the index is updated and resolution succeeds,
previously observed traffic is accounted for in one update, so no flow time
series with empty labels is created.

The Pod index is a dedicated cluster-wide controller-runtime cache restricted
to Pods and stripped to identity, owner references, annotations, hostNetwork
and Pod IPs, because the Agent's default cache only holds Pods on the local
node. Moving peer resolution entirely to `RDMAEndpoint` is a planned
follow-up.

## 4. RDMAEndpoint

Each Agent publishes one namespaced `RDMAEndpoint` per Pod on its node that
holds an RDMA device open or still has queue pairs in `qp_infos`. The object
is named after the Pod, owned by it with `controller: true`, and carries only
observed status, see the [API reference](../reference/rdmaendpoint.md).

- **Device discovery.** Device holders are found through `/dev/infiniband/uverbsN` descriptors in
  `/proc/*/fd`. Devices, ports, link layer, LID, subnet prefix and GIDs are
  read from sysfs inside the host network namespace, or inside the Pod's own
  network namespace for Pods with exclusively assigned VFs.
- **Publishing schedule.** A `QP_RTR` event or cleanup of exited processes
  triggers publication. Runs are spaced at least `endpoint.minInterval`
  apart (3 seconds by default). `endpoint.interval` (30 seconds by default)
  provides periodic publication to compensate for lost events.
- **Writes and refreshes.** Writes use Server-Side Apply with field manager
  `unifabric-agent`, updating metadata and the `status` subresource separately.
  Unchanged objects are rewritten at most once every 5 minutes to refresh
  `lastUpdateTime`. Readers treat objects not refreshed for more than
  15 minutes as stale.
- **Adoption and cleanup.** On start, the Agent adopts objects labelled `unifabric.io/node=<node>` left
  by its previous instance and deletes those whose Pod is gone. An object is
  deleted when the Pod no longer holds a device and has no queue pairs, which
  is logged as `rdma_endpoint_deleted reason=no_rdma_holder`.
- **Publication scope.** By default, only hostNetwork Pods and Pods not yet
  in the index are published; other Pods are resolved by IP.
  Enabling `endpoint.allPods` publishes every holder. InfiniBand requires
  this setting because a LID cannot otherwise be mapped back to a Pod.

## 5. Metrics

Flow metrics contain 12 labels: 5 source Pod labels (`src_pod_namespace`,
`src_pod_name`, `src_pod_top_owner_kind`, `src_pod_top_owner_namespace`,
`src_pod_top_owner_name`), the corresponding 5 `dst_pod_*` labels, and
`src_device` and `dst_device`.

| Metric | Type | Meaning |
| --- | --- | --- |
| `rdma_flow_bytes_total` | counter | Payload bytes posted from source to destination. |
| `rdma_flow_wrs_total` | counter | Work requests posted. |
| `rdma_flow_qps` | gauge | Queue pairs attributed to the flow in the last report. |
| `rdma_flow_sends_total` | counter | Send submissions, one per `post_send` or extended verbs setter. |
| `rdma_flow_send_bytes` | histogram | Payload bytes per submission, buckets from 64 B to 1 GiB in powers of two. |
| `rdma_flow_agent_qp_infos`, `_qp_stats`, `_qp_matched`, `_qp_unmatched`, `_qp_unresolved`, `_flows` | gauge | Map sizes and join results of the last report. |
| `rdma_flow_agent_send_events_received`, `_sampled` | gauge | Send events read from the ring buffer and selected for storage. |
| `rdma_flow_agent_send_events_dropped{sink}`, `_send_queue_length{sink}`, `_send_rows_inserted_total{sink}`, `_send_rows_failed_total{sink}`, `_send_insert_seconds{sink}`, `_send_sink_connected{sink}` | per sink | Storage path health. |

Counters are monotonically increasing only within a single Agent process.
A counter regression in a map indicates that the entry has been recreated,
so the component treats it as a new entry.

Unlike flow attribution metrics, send counters and the histogram accept
events with unresolved sources or destinations and retain the corresponding
empty labels to cover the message-size distribution of all submissions.

## 6. Send Event Pipeline

`agent.ebpfFlow.flowEvents.enabled` controls per-send event collection and
defaults to `false`. When only cumulative flow metrics are needed, disabling
event collection avoids ring buffer and export pipeline overhead.

- **Cumulative mode (`false`).** Probes still update `qp_infos` and `qp_stats`,
  so `rdma_flow_bytes_total`, `rdma_flow_wrs_total`, `rdma_flow_qps` and the
  `rdma_flow_agent_qp_*` health gauges are available. The BPF program does not
  write `send_events`; the Agent does not open that ring buffer, create sink
  queues, connect to OTLP or emit send-size metrics.
- **Event mode (`true`).** In addition to cumulative metrics, every submission
  is emitted to the ring buffer. The Agent publishes `rdma_flow_sends_total`
  and `rdma_flow_send_bytes`, and can export attributed event records through OTLP.

The Helm `ebpfFlowEventCollector` requires per-send events. Enabling the
Event Collector while `flowEvents.enabled` is `false` causes an error during
template rendering.

Send events are attributed with the snapshot published by the last report
and fanned out to sinks through the `sendSink` interface. Every sink has its
own bounded queue, writer goroutine, lazy connection and counters.

- **Queueing and batching.** `flowEvents.queue` controls queue capacity,
  defaulting to 200000 rows. A batch is sent after `flowEvents.batch` rows
  accumulate (10000 by default); partial batches are sent after the
  `flowEvents.flush` interval (1 second by default). A full queue or a failed
  send causes records to be dropped without retries, ensuring that the
  ring buffer consumer never blocks.
- **Sampling.** `flowEvents.sample` stores only one event in every N;
  metrics still count all events.
- **Aggregation.** `flowEvents.aggregate` merges submissions with identical
  `(kind, bytes, wrs)` values on the same QP within the window into one record
  with a `count`. Message sizes in LLM traffic are highly repetitive; a
  100 ms window typically reduces record counts by one to two orders of magnitude.
- **Connections and retries.** Connections are established on demand and
  retried every 30 seconds. Records arriving while disconnected are dropped
  and counted.

The only sink currently available is `otlp`. Each event record becomes an
OpenTelemetry log record sent over OTLP gRPC:

- `Timestamp` is the BPF event time converted from `CLOCK_MONOTONIC`.
- `EventName` and `Body` are both `rdma.send`.
- The resource contains `service.name=unifabric-agent` and `k8s.node.name`.
- Attributes are flat and typed: `tgid`, `qpn`, `bytes`, `wrs`, and `count`
  are int64; `node`, `kind`, `src_device`, `dst_device`, and the 10 Pod labels
  are strings.

The Event Collector manages the storage backends. See the
[usage guide](../usage/traffic-observability/rdma-flow-events.md) for configuration details.

## 7. Limitations

- Traffic from libraries bypassing `libibverbs` is not observed.
- UD queue pairs are not attributed.
- Send events are reported one by one, a 16 MiB ring buffer can overflow
  under extreme submission rates, losing detail but not the `qp_stats`
  totals.
- Flow series are never removed while the Agent runs.
- The cluster-wide Pod cache costs memory proportional to the cluster's Pod
  count on every node.
