# 拓扑感知调度

English version: [workload-tas.md](./workload-tas.md)

## 简介

拓扑感知调度把同一个 workload 的 Pod 放进同一个网络拓扑域,让跨节点 RDMA 流量走
短路径。Unifabric 或 NVIDIA Topograph 发现拓扑,并以标签形式写到 Kubernetes Node 上;
Kueue、Volcano 或 KAI Scheduler 等调度器消费这些标签。调度器只能使用真实存在于
Node 上的标签。本文以 Kueue 演示完整路径;Volcano 和 KAI Scheduler 通过各自的配置
消费相同的标签。

Helm values `topoDiscovery.*.nodeLabel.keyTemplate` 定义标签键,默认值如下:

| 调度层级 | 默认 Node 标签 | 含义 |
| --- | --- | --- |
| scale-up tier 1 | `scale-up.unifabric.io/tier-1` | GPU 高速互联域,如 NVLink 域 |
| scale-out tier 1 | `scale-out.unifabric.io/tier-1` | 第一层 scale-out,直连主机 |
| scale-out tier 2 | `scale-out.unifabric.io/tier-2` | 第二层 scale-out,位于 tier 1 上游 |
| scale-out tier 3 | `scale-out.unifabric.io/tier-3` | 第三层 scale-out,位于 tier 2 上游 |
| 节点 | `kubernetes.io/hostname` | Kubernetes 默认节点标签,最细粒度的拓扑域 |

> **注意:** 如果自定义了 `topoDiscovery.*.nodeLabel.keyTemplate`,本文中所有调度器
> 配置和 workload 注解都必须改用自定义后的键。

## 前提条件

- 已按[基础安装](../getting-started.zh.md)安装 Unifabric,并完成对应网络场景的指南:
  [SONiC RoCE](../getting-started-sonic-roce.zh.md)、
  [Spectrum-X](../getting-started-spectrum-x.zh.md) 或
  [InfiniBand](../getting-started-infiniband.zh.md)。
- 已安装 Kueue,并开启 `TopologyAwareScheduling` feature gate。
- GPU 示例要求节点上报可分配的 `nvidia.com/gpu` 资源。

## 配置与使用

### 第 1 步:确认 Node 上存在拓扑标签

Node 上实际存在的标签是调度器的事实源。不同网络场景写入的标签层级不同:

```bash
kubectl get nodes -L scale-up.unifabric.io/tier-1,scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2,scale-out.unifabric.io/tier-3,kubernetes.io/hostname
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

参与拓扑感知调度的每个 Node 都必须拥有 Kueue `Topology` 将要引用的标签。如果某个
标签层级没有覆盖所有参与的 Node,不要把该层级加入 Kueue `Topology`。

### 第 2 步:创建 Kueue 队列

下面的示例创建四个资源:Kueue `Topology` 按从粗到细列出拓扑层级,并以
`kubernetes.io/hostname` 结尾;`ResourceFlavor` 把 GPU 节点绑定到该 `Topology`,
并容忍 `nvidia.com/gpu:NoSchedule` 污点;`ClusterQueue` 设置资源配额;`LocalQueue`
把 `ClusterQueue` 暴露给 `training` 命名空间。

> **注意:** 示例使用 Kueue `kueue.x-k8s.io/v1beta2` API。如果集群运行较旧的
> Kueue,把 API 版本改成其 CRD 支持的版本。

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
  - nodeLabel: scale-out.unifabric.io/tier-1
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

> **注意:** 如果只允许部分节点进入该 flavor,把 `ResourceFlavor.spec.nodeLabels`
> 设置为这些节点上真实存在的标签,例如 `node-role.kubernetes.io/gpu: "true"`。

应用文件后,确认队列已就绪:

```bash
kubectl apply -f tas-queues.yaml
kubectl get topology,resourceflavor,clusterqueue
kubectl -n training get localqueue
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

> **注意:** 当所有参与的 Node 也都带有 `scale-out.unifabric.io/tier-2` 标签时,
> 可以在 tier 1 之上增加该层级,构成更深的层次:
>
> ```yaml
> spec:
>   levels:
>   - nodeLabel: scale-out.unifabric.io/tier-2
>   - nodeLabel: scale-out.unifabric.io/tier-1
>   - nodeLabel: kubernetes.io/hostname
> ```
>
> 增加层级之前,必须确认该 flavor 中的每个 Node 都有该标签。

### 第 3 步:为 Workload 加注解

Job 通过 Pod 模板上的一个注解选择放置规则,分两种 case:

- Case 1,偏好同一 tier 1 域:吞吐和排队时间比严格的局部性更重要,作业可以接受
  部分跨域放置。
- Case 2,要求同一 tier 1 域:作业对跨域带宽敏感,例如 AllReduce、AllGather 训练,
  宁可等待也不跨域运行。

#### Case 1:偏好同一 tier 1 域

