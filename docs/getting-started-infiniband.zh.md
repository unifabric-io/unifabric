# 在 InfiniBand 网络部署 Unifabric

English version: [getting-started-infiniband.md](./getting-started-infiniband.md)

## 简介与部署目标

本文指导你在使用 InfiniBand 网络的集群中部署 Unifabric 拓扑发现,例如 Mellanox
网卡运行在 IB 模式并连接 IB 交换机。在该环境中,NVIDIA Topograph 负责发现 fabric
拓扑,Unifabric 把结果写入 Node label。

下图是本文的示例拓扑:tier 3 core、tier 2 spine、tier 1 leaf 三层网络,四个
节点分别位于两个 tier 1 性能域:

![InfiniBand 三层拓扑示例](images/infiniband-topology-example.png)

Scale-out 拓扑发现完成后,四个节点带有以下 label:

| 节点 | 预期 label |
| --- | --- |
| `node1` | `scale-out.unifabric.io/tier-1=S-fc6a1c03006636c0`<br>`scale-out.unifabric.io/tier-2=S-fc6a1c0300afca40`<br>`scale-out.unifabric.io/tier-3=S-fc6a1c0300b03c40` |
| `node2` | `scale-out.unifabric.io/tier-1=S-fc6a1c03006636c0`<br>`scale-out.unifabric.io/tier-2=S-fc6a1c0300afca40`<br>`scale-out.unifabric.io/tier-3=S-fc6a1c0300b03c40` |
| `node3` | `scale-out.unifabric.io/tier-1=S-fc6a1c03006637c0`<br>`scale-out.unifabric.io/tier-2=S-fc6a1c0300afca40`<br>`scale-out.unifabric.io/tier-3=S-fc6a1c0300b03c40` |
| `node4` | `scale-out.unifabric.io/tier-1=S-fc6a1c03006637c0`<br>`scale-out.unifabric.io/tier-2=S-fc6a1c0300afca40`<br>`scale-out.unifabric.io/tier-3=S-fc6a1c0300b03c40` |

tier 数字越大,位置越靠近网络上层。Scale-up label 来自节点内的 GPU 互联拓扑,
不在上图中展示。

部署完成后,Agent metrics 和内置 RDMA Grafana 看板可以展示节点 RDMA 状态:按
集群、节点、Pod 和 Workload 维度查看吞吐、利用率、QoS、拥塞和错误指标。

## 前提条件

继续之前,确认以下各项均成立:

- [ ] 已通过[基础安装](./getting-started.zh.md)完成 Unifabric 安装。
- [ ] 节点上存在 IB 模式的 RDMA 设备。

## 安装

先完成[基础安装](./getting-started.zh.md)。基础安装部署了所有场景共用的
Controller、Agent 和观测组件。本文不需要其他安装步骤。

## 环境配置

为 InfiniBand 的 scale-up 和 scale-out 网络选择 NVIDIA Topograph 作为写入器。
该命令只修改场景相关的 mode,保留基础安装中的其他 values。在可以用 `kubectl`
和 Helm 访问集群的主机上执行:

```bash
helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set topoDiscovery.scaleUp.mode=nv-topograph \
  --set topoDiscovery.scaleOut.mode=nv-topograph \
  --wait
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

## 验证

检查 topograph 组件、node-data-broker DaemonSet 和 Node label:

```bash
kubectl -n unifabric-system get pods
kubectl get pods -n unifabric-system -o wide
kubectl get fabricnodes.unifabric.io
kubectl get nodes -L scale-up.unifabric.io/tier-1,scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2,scale-out.unifabric.io/tier-3,kubernetes.io/hostname
```

`FabricNode` 列表包含所有目标节点,且 `READY` 为 `True`:

```text
NAME                TOTALNICS   READY   ROLE   NODEIP
node1               2           True    GPU    10.0.0.1
node2               2           True    GPU    10.0.0.2
node3               2           True    GPU    10.0.0.3
node4               2           True    GPU    10.0.0.4
```

查看单个节点的 Agent 上报结果。缩写 `fn` 也可以查询 `FabricNode` 资源:

```bash
kubectl get fabricnode <node-name> -o yaml
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

确认 `status.nodeRole`、`status.scaleOutNics` 和 `status.conditions` 符合预期,
且参与发现的 IB 网卡状态为 `up`。

Scale-out 拓扑发现成功后,Controller 会创建 `scaleout` Topology:

```bash
kubectl get topo
```

```text
NAME       AGE
scaleout   113m
```

查看完整结果。`status.domains` 描述性能域之间的父子关系,
`status.nodes[].switchDomainPath` 按 tier 3 到 tier 1 的顺序记录节点所在的完整
路径:

```bash
kubectl get topo scaleout -o yaml
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

配置 Kueue、Volcano 或 KAI Scheduler 时,只使用已经真实写到 Node 上的 label。

## 故障排查

先按[基础安装故障排查](./getting-started.zh.md#故障排查)排除 `FabricNode` 和
RDMA 指标的通用问题。

### Node label 没有写入

1. 确认 NVIDIA topograph、node-observer 和 node-data-broker 正常运行,并且有
   权限更新 Node。
2. 确认 node-data-broker Pod 已运行在目标 GPU 节点上。
3. 如果 scale-out label 没有写入,确认 node-data-broker Pod 中的
   `ibnetdiscover` 可用。
4. 如果自定义了 `topoDiscovery.*.nodeLabel.keyTemplate`,同步更新调度器配置中的
   label key。

## 卸载

删除 Helm release:

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

如果不再需要 CRD,删除它们。此操作会同时删除所有 `FabricNode`、`Switch` 和
`Topology` 资源:

```bash
kubectl delete crd fabricnodes.unifabric.io switches.unifabric.io topologies.unifabric.io
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

## 下一步

- 在调度器中使用这些 label:参见 [Kueue TAS 工作负载示例](./usage/workload-tas.zh.md)。
- 渲染拓扑:参见[拓扑可视化指南](./usage/topology-visualization.zh.md)。
- 查阅字段:参见 [FabricNode 参考](./reference/fabricnode.zh.md)。
- 调整安装配置:参见 [Helm values 参考](../chart/README.md)。
