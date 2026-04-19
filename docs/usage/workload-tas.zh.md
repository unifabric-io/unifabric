# 拓扑感知调度

数据中心通常会按机架（rack）、区块（block）等组织单元划分节点。同一组织单元内的
节点通常网络距离更近、带宽更好；跨机架或跨区块的节点则距离更远。拓扑感知调度需要
把这些层级关系表达成 Kubernetes Node label，让每个节点标明自己所属的拓扑域。
Unifabric 或 NVIDIA Topograph 负责发现拓扑并写回 Node label；Kueue、Volcano 或
KAI Scheduler 最终消费这些 label 来放置工作负载。

Unifabric 通过 helm values `topologyLabels.*` 统一定义会被调度器消费的 label key：

| 调度层级 | 默认 Node label | 使用说明 |
| --- | --- | --- |
| scale-up | `unifabric.io/scale-up` | GPU 高速互联域，例如 NVLink domain |
| scale-out leaf | `unifabric.io/scale-out-leaf` | leaf 级 scale-out 拓扑域，主机直接上联域 |
| scale-out spine | `unifabric.io/scale-out-spine` | spine 级 scale-out 拓扑域，leaf 的上联域 |
| scale-out core | `unifabric.io/scale-out-core` | core 级 scale-out 拓扑域，spine 的上联域 |
| node | `kubernetes.io/hostname` | Kubernetes 默认节点标签，是最细粒度的节点级拓扑域 |

调度器只能使用已经真实写到 Node 上的 label。配置 Kueue `Topology`、Volcano 或 KAI
Scheduler 时，拓扑层级应按从粗到细排列；如果某一层 label 没有覆盖所有参与调度的节点，
就不要把它加入拓扑配置。

## 安装拓扑发现

使用 TAS 队列前，先按 fabric 场景完成 Unifabric 安装。然后按下面的小节验证拓扑结果，确认
目标节点已经写入预期的 Node label。安装步骤可根据下面不同场景跳转到对应文档。

- [通用 SONiC RoCE](./getting-started-sonic-roce.zh.md)：适用于 SONiC 交换机承载 RoCE 网络的场景。
- [Spectrum-X fabric](./getting-started-spectrum-x.zh.md)：适用于 Spectrum-X 交换机的场景。
- [InfiniBand fabric](./getting-started-infiniband.zh.md)：适用于 NVIDIA InfiniBand 网络场景。

## 验证拓扑结果

验证时以 Node 上真实存在的 label 为准：确认节点已经写入前文提到的
`unifabric.io/` 开头的拓扑 label。不同 fabric 场景会写入的 label 层级可能不同。

```bash
kubectl get nodes -L unifabric.io/scale-up,unifabric.io/scale-out-core,unifabric.io/scale-out-spine,unifabric.io/scale-out-leaf,kubernetes.io/hostname
```

检查：

- 参与调度的节点已经写入预期的 `unifabric.io/` 拓扑 label。

`FabricNode` 和 `ScaleOutLeafGroup` CR 只会在 SONiC RoCE 场景中生成。Spectrum-X 和
InfiniBand 场景通过 NVIDIA topograph 写回 Node label，不会创建或更新这些 CR。

```bash
kubectl get fabricnodes
kubectl get fabricnode <node-name> -o yaml
kubectl get scaleoutleafgroups -o wide
kubectl get scaleoutleafgroup <group-name> -o yaml
```

`FabricNode` 输出示例：

```yaml
apiVersion: unifabric.io/v1beta1
kind: FabricNode
metadata:
  name: node-gpu-1
status:
  conditions:
  - type: Ready
    status: "True"
    reason: Ready
    message: All FabricNode conditions are ready
  - type: LLDPNeighborsReady
    status: "True"
    reason: LLDPNeighborsReady
    message: All selected RDMA interfaces have LLDP neighbors
  totalNics: 2
  nodeRole: GPU
  nodeIP: 192.168.200.6
  scaleOutNics:
  - name: eth1
    rdma: true
    rdmaDeviceName: mlx5_0
    state: up
    ipv4: 172.17.1.11/24
    ipv6: ""
    lldpNeighbor:
      hostname: gpu-leaf-1
      mgmtIP: 10.10.10.1
      mac: 00:11:22:33:44:55
      port: Ethernet1/1
      description: GPU leaf switch
```

`ScaleOutLeafGroup` 输出示例：

```yaml
apiVersion: unifabric.io/v1beta1
kind: ScaleOutLeafGroup
metadata:
  name: 6f8a91c2
status:
  healthy: true
  healthyNodes: 2
  totalNodes: 2
  nodes:
  - name: node-gpu-1
    healthy: true
  - name: node-gpu-2
    healthy: true
  switches:
  - name: gpu-leaf-1
    mgmtIP: 10.10.10.1
  - name: gpu-leaf-2
    mgmtIP: 10.10.10.2
```

