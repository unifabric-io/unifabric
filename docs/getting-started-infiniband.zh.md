# InfiniBand Fabric

本文说明如何在 InfiniBand 网卡集群中部署 Unifabric。该场景适用于 IB 网络，例如 Mellanox
网卡运行在 IB 模式并连接 IB 交换机。

## 示例拓扑和部署目标

下图展示一个 tier 3 core、tier 2 spine、tier 1 leaf 三层拓扑，四个节点分别位于两个
tier 1 性能域：

![InfiniBand 三层拓扑示例](images/infiniband-topology-example.png)

Scale-out 拓扑发现完成后，四个节点预期获得以下 label：

| 节点 | 预期 label |
| --- | --- |
| `node1` | `scale-out.unifabric.io/tier-1=S-fc6a1c03006636c0`<br>`scale-out.unifabric.io/tier-2=S-fc6a1c0300afca40`<br>`scale-out.unifabric.io/tier-3=S-fc6a1c0300b03c40` |
| `node2` | `scale-out.unifabric.io/tier-1=S-fc6a1c03006636c0`<br>`scale-out.unifabric.io/tier-2=S-fc6a1c0300afca40`<br>`scale-out.unifabric.io/tier-3=S-fc6a1c0300b03c40` |
| `node3` | `scale-out.unifabric.io/tier-1=S-fc6a1c03006637c0`<br>`scale-out.unifabric.io/tier-2=S-fc6a1c0300afca40`<br>`scale-out.unifabric.io/tier-3=S-fc6a1c0300b03c40` |
| `node4` | `scale-out.unifabric.io/tier-1=S-fc6a1c03006637c0`<br>`scale-out.unifabric.io/tier-2=S-fc6a1c0300afca40`<br>`scale-out.unifabric.io/tier-3=S-fc6a1c0300b03c40` |

tier 数字越大表示越靠近网络上层。Scale-up label 由节点内的 GPU 高速互联拓扑决定，
不在上图中展示。

部署完成后，还可以通过 Unifabric Agent metrics 和内置 RDMA Grafana Dashboard 观测
节点 RDMA 状态，按集群、节点、Pod 和 Workload 维度查看吞吐、利用率、QoS、拥塞和错误指标。

## 前置条件

- 已按照[基础安装](./getting-started.zh.md)完成 Unifabric 安装。
- 节点上存在 IB 模式的 RDMA 设备。


## 配置 InfiniBand 拓扑发现

先按照[基础安装](./getting-started.zh.md)完成通用组件安装，再为 InfiniBand scale-up
和 scale-out 网络执行以下 Helm upgrade。该命令只设置当前场景相关的 mode，基础安装
中的其他 values 保持不变。

```bash
helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set topoDiscovery.scaleUp.mode=nv-topograph \
  --set topoDiscovery.scaleOut.mode=nv-topograph \
  --wait
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
node1               2           True    GPU    10.0.0.1
node2               2           True    GPU    10.0.0.2
node3               2           True    GPU    10.0.0.3
node4               2           True    GPU    10.0.0.4
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
    - name: S-fc6a1c03006637c0
      parent: S-fc6a1c0300afca40
      tier: 1
  nodes:
    - domainPath:
        - S-fc6a1c0300b03c40
        - S-fc6a1c0300afca40
        - S-fc6a1c03006636c0
      nodes:
        - node1
        - node2
    - domainPath:
        - S-fc6a1c0300b03c40
        - S-fc6a1c0300afca40
        - S-fc6a1c03006637c0
      nodes:
        - node3
        - node4
```

配置 Kueue、Volcano 或 KAI Scheduler 时，应只使用上述命令中已经真实写到 Node 上的 label。

## 常见问题

`FabricNode` 和 RDMA 指标的通用排障请参阅
[基础安装常见问题](./getting-started.zh.md#常见问题)。

### Node label 没有写入

- 确认 NVIDIA topograph、node-observer 和 node-data-broker 组件正常运行，并且有权限更新 Node。
- 确认 node-data-broker Pod 已运行在目标 GPU 节点。
- 如果 ScaleOut label 没有写入，确认 node-data-broker Pod 中的 `ibnetdiscover` 可用。
- 如果自定义了 Helm values 中的 `topoDiscovery.*.nodeLabel.keyTemplate`，调度器配置中的 label key 也必须同步更新。

## 卸载

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

## 下一步

- 返回 [文档索引](./README.zh.md)。
- 阅读 [Kueue TAS 工作负载示例](./usage/workload-tas.zh.md)。
- 查看 [Helm values 参考](../chart/README.md)。
