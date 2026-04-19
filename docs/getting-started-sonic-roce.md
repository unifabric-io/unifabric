# General SONiC RoCE

Chinese version: [getting-started-sonic-roce.zh.md](./getting-started-sonic-roce.zh.md)

This guide explains how to deploy Unifabric in a cluster where SONiC switches carry the RoCE network. This scenario discovers scale-out leaf topology from node RDMA NICs and LLDP neighbor information.

## Deployment Goals

After deployment, the cluster should achieve two goals:

- Nodes are labeled with the leaf topology label consumed by schedulers. The default label is `unifabric.io/scale-out-leaf=<group-name>`.
- Node RDMA state is observable through Unifabric Agent metrics, including RDMA device, port, priority, and Pod attribution metrics.
- The corresponding topology can be queried through `FabricNode` CRs and `ScaleOutLeafGroup` CRs.

> Unifabric currently recognizes only leaf labels. Spine and core topology recognition is in progress.

## Prerequisites

- A Kubernetes cluster with Linux worker nodes.
- `kubectl` and Helm 3 installed.
- RDMA-capable network interfaces visible under `/sys/class/infiniband` on the nodes.
- LLDP is available on switches and nodes. The Agent reads LLDP neighbor information.
- The Agent requires privileged permissions to access host networking, RDMA devices, `/proc`, and container runtime state.
- Prometheus Operator and Grafana Operator are installed in the cluster. If they are not installed, disable
  `ServiceMonitor` and `GrafanaDashboard` when installing Unifabric to avoid CRD creation failures.

Verify cluster access:

```bash
kubectl cluster-info
kubectl get nodes -o wide
```

## Install Unifabric

The following command uses the latest release. The example excludes `eth9` as a storage RDMA NIC from scale-out leaf grouping. Other RDMA NICs participate in LLDP topology discovery and RDMA metrics by default.

```bash
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unifabric-io/unifabric/releases/latest | grep '"tag_name":' | cut -d '"' -f4)
CHART_VERSION="${LATEST_TAG#v}"

helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --version "${CHART_VERSION}" \
  --namespace unifabric-system \
  --create-namespace \
  --set nvidiaTopograph.enable=false \
  --set scaleOutDiscovery.leafGroups.enabled=true \
  --set-string nodeTopologyDiscovery.storageInterfaceSelector="interface=eth9" \
  --set nodeMetrics.enabled=true \
  --set nodeMetrics.serviceMonitor.enabled=true \
  --set grafanaDashboard.enabled=true \
  --wait
```

Parameters:

| Helm value | Purpose |
| --- | --- |
| `nvidiaTopograph.enable` | Keep `false` for SONiC RoCE / LLDP. Topology is discovered by Unifabric Agent / Controller. |
| `scaleOutDiscovery.leafGroups.enabled` | Enables Unifabric `ScaleOutLeafGroup` discovery and leaf Node label write-back. |
| `nodeTopologyDiscovery.storageInterfaceSelector` | Optional: selects storage RDMA NICs and excludes them from scale-out leaf grouping. Metrics are labeled `kind=storage`. Supports `interface=eth9` or `cidr=172.20.0.0/16`. |
| `nodeTopologyDiscovery.scaleUpInterfaceSelector` | Optional: selects scale-up RDMA NICs and excludes them from scale-out leaf grouping. Metrics are labeled `kind=scaleUp`. |
| `nodeTopologyDiscovery.scaleOutInterfaceSelector` | Optional: restricts RDMA NICs that participate in scale-out leaf grouping. When empty, all RDMA NICs not matched by storage / scale-up selectors participate. |
| `nodeMetrics.enabled` | Enables Agent metrics for node RDMA observability. |
| `nodeMetrics.serviceMonitor.enabled` | Creates the Prometheus Operator `ServiceMonitor`. |
| `grafanaDashboard.enabled` | Renders the built-in RDMA dashboards. |
| `topologyLabels.scaleOutLeaf` | Leaf Node label key. Default: `unifabric.io/scale-out-leaf`. |

