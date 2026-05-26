# Unifabric

## Overview

Unifabric automatically discovers the RDMA network topology within Kubernetes clusters, publishes the results as Kubernetes CRD resources, and applies corresponding topology labels to nodes. Designed specifically for AI data center clusters, it helps the scheduler understand the network layout—such as whether nodes are located in the same acceleration zone or connected to the same leaf or spine switch—thereby enabling more efficient and rational scheduling decisions. In addition, Unifabric provides node-level RDMA metrics (including Pod-attributed dimensions) that can be monitored and analyzed via dashboards.

Key features:

- Improve GPU utilization
- Reduce cross-node latency
- RoCE and InfiniBand hybrid topology discovery (InfiniBand case integrates with [NVIDIA/topograph](https://github.com/NVIDIA/topograph))
- Kubernetes CRD topology resources
- Rich RDMA metrics for Pods, Nodes, and switches

## Core Concepts

- FabricNode: Records per-node RDMA information, LLDP neighbors, node IP, node type, and RDMA Pod status.
- Switch: Records switch dial targets and observed LLDP neighbor snapshots used by switch-driven scale-out topology discovery.
- SwitchTopologyDiscovery: Computes scale-out leaf, spine, and core domains from FabricNode and Switch data, then writes Node topology labels.
- Node RDMA observability: Exposes Node RDMA metrics for monitoring device, port, and workload traffic.

## Next Steps

- Read the getting started deployment guide in [docs/getting-started.md](./docs/getting-started.md).
- Read the Helm values reference in [chart/README.md](./chart/README.md).
- Read the RDMA observability usage guide in [docs/usage/rdma-metrics.md](./docs/usage/rdma-metrics.md).
- Read the Kueue TAS workload example in [docs/usage/workload-tas.md](./docs/usage/workload-tas.md).

## License

This project is licensed under the Apache License 2.0. See [LICENSE](./LICENSE).