`kueue.x-k8s.io/podset-preferred-topology` 注解让 Kueue 尽量把整个 PodSet 放进
指定层级的同一个域。当单个域容量不足时,Kueue 在更高层级或跨域放置,避免长时间
pending。

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
        kueue.x-k8s.io/podset-preferred-topology: scale-out.unifabric.io/tier-1
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

#### Case 2:要求同一 tier 1 域

`kueue.x-k8s.io/podset-required-topology` 注解要求 Kueue 把整个 PodSet 放进指定
层级的同一个域。当没有任何域能同时容纳所有 Pod 时,Workload 保持未准入,直到资源
或配置发生变化。

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
        kueue.x-k8s.io/podset-required-topology: scale-out.unifabric.io/tier-1
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

## 验证

提交 Case 1 或 Case 2 的 Job,确认 Kueue 已准入该 Workload:

```bash
kubectl create -f tas-job.yaml
kubectl -n training get workloads.kueue.x-k8s.io
kubectl -n training describe workloads.kueue.x-k8s.io <workload-name>
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

确认 Pod 落到共享同一拓扑域的 Node 上:

```bash
kubectl -n training get pods -o wide
kubectl get nodes -L scale-out.unifabric.io/tier-1
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

满足以下条件时验证通过:

- Case 2:所有 Pod 所在 Node 的 `scale-out.unifabric.io/tier-1` 取值相同。
- Case 1:Kueue 先尝试同一个域;容量不足时 Pod 可以跨域,Workload 仍被准入。
- 当注解使用更高层级(如 `scale-out.unifabric.io/tier-2`)时,所有 Pod 所在 Node
  在该层级的标签取值相同。

## 故障排查

### Workload 保持未准入

先确认前提条件成立:参与的 Node 拥有 `Topology.spec.levels[*].nodeLabel` 声明的
所有标签,且 Node 处于 Ready 状态、未被 cordon。

1. 查看准入事件:

   ```bash
   kubectl -n training describe workloads.kueue.x-k8s.io <workload-name>
   ```

   如果事件提示拓扑域容量不足,说明没有任何单个域能容纳该 PodSet。

2. 确认 `ClusterQueue` 的 `nominalQuota` 覆盖 Job 申请的 `cpu`、`memory` 和
   `nvidia.com/gpu`。
3. 确认目标 Node 的 `status.allocatable` 有足够资源。

### Workload 已准入但 Pod 一直 Pending

1. 确认 `ResourceFlavor.spec.tolerations` 覆盖了 GPU 污点。
2. 如果设置了 `ResourceFlavor.spec.nodeLabels`,确认 Node 上确实有这些标签。
3. 查看 Pod 事件:

   ```bash
   kubectl -n training describe pod <pod-name>
   ```

### 拓扑标签缺失或不稳定

1. 列出 Node 上实际存在的标签:

   ```bash
   kubectl get nodes -L scale-up.unifabric.io/tier-1,scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2,scale-out.unifabric.io/tier-3,kubernetes.io/hostname
   ```

2. SONiC RoCE 场景下,检查发现输入。Spectrum-X 和 InfiniBand 场景通过 NVIDIA
   Topograph 写标签,不创建这些资源;这两个场景直接查阅场景指南。

   ```bash
   kubectl get fabricnodes
   kubectl get fabricnode <node-name> -o yaml
   ```

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

   确认 `status.conditions` 中 `Ready` 和 `LLDPNeighborsReady` 为 `True`。

   ```bash
   kubectl get switches -o wide
   kubectl get switch <switch-name> -o yaml
   ```

   ```yaml
   apiVersion: unifabric.io/v1beta1
   kind: Switch
   metadata:
     name: gpu-leaf-1
   spec:
     mgmtIP: 10.10.10.1
     role: ScaleOut
   status:
     hostname: gpu-leaf-1
     healthy: true
     conditions:
     - type: Connected
       status: "True"
       reason: StreamReady
     lldpNeighborCount: 2
     lldpNeighbors:
     - remoteSystemType: KubernetesNode
       remoteSystemName: node-gpu-1
     - remoteSystemType: Switch
       remoteSystemName: gpu-spine-1
   ```

   确认 `status.healthy` 为 `true`,且 `status.lldpNeighbors` 包含预期的节点与
   交换机邻居。

3. 如果标签仍然缺失,进入对应场景指南的故障排查章节:
   [SONiC RoCE](../getting-started-sonic-roce.zh.md)、
   [Spectrum-X](../getting-started-spectrum-x.zh.md) 或
   [InfiniBand](../getting-started-infiniband.zh.md)。

## 参考

- [Topology 参考](../reference/topology.zh.md):聚合后的拓扑资源。
- [FabricNode 参考](../reference/fabricnode.zh.md):节点侧发现输入。
- [Switch 参考](../reference/switch.zh.md):交换机侧发现输入。
- [Scale-out 拓扑设计](../design/scaleout-topology.zh.md):拓扑分层与标签的生成方式。
