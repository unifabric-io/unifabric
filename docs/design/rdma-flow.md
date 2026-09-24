# eBPF RDMA Flow Attribution Design

中文版：[rdma-flow.zh.md](./rdma-flow.zh.md)

## 1. Purpose

Host RDMA counters tell how much a device or a Pod sends, not where the
traffic goes. The `ebpfFlow` component of the Agent attributes RDMA send
traffic to Pod-to-Pod flows without touching the application, so operators
can see which training or inference job talks to which peer, how large its
messages are and, optionally, inspect every submission.

It is disabled by default and enabled per release through
`agent.ebpfFlow.enabled`. The code lives in `pkg/agent/ebpfflow` and runs as
a controller-runtime `Runnable` inside the existing Agent DaemonSet.

## 2. Data Plane

The component only observes `libibverbs`. Libraries that talk to the kernel
or the device directly are invisible to it.

- **Control plane.** A uprobe on `ibv_modify_qp` in every `libibverbs`
  instance found on the node fires when a queue pair enters RTR with an
  address vector. The BPF program stores the destination GID or LID, the
  destination QPN, the port, the device name and the process command in the
  `qp_infos` map keyed by `(tgid, qpn)`, and emits a `QP_RTR` event.
- **Callback discovery.** Public `libibverbs` probes such as
  `ibv_create_qp`, `ibv_reg_mr` and `ibv_query_qp` read the provider
  function pointers of live verbs objects and report the `post_send`,
  `wr_start`, `wr_complete` and related addresses through the
  `callback_events` ring buffer. User space translates them to file offsets
  inside the provider library and attaches address-only uprobes.
- **Data plane.** The provider uprobes count payload bytes and work requests
  per `(tgid, qpn)` in the `qp_stats` map for both the legacy `post_send`
  path and the extended verbs setters. When `events.enabled` is true they also
  push one 32 byte record per submission to the `send_events` ring buffer;
  when false the BPF configuration map disables those records at the source.
- **Persistence.** `qp_infos` and `qp_stats` are pinned under
  `/sys/fs/bpf/unifabric` so a restarted Agent keeps destinations and totals.
  A layout change recreates the pins. Learned callback offsets are cached per
  library content identity under `/var/lib/unifabric/ebpf-flow` so restarts
  re-attach without waiting for new verbs objects.

Libraries and processes are discovered through a `sched_process_exec`
tracepoint and a periodic scan of `/proc/*/maps`, which needs `hostPID`.
Versioned `libibverbs` symbols are resolved through the ELF dynamic symbol
table because `cilium/ebpf` picks the compatibility alias otherwise.

## 3. Attribution

Every report interval, default 5 s, the component joins `qp_stats` with
`qp_infos` and resolves both ends of every queue pair.

- **Source.** The `tgid` maps to a Pod UID through its cgroup path, the UID
  to a Pod through the Pod index.
- **Destination, in order.**
  1. A RoCE GID that is an IPv4 mapped or IPv6 address is looked up in the
     Pod IP index, which includes CNI annotations such as Multus
     `network-status` and Calico `podIPs`.
  2. A link-local GID carries an EUI-64 encoded MAC that is matched against
     MAC addresses found in the same annotations.
  3. The GID is looked up in the `RDMAEndpoint` index. When several Pods on
     the node share the GID, the destination QPN picks the one that reported
     it, otherwise a single holder wins and an ambiguous GID stays
     unresolved.
  4. InfiniBand queue pairs without a GRH carry a LID. The local port's
     subnet prefix, read from GID index 0, forms the key
     `<subnetPrefix>/<lid>` looked up in the `RDMAEndpoint` index, again
     refined by QPN.
- **Owner.** Each Pod's top-level owner is walked through
  `ownerReferences` with a table of known workload kinds, cached by UID.
  Unknown kinds stop the walk at that level.

hostNetwork Pods share the node IP and are excluded from the IP index, they
resolve only through `RDMAEndpoint`. Flows whose ends are not both Pods are
skipped without advancing their baseline, so traffic seen before an index
caught up is credited once after resolution and no series with empty labels
is created.

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

- Device holders are found through `/dev/infiniband/uverbsN` descriptors in
  `/proc/*/fd`. Devices, ports, link layer, LID, subnet prefix and GIDs are
  read from sysfs inside the host network namespace, or inside the Pod's own
  namespace for Pods with exclusively assigned VFs.
