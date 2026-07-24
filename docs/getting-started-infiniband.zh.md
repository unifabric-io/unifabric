# InfiniBand Fabric

本文说明如何在 InfiniBand 网卡集群中部署 Unifabric。该场景适用于 IB 网络，例如 Mellanox
网卡运行在 IB 模式并连接 IB 交换机。

## 部署目标

完成部署后，集群中应达成两个目标：

- Node 会获得供调度系统使用的 `scale-up.unifabric.io/tier-N` 和
  `scale-out.unifabric.io/tier-N` 拓扑 label。`N` 从 1 开始，数字越小表示拓扑距离越近，
  通信性能通常越高。
- 可通过 Unifabric Agent metrics 和内置 RDMA Grafana Dashboard 观测节点 RDMA 状态，
  按集群、节点、Pod 和 Workload 维度查看吞吐、利用率、QoS、拥塞和错误指标。

## 前置条件

- Kubernetes 集群，包含目标 GPU 节点。
- 已安装 `kubectl` 和 `helm` cli。
- 节点上存在 InfiniBand / RDMA 设备，并能在 `/sys/class/infiniband` 下看到。
- 集群需要安装 Prometheus Operator 和 Grafana Operator，如果未安装，请在安装 Unifabric 时取消下发 ServiceMonitor
  和 GrafanaDashboard，避免下发 CRD 失败。

确认当前集群连接：

```bash
kubectl cluster-info
kubectl get nodes -o wide
```

## 安装 Unifabric

以下命令使用最新的 release 版本。示例将 RDMA interface selector 留空，因此所有 RDMA
网卡都会被 metrics 观测。InfiniBand 拓扑 label 由 NVIDIA topograph 写回。

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

参数说明：

| Helm value | 用途 |
| --- | --- |
| `topoDiscovery.scaleUp.mode` | 设置为 `nv-topograph`，由 NVIDIA Topograph 写入 scale-up label；该设置与 scale-out mode 相互独立。 |
| `topoDiscovery.scaleOut.mode` | 设置为 `nv-topograph`，由 NVIDIA Topograph 写入 scale-out label。 |
| `topoDiscovery.storage.mode` | 设置为 `unifabric-ib`，使用内置 InfiniBand storage 发现。 |
| `nvidiaTopograph.provider.name` | 该场景默认使用 `infiniband-k8s`，无需额外设置。 |
| `nvidiaTopograph.useGpuCliqueLabel` | 默认 `true`，使用 GPU Operator clique label 作为加速卡拓扑来源；设为 `false` 时才需要 node-data-broker 通过 `pods/exec` 发现拓扑。 |
| `nodeMetrics.enabled` | 开启 Agent Metrics 用于节点 RDMA 可观测。 |
| `fabricNode.scaleOutInterfaceSelector` | 选择参与 scale-out 拓扑和 RDMA 指标观测的 RDMA 网卡，并在 RDMA 指标中打上 `kind=scaleOut` 标签。支持 `interface=ib*,mlx*` 或 `cidr=172.17.0.0/16`，默认为全部 RDMA 网卡。 |
| `fabricNode.storageInterfaceSelector` | 选择一组 RDMA 存储网卡观测，并在 RDMA 指标中打上 `kind=storage` 标签。支持 `interface=ib*,mlx*` 或 `cidr=172.17.0.0/16`。默认为空。 |
| `nodeMetrics.serviceMonitor.enabled` | 创建 Prometheus Operator 使用的 `ServiceMonitor`。 |
| `grafanaDashboard.enabled` | 渲染内置 RDMA Dashboard。 |

更多 Helm 参数见 [chart/README.md](../chart/README.md)。

如果您位于中国地区，可以额外增加下面的参数，加速下载：

```bash
--set global.registry=m.daocloud.io \
--set controller.image.repository=ghcr.io/unifabric-io/unifabric-controller \
--set agent.image.repository=ghcr.io/unifabric-io/unifabric-agent \
--set nvidiaTopograph.image.repository=ghcr.io/nvidia/topograph
```

## 验证部署

重点检查 topograph 组件、node-data-broker DaemonSet、Node annotation 和 Node label：

```bash
kubectl -n unifabric-system get pods
kubectl get pods -n unifabric-system -o wide
kubectl get fabricnodes.unifabric.io
kubectl get nodes -L scale-up.unifabric.io/tier-1,scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2,scale-out.unifabric.io/tier-3,kubernetes.io/hostname
```

FabricNode 列表应包含所有目标节点，且 `READY` 为 `True`：

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

查看单个节点的 Agent 上报结果：

```bash
kubectl get fabricnode <node-name> -o yaml
```

重点确认 `status.nodeRole`、`status.scaleOutNics` 和 `status.conditions` 符合预期，
参与发现的 IB 网卡状态为 `up`。`FabricNode` 也可以使用缩写 `fn` 查询。

