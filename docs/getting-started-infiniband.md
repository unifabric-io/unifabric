# InfiniBand Fabric

中文版: [getting-started-infiniband.zh.md](./getting-started-infiniband.zh.md)

This guide explains how to deploy Unifabric in an InfiniBand NIC cluster. This scenario is for IB networking, such as Mellanox NICs in IB mode with IB switches.

## Deployment Goals

After deployment, the cluster should achieve two goals:

- Nodes receive the `scale-up.unifabric.io/tier-N` and
  `scale-out.unifabric.io/tier-N` topology labels used by schedulers. `N`
  starts at 1; a smaller number represents a shorter topology distance and
  typically higher communication performance.
- Node RDMA state is observable through Unifabric Agent metrics and the built-in RDMA Grafana dashboards, with throughput, utilization, QoS, congestion, and error metrics grouped by cluster, node, Pod, and workload.

## Prerequisites

- A Kubernetes cluster with target GPU nodes.
- `kubectl` and Helm 3 installed.
- InfiniBand / RDMA devices visible under `/sys/class/infiniband` on the nodes.
- Prometheus Operator and Grafana Operator are installed in the cluster. If they are not installed, disable
  `ServiceMonitor` and `GrafanaDashboard` when installing Unifabric to avoid CRD creation failures.

Verify cluster access:

```bash
kubectl cluster-info
kubectl get nodes -o wide
```

## Install Unifabric

The following command uses the latest release. The example leaves RDMA interface selectors empty, so all RDMA NICs are observed by metrics. InfiniBand topology labels are written by NVIDIA topograph.

```bash
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unifabric-io/unifabric/releases/latest | grep '"tag_name":' | cut -d '"' -f4)
CHART_VERSION="${LATEST_TAG#v}"

helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --version "${CHART_VERSION}" \
  --namespace unifabric-system \
  --create-namespace \
  --set topoDiscovery.scaleUp.mode=nv-topograph \
  --set topoDiscovery.scaleOut.mode=nv-topograph \
  --set-string fabricNode.scaleOutInterfaceSelector="" \
  --set-string fabricNode.storageInterfaceSelector="" \
  --set nodeMetrics.enabled=true \
  --set nodeMetrics.serviceMonitor.enabled=true \
  --set grafanaDashboard.enabled=true \
  --wait --debug
```

Parameters:

| Helm value | Purpose |
| --- | --- |
| `topoDiscovery.scaleUp.mode` | Set to `nv-topograph` to select NVIDIA Topograph as the scale-up label writer. This setting is independent of scale-out mode. |
| `topoDiscovery.scaleOut.mode` | Set to `nv-topograph` to select NVIDIA Topograph as the scale-out label writer. |
| `topoDiscovery.storage.mode` | Set to `unifabric-ib` for built-in InfiniBand storage discovery. |
| `nvidiaTopograph.provider.name` | Defaults to `infiniband-k8s` for this scenario. |
| `nvidiaTopograph.useGpuCliqueLabel` | Defaults to `true` and uses the GPU Operator clique label as the accelerator topology source. Set it to `false` only when node-data-broker should discover topology through `pods/exec`. |
| `nodeMetrics.enabled` | Enables Agent metrics for node RDMA observability. |
| `fabricNode.scaleOutInterfaceSelector` | Selects RDMA NICs included in scale-out topology and RDMA metrics, and labels them with `kind=scaleOut` in RDMA metrics. Supports `interface=ib*,mlx*` or `cidr=172.17.0.0/16`. Defaults to all RDMA NICs. |
| `fabricNode.storageInterfaceSelector` | Selects storage RDMA NICs and labels them with `kind=storage` in RDMA metrics. Supports `interface=ib*,mlx*` or `cidr=172.17.0.0/16`. Defaults to empty. |
| `nodeMetrics.serviceMonitor.enabled` | Creates the Prometheus Operator `ServiceMonitor`. |
| `grafanaDashboard.enabled` | Renders the built-in RDMA dashboards. |

For more Helm parameters, see [chart/README.md](../chart/README.md).

If you are in mainland China, add the following parameters to speed up image pulls:

```bash
--set global.registry=m.daocloud.io \
--set controller.image.repository=ghcr.io/unifabric-io/unifabric-controller \
--set agent.image.repository=ghcr.io/unifabric-io/unifabric-agent \
--set nvidiaTopograph.image.repository=ghcr.io/nvidia/topograph
```

## Verify the Deployment

Check topograph components, node-data-broker DaemonSet, Node annotation, and Node labels:

```bash
kubectl -n unifabric-system get pods
kubectl get pods -n unifabric-system -o wide
kubectl get fabricnodes.unifabric.io
kubectl get nodes -L scale-up.unifabric.io/tier-1,scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2,scale-out.unifabric.io/tier-3,kubernetes.io/hostname
```

The FabricNode list should include every target Node with `READY` set to `True`:

```text
NAME                TOTALNICS   READY   ROLE   NODEIP
gpu-10-127-107-12   9           True    GPU    10.127.107.12
gpu-10-127-107-14   9           True    GPU    10.127.107.14
gpu-10-127-107-15   9           True    GPU    10.127.107.15
gpu-10-127-107-16   9           True    GPU    10.127.107.16
gpu-10-127-107-17   9           True    GPU    10.127.107.17
gpu-10-127-107-18   9           True    GPU    10.127.107.18
gpu-10-127-107-19   9           True    GPU    10.127.107.19
gpu-10-127-107-20   9           True    GPU    10.127.107.20
gpu-10-127-107-21   9           True    GPU    10.127.107.21
```