- Publishing is event driven. A `QP_RTR` event or a cleanup of exited
  processes triggers a run, runs are spaced at least `endpoint.minInterval`
  apart, default 3 s, and `endpoint.interval`, default 30 s, is the fallback
  covering lost events.
- Writes use Server-Side Apply with field manager `unifabric-agent`, once for
  metadata and once for the `status` subresource. Unchanged objects are
  rewritten at most every 5 minutes to refresh `lastUpdateTime`, readers
  treat objects older than 15 minutes as stale.
- On start the Agent adopts objects labelled `unifabric.io/node=<node>` left
  by its previous instance and deletes those whose Pod is gone. An object is
  deleted when the Pod no longer holds a device and has no queue pairs, which
  is logged as `rdma_endpoint_deleted reason=no_rdma_holder`.
- By default only hostNetwork Pods and Pods not yet in the index are
  published because other Pods resolve through their IPs.
  `endpoint.allPods` publishes every holder, which InfiniBand fabrics need
  because LIDs cannot be looked up otherwise.

## 5. Metrics

Flow metrics carry ten labels: `src_pod_namespace`, `src_pod_name`,
`src_pod_top_owner_kind`, `src_pod_top_owner_namespace`,
`src_pod_top_owner_name`, the five `dst_pod_*` counterparts, plus
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

Counters are monotonic within one Agent process. Counter regressions in the
maps, which mean a recreated entry, are treated as a new entry. The send
counters and histogram accept unresolved ends with empty labels because the
size distribution is only useful when it covers every submission.

## 6. Send Event Pipeline

`agent.ebpfFlow.flowEvents.enabled` controls per-send event collection and defaults
to `true` for compatibility:

- **Cumulative mode (`false`).** Probes still update `qp_infos` and `qp_stats`,
  so `rdma_flow_bytes_total`, `rdma_flow_wrs_total`, `rdma_flow_qps` and the
  `rdma_flow_agent_qp_*` health gauges are available. The BPF program does not
  write `send_events`; the Agent does not open that ring buffer, create sink
  queues, connect to OTLP or emit send-size metrics.
- **Event mode (`true`).** In addition to cumulative metrics, every submission
  is emitted to the ring buffer. The Agent publishes `rdma_flow_sends_total`
  and `rdma_flow_send_bytes`, and can export attributed rows through OTLP.

The Helm `ebpfFlowEventCollector` requires events. Enabling the Collector while
`flowEvents.enabled` is false is rejected during template rendering.

Send events are attributed with the snapshot published by the last report
and fanned out to sinks through the `sendSink` interface. Every sink has its
own bounded queue, writer goroutine, lazy connection and counters.

- The queue holds `flowEvents.queue` rows, default 200000, batches of
  `flowEvents.batch` rows, default 10000, or a partial batch after `flowEvents.flush`,
  default 1 s. A full queue and a failed export both drop rows without
  retries so the ring buffer consumer never blocks.
- `flowEvents.sample` keeps one event in N for storage while metrics see all.
- `flowEvents.aggregate` merges identical `(kind, bytes, wrs)` submissions of a
  queue pair inside the window into one row with a `count`. LLM traffic
  repeats a few message sizes, a 100 ms window typically cuts rows by one to
  two orders of magnitude.
- Connections are established lazily and retried every 30 s, rows arriving
  while disconnected are dropped and counted.

The only sink is `otlp`. Each row becomes one OpenTelemetry log record sent
over OTLP gRPC, `Timestamp` is the BPF event time converted from
`CLOCK_MONOTONIC`, `EventName` and `Body` are `rdma.send`, the resource
carries `service.name=unifabric-agent` and `k8s.node.name`, and the
attributes are flat and typed: `tgid`, `qpn`, `bytes`, `wrs`, `count` as
int64 and `node`, `kind`, `src_device`, `dst_device` and the ten Pod labels
as strings. The Collector owns the backends, see the
[usage guide](../usage/rdma-flow.md).

## 7. Limitations

- Traffic from libraries bypassing `libibverbs` is not observed.
- UD queue pairs are not attributed.
- Send events are reported one by one, a 16 MiB ring buffer can overflow
  under extreme submission rates, losing detail but not the `qp_stats`
  totals.
- Flow series are never removed while the Agent runs.
- The cluster-wide Pod cache costs memory proportional to the cluster's Pod
  count on every node.
