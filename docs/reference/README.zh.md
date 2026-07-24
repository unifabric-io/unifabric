# Unifabric API 参考

English version: [README.md](./README.md)

Unifabric 提供 3 个 `unifabric.io/v1beta1` 集群级自定义资源。每个 CRD 都有独立的
API 参考页：

| Kind | 资源名 | 缩写 | 说明 |
| --- | --- | --- | --- |
| [`FabricNode`](./fabricnode.zh.md) | `fabricnodes` | `fn` | 节点 Agent 上报的 RDMA 网卡、LLDP 邻居和 RDMA Pod 状态 |
| [`Switch`](./switch.zh.md) | `switches` | `sw` | 交换机声明、switch-agent 连接和交换机 LLDP 状态 |
| [`Topology`](./topology.zh.md) | `topologies` | `topo` | scale-out、scale-up 和 storage 的只读汇总拓扑 |

查看集群中安装的 API：

```bash
kubectl api-resources --api-group=unifabric.io
kubectl get fn
kubectl get sw
kubectl get topo
```

`spec` 表示期望状态，由用户或组件声明；`status` 表示观测状态，由 Unifabric 组件维护。
除非文档明确说明，否则不要手工编辑 `status`。

CRD 的 OpenAPI schema 以 [`chart/crds`](../../chart/crds/) 中生成的清单为准。
