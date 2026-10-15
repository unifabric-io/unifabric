# Roadmap

中文版：[roadmap.zh.md](./roadmap.zh.md)

- Last updated: 2026-10-15
- Baseline version: v0.3.0 (the latest release when this roadmap was written)

This roadmap describes the planned direction of Unifabric. It is a living
document: items, priorities, and target releases may change based on community
feedback. Discuss or propose roadmap items through
[GitHub issues](https://github.com/unifabric-io/unifabric/issues).

## Planned items

| Area | Item | Status | Target release |
| --- | --- | --- | --- |
| Topology discovery | RDMA NIC discovery and classification (scale-out / storage) | Completed | v0.0.1 |
| | LLDP neighbor discovery (node and switch agents) | Completed | v0.0.2 |
| | Switch-driven scale-out topology computation | Completed | v0.0.2 |
| | Topology status aggregation and automatic node labels | Completed | v0.1.0 |
| | Scheduler integration via topology labels (Kueue / Volcano / KAI TAS) | Completed | v0.1.0 |
| | NVIDIA Topograph / NetQ import (InfiniBand, Spectrum-X) | Completed | v0.3.0 |
| | Topology visualization (Topology API + Grafana) | Completed | v0.3.0 |
| | Native scale-up (NVLink) topology discovery | Planned | v0.5 |
| | Native storage network topology discovery | Planned | v0.5 |
| RDMA observability | RDMA device, port, congestion, and error metrics | Completed | v0.0.1 |
| | Pod, namespace, and workload attribution | Completed | v0.0.1 |
| | Prometheus integration and Grafana dashboards | Completed | v0.1.0 |
| | eBPF Pod-to-Pod flow attribution and RDMAEndpoint | Completed | v0.3.0 |
| | Per-send flow events with ClickHouse / Elasticsearch storage | Completed | v0.3.0 |
| | GPU-initiated RDMA (GDAKI / IBGDA) observability | Planned | Backlog |
| RDMA network health checks | Passive health status (LLDP readiness, switch connectivity, probes) | Completed | v0.1.0 |
| | Active network path probing and link degradation detection | Planned | v0.4 |
| | Automatic fault remediation | Planned | Backlog |

Status values: `Planned`, `In progress`, `Completed`, `Deferred`.