- `FabricNode.status.conditions` 中 `Ready` 和 `LLDPNeighborsReady` 为 `True`。
- `ScaleOutLeafGroup.status.nodes` 包含预期节点。
- 节点上出现 `unifabric.io/scale-out-leaf=<group-name>`。

这些检查通过后，说明拓扑识别结果已经通过 Kubernetes Node label 暴露出来，可继续配置
后续调度队列。

## Kueue 示例

示例使用当前 Kueue `kueue.x-k8s.io/v1beta2` API。若集群安装的是较旧版本
Kueue，请按该版本 CRD 支持的 API version 调整。

### 前置条件

- 已按 [文档索引](../README.zh.md) 中对应 fabric 场景完成 Unifabric 安装，并在上一节
  验证过拓扑结果。
- Kueue 已安装，并启用了 `TopologyAwareScheduling` feature gate。
- 参与拓扑感知调度的节点存在 `kubernetes.io/hostname` 标签。
- 参与拓扑感知调度的节点已经写入本示例要使用的拓扑 label，例如
  `unifabric.io/scale-out-leaf`。
- GPU 示例要求节点已上报 `nvidia.com/gpu` 可分配资源。
- 如果 GPU 节点带有 `nvidia.com/gpu:NoSchedule` taint，需要在
  `ResourceFlavor.spec.tolerations` 中声明对应 toleration。

再次确认 Kueue Topology 将消费的 Node label：

```bash
kubectl get nodes -L unifabric.io/scale-up,unifabric.io/scale-out-core,unifabric.io/scale-out-spine,unifabric.io/scale-out-leaf,kubernetes.io/hostname
```

如果某个 GPU 节点没有期望的拓扑 label，先回到上一节或对应安装文档排查拓扑发现组件。

### 创建 Kueue 拓扑感知调度队列

以下示例创建：

- 一个使用 `unifabric.io/scale-out-leaf` Node label 的 Kueue `Topology`。
- 一个引用该 Topology 的 GPU `ResourceFlavor`。
- 一个面向 `training` namespace 的 `LocalQueue`。

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: training
---
apiVersion: kueue.x-k8s.io/v1beta2
kind: Topology
metadata:
  name: unifabric-scaleout
spec:
  levels:
  - nodeLabel: unifabric.io/scale-out-leaf
  - nodeLabel: kubernetes.io/hostname
---
apiVersion: kueue.x-k8s.io/v1beta2
kind: ResourceFlavor
metadata:
  name: unifabric-gpu-tas
spec:
  topologyName: unifabric-scaleout
  tolerations:
  - key: nvidia.com/gpu
    operator: Exists
    effect: NoSchedule
  # 如果集群中只有一部分节点应该进入该 flavor，可按实际标签打开下面配置。
  # nodeLabels:
  #   node-role.kubernetes.io/gpu: "true"
---
apiVersion: kueue.x-k8s.io/v1beta2
kind: ClusterQueue
metadata:
  name: unifabric-gpu-tas
spec:
  namespaceSelector: {}
  resourceGroups:
  - coveredResources:
    - cpu
    - memory
    - nvidia.com/gpu
    flavors:
    - name: unifabric-gpu-tas
      resources:
      - name: cpu
        nominalQuota: 160
      - name: memory
        nominalQuota: 2Ti
      - name: nvidia.com/gpu
        nominalQuota: 16
---
apiVersion: kueue.x-k8s.io/v1beta2
kind: LocalQueue
metadata:
  namespace: training
  name: tas
spec:
  clusterQueue: unifabric-gpu-tas
```

应用后确认队列可用：

```bash
kubectl get topology,resourceflavor,clusterqueue
kubectl -n training get localqueue
```

如果你的集群已经通过 NVIDIA topograph 或其他组件写入了 spine/core label，可以把
Kueue `Topology` 改成更完整的层级。例如：

```yaml
apiVersion: kueue.x-k8s.io/v1beta2
kind: Topology
metadata:
  name: unifabric-scaleout
spec:
  levels:
  - nodeLabel: unifabric.io/scale-out-spine
  - nodeLabel: unifabric.io/scale-out-leaf
  - nodeLabel: kubernetes.io/hostname
```

配置前必须确认所有参与该 `ResourceFlavor` 的节点都实际带有
`unifabric.io/scale-out-spine` label。在 topograph 场景中，不要通过
`ScaleOutLeafGroup` 判断这些 label 是否已经生成，应以 Node 上的真实 label 为准。

### 示例 1：优先调度到同一个 leaf group

`kueue.x-k8s.io/podset-preferred-topology` 表示尽量把同一个 PodSet 放进指定拓扑
层级的同一个 domain。如果单个 leaf group 容量不足，Kueue 可以向更高层或跨
domain 放置，避免工作负载长期 pending。

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  generateName: tas-preferred-
  namespace: training
  labels:
    kueue.x-k8s.io/queue-name: tas
spec:
  parallelism: 4
  completions: 4
  completionMode: Indexed
  template:
    metadata:
      annotations:
        kueue.x-k8s.io/podset-preferred-topology: unifabric.io/scale-out-leaf
    spec:
      restartPolicy: Never
      containers:
      - name: worker
        image: busybox:1.36
        command: ["sh", "-c", "sleep 3600"]
        resources:
          requests:
            cpu: "8"
            memory: 32Gi
            nvidia.com/gpu: "1"
          limits:
            nvidia.com/gpu: "1"
```