For more Helm parameters, see [chart/README.md](../chart/README.md).

## Verify the Deployment

Wait for the controller and agent:

```bash
kubectl -n unifabric-system get pods
kubectl -n unifabric-system rollout status deployment/unifabric-controller
kubectl -n unifabric-system rollout status daemonset/unifabric-agent
```

Inspect `FabricNode`:

```bash
kubectl get fabricnodes
kubectl get fabricnode <node-name> -o yaml
```

Check:

- `status.scaleOutNics` contains the expected scale-out RDMA NICs.
- `status.storageNics` contains only storage network interfaces.
- `status.scaleOutNics[*].lldpNeighbor.hostname` is populated.
- `Ready` and `LLDPNeighborsReady` in `status.conditions` are `True`.

Inspect leaf groups and Node labels:

```bash
kubectl get scaleoutleafgroups -o wide
kubectl get scaleoutleafgroup <group-name> -o yaml
kubectl get nodes -L unifabric.io/scale-out-leaf,kubernetes.io/hostname
```

If nodes are grouped correctly, `ScaleOutLeafGroup.status.nodes` contains those nodes, and the nodes have `unifabric.io/scale-out-leaf=<group-name>`.

When configuring Kueue, Volcano, or KAI Scheduler, use only labels that are actually written to Nodes. The current SONiC RoCE / LLDP scenario writes only the leaf label automatically; spine/core levels are not written yet.

Verify RDMA metrics resources:

```bash
kubectl -n unifabric-system get service unifabric-agent-metrics
kubectl -n unifabric-system get servicemonitor unifabric-agent-metrics
```

Check the Agent metrics endpoint directly:

```bash
POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-agent -o jsonpath='{.items[0].status.podIP}')
curl -s "http://${POD_IP}:8082/metrics" | grep '^unifabric_'
```

Check RDMA metric NIC classification:

```bash
curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="scaleOut"'
```

## Troubleshooting

### `FabricNode` Has No Scale-Out NICs

- Confirm that RDMA devices are visible under `/sys/class/infiniband` on the node.
- Confirm that `nodeTopologyDiscovery.scaleOutInterfaceSelector` matches the real interface name or CIDR.
- Confirm that scale-out interfaces were not accidentally matched as storage or scale-up; those interfaces are excluded from scale-out grouping.

### `LLDPNeighborsReady=False`

- Confirm that LLDP is enabled on switches.
- Confirm that node-side `lldpd` works and the Agent Pod can read LLDP information.
- Confirm that `nodeTopologyDiscovery.initialScanDelay` is long enough, so the first Agent scan does not run before LLDP learning completes.

### No `ScaleOutLeafGroup` or Node Label

- Confirm that `scaleOutDiscovery.leafGroups.enabled=true`.
- Confirm that `FabricNode.status.nodeRole` is not `Storage`.
- Confirm that at least one `scaleOutNics` entry has `state=up` and `lldpNeighbor.hostname`.
- Check controller logs:

  ```bash
  kubectl -n unifabric-system logs deployment/unifabric-controller
  ```

### RDMA Metrics Do Not Include RoCE NICs

- Confirm that the Agent Pod is running and RDMA devices are visible under `/sys/class/infiniband` on the node.
- Confirm that `nodeMetrics.enabled=true`.
- If using Prometheus Operator, confirm that `nodeMetrics.serviceMonitor.enabled=true` and the `ServiceMonitor` is selected by Prometheus.
- Query the Agent metrics endpoint directly first. If `unifabric_` metrics exist there, troubleshoot Prometheus target discovery next.

## Uninstall

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

If CRDs are no longer needed, delete them manually:

```bash
kubectl delete crd fabricnodes.unifabric.io scaleoutleafgroups.unifabric.io
```

## Next Steps

- Return to the [documentation index](./README.md).
- Read the [Kueue TAS workload example](./usage/workload-tas.md).
- See the [Helm values reference](../chart/README.md).