Inspect the Agent report for an individual Node:

```bash
kubectl get fabricnode <node-name> -o yaml
```

Confirm that `status.nodeRole`, `status.scaleOutNics`, and `status.conditions`
match the expected state and that participating IB NICs are `up`. You can also
query `FabricNode` resources with the `fn` short name.

After ScaleOut topology discovery succeeds, the `scaleout` Topology is created:

```bash
kubectl get topo
```

```text
NAME       AGE
scaleout   113m
```

Inspect the complete result:

```bash
kubectl get topo scaleout -o yaml
```

The following example represents a three-tier leaf, spine, and core topology.
`status.domains` describes parent-child relationships between performance
domains, while `status.nodes[].domainPath` records each Node's complete path
from tier 3 to tier 1:

```yaml
apiVersion: unifabric.io/v1beta1
kind: Topology
metadata:
  name: scaleout
status:
  domains:
    - name: S-fc6a1c0300b03c40
      tier: 3
    - name: S-fc6a1c0300afca40
      parent: S-fc6a1c0300b03c40
      tier: 2
    - name: S-fc6a1c03006636c0
      parent: S-fc6a1c0300afca40
      tier: 1
  nodes:
    - domainPath:
        - S-fc6a1c0300b03c40
        - S-fc6a1c0300afca40
        - S-fc6a1c03006636c0
      nodes:
        - gpu-10-127-107-12
        - gpu-10-127-107-14
        - gpu-10-127-107-15
        - gpu-10-127-107-16
        - gpu-10-127-107-17
        - gpu-10-127-107-18
        - gpu-10-127-107-19
        - gpu-10-127-107-20
        - gpu-10-127-107-21
```

When configuring Kueue, Volcano, or KAI Scheduler, use only labels that are actually written to Nodes.

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

## Troubleshooting

### Differences Between NVIDIA Topograph ScaleUp Discovery Modes

- With `nvidiaTopograph.useGpuCliqueLabel=true`, Topograph reads the
  `nvidia.com/gpu.clique` label written by GPU Operator. node-data-broker does
  not need to enter the NVIDIA device-plugin Pod to collect the clique ID.
- With `nvidiaTopograph.useGpuCliqueLabel=false`, node-data-broker finds the
  `nvidia-device-plugin-daemonset` Pod running on the target Node in the
  `gpu-operator` namespace by default, then executes the following command
  through `pods/exec`:

  ```bash
  nvidia-smi -q | grep "ClusterUUID\|CliqueId" | sort -u
  ```

  The result is combined as `<ClusterUUID>.<CliqueId>`, written to the
  `topograph.nvidia.com/cluster-id` Node annotation, and used as the source for
  `scale-up.unifabric.io/tier-1`. The chart grants node-data-broker permission
  to read DaemonSets and Pods and create `pods/exec` only when
  `provider.name=infiniband-k8s` and this option is `false`.
- `ibnetdiscover` discovers the ScaleOut InfiniBand switch hierarchy. Topograph
  executes it in node-data-broker Pods independently of `useGpuCliqueLabel`.
- `ClusterUUID=00000000-0000-0000-0000-000000000000` means that no valid
  multi-node NVLink domain is available. Topograph v0.5.0 does not reject this
  all-zero value and can produce an invalid `topograph.nvidia.com/cluster-id`
  annotation or scale-up label. Do not use such a value for scheduling.

Inspect the input and output on a Node:

```bash
kubectl get node <node-name> -o json | jq '{
  gpuClique: .metadata.labels["nvidia.com/gpu.clique"],
  clusterID: .metadata.annotations["topograph.nvidia.com/cluster-id"],
  scaleUp: .metadata.labels["scale-up.unifabric.io/tier-1"]
}'
```

### Node Labels Are Not Written

- Confirm that NVIDIA topograph, node-observer, and node-data-broker are running and have permission to update Nodes.
- Confirm that node-data-broker Pods run on target GPU nodes.
- Confirm that corresponding nodes have the `topograph.nvidia.com/cluster-id` annotation.
- If `nvidiaTopograph.useGpuCliqueLabel` is disabled, confirm that GPU
  Operator's `nvidia-device-plugin-daemonset` is running and node-data-broker
  can create `pods/exec`.
- If ScaleOut labels are not written, confirm that `ibnetdiscover` is available
  in the node-data-broker Pod.
- If `topoDiscovery.*.label.keyTemplate` values are customized, scheduler label keys must be updated accordingly.

### RDMA Metrics Do Not Include IB NICs

- Confirm that the Agent Pod is running and IB devices are visible under `/sys/class/infiniband` on the node.
- Confirm that `fabricNode.scaleOutInterfaceSelector` matches the IB NIC name or CIDR.
- Confirm that `nodeMetrics.enabled=true`.
- If using Prometheus Operator, confirm that `nodeMetrics.serviceMonitor.enabled=true` and the `ServiceMonitor` is selected by Prometheus.

## Uninstall

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

## Next Steps

- Return to the [documentation index](./README.md).
- Read the [Kueue TAS workload example](./usage/workload-tas.md).
- See the [Helm values reference](../chart/README.md).
