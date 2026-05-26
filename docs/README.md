# Unifabric Documentation

中文版：[README.zh.md](./README.zh.md)

This directory is organized around the usual reader path: deploy Unifabric first,
then read usage guides, design documents, and development notes.

## Get Started

Choose the installation guide that matches the cluster's physical network:

- [General SONiC RoCE](./getting-started-sonic-roce.md): For RoCE networks carried by SONiC switches.
- [Spectrum-X fabric](./getting-started-spectrum-x.md): For Spectrum-X switch fabrics.
- [InfiniBand fabric](./getting-started-infiniband.md): For NVIDIA InfiniBand networks.

## Usage Guides

- [RDMA observability usage guide](./usage/rdma-metrics.md): Enable and verify RDMA metrics, Prometheus scraping, and Grafana dashboards.
- [Kueue TAS workload example](./usage/workload-tas.md): Use Unifabric scale-out leaf Node labels with Kueue Topology Aware Scheduling.

## Design Docs

- [FabricNode CRD design](./design/fabricnode.md): Node-local RDMA topology state resource design.
- [Scale-out topology discovery design](./design/scaleout-topology.md): Switch and SwitchGroup based scale-out topology discovery and Node label reconciliation design.
- [ScaleOutLeafGroup CRD design](./design/scaleoutleafgroup.md): Scale-out leaf grouping and Node label reconciliation design.
- [RDMA observability design](./design/rdma-metrics.md): RDMA metric model, Pod attribution, and collection design.

## Development

- [NVAIR development guide](./development/dev-with-nvair.md): Build an e2e topology with NVAIR, install monitoring components, and deploy Unifabric for development.

## Reference

- [Helm values reference](../chart/README.md): Chart parameters and default values.