适合场景：

- 希望减少跨 leaf RDMA 流量，但更重视吞吐和排队时间。
- 单个训练任务可接受少量跨 leaf 放置。

### 示例 2：强制调度到同一个 leaf group

`kueue.x-k8s.io/podset-required-topology` 表示同一个 PodSet 必须全部放进指定拓扑
层级的同一个 domain。若没有任何 leaf group 能同时容纳所有 Pod，Workload 会保持
未准入状态，直到资源满足要求或配置发生变化。

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  generateName: tas-required-
  namespace: training
  labels:
    kueue.x-k8s.io/queue-name: tas
spec:
  parallelism: 8
  completions: 8
  completionMode: Indexed
  template:
    metadata:
      annotations:
        kueue.x-k8s.io/podset-required-topology: unifabric.io/scale-out-leaf
    spec:
      restartPolicy: Never
      containers:
      - name: worker
        image: busybox:1.36
        command: ["sh", "-c", "sleep 3600"]
        resources:
          requests:
            cpu: "8"
            memory: 32Gi
            nvidia.com/gpu: "1"
          limits:
            nvidia.com/gpu: "1"
```

适合场景：

- AllReduce、AllGather 等对跨 leaf 带宽敏感的训练任务。
- 拓扑不满足时宁可排队，也不希望跨 leaf 运行。

### 验证调度结果

查看 Kueue Workload 准入状态：

```bash
kubectl -n training get workloads.kueue.x-k8s.io
kubectl -n training describe workloads.kueue.x-k8s.io <workload-name>
```

查看 Pod 实际落点：

```bash
kubectl -n training get pods -o wide
```

查看 Pod 所在节点的拓扑 label：

```bash
kubectl get nodes -L unifabric.io/scale-up,unifabric.io/scale-out-core,unifabric.io/scale-out-spine,unifabric.io/scale-out-leaf,kubernetes.io/hostname
```

如果 Job annotation 使用的是 `unifabric.io/scale-out-leaf`：

- 对于 `required` 示例，所有 Pod 所在节点的 `unifabric.io/scale-out-leaf` 值应相同。
- 对于 `preferred` 示例，Kueue 会先尝试同 leaf 放置；当资源不足时，可能会跨 leaf
  放置。

如果 Job annotation 使用的是 `unifabric.io/scale-out-spine` 或其他更高层级 label，
则应检查所有 Pod 所在节点在对应层级的 label 值是否相同。

### 排障

Workload 一直未准入：

- 检查 `kubectl -n training describe workloads.kueue.x-k8s.io <workload-name>` 中
  是否提示 topology domain 容量不足。
- 确认 `ClusterQueue` 的 `nominalQuota` 覆盖了 Job 请求的 `cpu`、`memory` 和
  `nvidia.com/gpu`。
- 确认目标节点 Ready、未 cordon，且 `status.allocatable` 中有足够资源。
- 确认参与调度的节点同时具有 Kueue `Topology.spec.levels[*].nodeLabel` 中声明的
  所有 label。
- 如果 `Topology.spec.levels` 中加入了 `unifabric.io/scale-up`、
  `unifabric.io/scale-out-spine` 或 `unifabric.io/scale-out-core`，确认这些 label
  已经写入 Node。

Pod 已准入但无法调度：

- 检查 GPU taint 是否已由 `ResourceFlavor.spec.tolerations` 覆盖。
- 如果启用了 `ResourceFlavor.spec.nodeLabels`，确认对应节点确实带有这些标签。
- 查看 Pod 事件：

  ```bash
  kubectl -n training describe pod <pod-name>
  ```

拓扑 label 缺失或不稳定：

- 回到对应安装文档排查拓扑发现组件：
  [通用 SONiC RoCE](../getting-started-sonic-roce.zh.md)、
  [Spectrum-X fabric](../getting-started-spectrum-x.zh.md) 或
  [InfiniBand fabric](../getting-started-infiniband.zh.md)。
- 确认 Node 上真实存在的拓扑 label：

  ```bash
  kubectl get nodes -L unifabric.io/scale-up,unifabric.io/scale-out-core,unifabric.io/scale-out-spine,unifabric.io/scale-out-leaf,kubernetes.io/hostname
  ```

- 如果自定义了 Helm values 中的 `topologyLabels.*`，Kueue
  `Topology.spec.levels[*].nodeLabel` 和 Job annotation 的值也必须同步更新。
