# unifabric

Unifabric is an LLDP and RDMA topology discovery project built around two
components:

- `controller`: manages cluster-scoped topology aggregation such as
  `ScaleOutLeafGroup`
- `agent`: runs on nodes to collect LLDP, RDMA, and Pod-level topology data

The canonical Go module path is `github.com/unifabric-io/unifabric`.

This repository currently includes:

- Host LLDP discovery
- Host RDMA metrics
- Pod RDMA metrics when SR-IOV RDMA is available
- Storage network topology discovery
- GPU network topology discovery
- Scale-out leaf grouping based on LLDP neighbors

Default labels:

- `unifabric.io/accelerator`
- `unifabric.io/leaf`
- `unifabric.io/spine`
- `unifabric.io/core`

The Helm chart can optionally enable the vendored `topograph` chart as a
subchart in Kubernetes InfiniBand mode. The IB defaults are baked into the
vendored `topograph` values, and topology labels are taken from
`global.topologyLabels`, so enabling it only needs
`--set topograph.enabled=true`.