ScaleOut 拓扑发现成功后，会生成 `scaleout` Topology：

```bash
kubectl get topo
```

```text
NAME       AGE
scaleout   113m
```

查看完整结果：

```bash
kubectl get topo scaleout -o yaml
```

以下示例展示了一个 leaf、spine、core 三层拓扑。`status.domains` 描述性能域之间的父子关系，
`status.nodes[].domainPath` 按 tier 3 到 tier 1 的顺序记录节点所在的完整路径：

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

配置 Kueue、Volcano 或 KAI Scheduler 时，应只使用上述命令中已经真实写到 Node 上的 label。

验证 RDMA metrics 资源：

```bash
kubectl -n unifabric-system get service unifabric-agent-metrics
kubectl -n unifabric-system get servicemonitor unifabric-agent-metrics
```

直接检查 Agent metrics 端点：

```bash
POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-agent -o jsonpath='{.items[0].status.podIP}')
curl -s "http://${POD_IP}:8082/metrics" | grep '^unifabric_'
```

## 常见问题

### NVIDIA Topograph ScaleUp 发现方式区别

- `nvidiaTopograph.useGpuCliqueLabel=true` 时，Topograph 直接使用 GPU Operator 写入的
  `nvidia.com/gpu.clique` label，不需要 node-data-broker 进入 NVIDIA device-plugin Pod
  采集 Clique ID。
- `nvidiaTopograph.useGpuCliqueLabel=false` 时，node-data-broker 默认查找
  `gpu-operator` namespace 中、与目标 Node 同节点运行的
  `nvidia-device-plugin-daemonset` Pod，并通过 `pods/exec` 执行：

  ```bash
  nvidia-smi -q | grep "ClusterUUID\|CliqueId" | sort -u
  ```

  采集结果会组合为 `<ClusterUUID>.<CliqueId>`，写入 Node 的
  `topograph.nvidia.com/cluster-id` annotation，再作为
  `scale-up.unifabric.io/tier-1` 的数据来源。Chart 仅在
  `provider.name=infiniband-k8s` 且该开关为 `false` 时，为 node-data-broker
  增加读取 DaemonSet、读取 Pod 和创建 `pods/exec` 的权限。
- `ibnetdiscover` 用于发现 ScaleOut 的 InfiniBand switch 层级，由 Topograph
  进入 node-data-broker Pod 执行；它不受 `useGpuCliqueLabel` 控制。
- `ClusterUUID=00000000-0000-0000-0000-000000000000` 表示没有有效的多节点
  NVLink 域。Topograph v0.5.0 不会排除这个全零值，可能生成无效的
  `topograph.nvidia.com/cluster-id` 或 scale-up label，不应将其用于调度。

可以检查 Node 上的输入和输出：

```bash
kubectl get node <node-name> -o json | jq '{
  gpuClique: .metadata.labels["nvidia.com/gpu.clique"],
  clusterID: .metadata.annotations["topograph.nvidia.com/cluster-id"],
  scaleUp: .metadata.labels["scale-up.unifabric.io/tier-1"]
}'
```

### Node label 没有写入

- 确认 NVIDIA topograph、node-observer 和 node-data-broker 组件正常运行，并且有权限更新 Node。
- 确认 node-data-broker Pod 已运行在目标 GPU 节点。
- 确认对应节点有 `topograph.nvidia.com/cluster-id` annotation。
- 如果关闭了 `nvidiaTopograph.useGpuCliqueLabel`，确认 GPU Operator 的
  `nvidia-device-plugin-daemonset` 正常运行，并且 node-data-broker 可以创建
  `pods/exec`。
- 如果 ScaleOut label 没有写入，确认 node-data-broker Pod 中的 `ibnetdiscover` 可用。
- 如果自定义了 Helm values 中的 `topoDiscovery.*.label.keyTemplate`，调度器配置中的 label key 也必须同步更新。

### RDMA metrics 没有采集到 IB 网卡

- 确认 Agent Pod 正常运行，并且节点上能在 `/sys/class/infiniband` 下看到 IB 设备。
- 确认 `fabricNode.scaleOutInterfaceSelector` 匹配节点上的 IB 网卡名称或 CIDR。
- 确认 `nodeMetrics.enabled=true`。
- 如果使用 Prometheus Operator，确认 `nodeMetrics.serviceMonitor.enabled=true` 且
  `ServiceMonitor` 能被 Prometheus selector 选中。

## 卸载

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

## 下一步

- 返回 [文档索引](./README.zh.md)。
- 阅读 [Kueue TAS 工作负载示例](./usage/workload-tas.zh.md)。
- 查看 [Helm values 参考](../chart/README.md)。
