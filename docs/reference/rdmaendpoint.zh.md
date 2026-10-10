# RDMAEndpoint

English: [rdmaendpoint.md](./rdmaendpoint.md)

`RDMAEndpoint` 是 Agent 为本节点上每个持有 RDMA 设备的 Pod 发布的命名空间级、
只有 status 的资源。它记录设备、端口、GID、LID、子网前缀以及进入过 RTR 的
QP，其他节点的 Agent 据此把发往节点网卡或 InfiniBand LID 的流量归属回接收
Pod。它没有 `spec`。

该资源只在 `agent.ebpfFlow.enabled` 为 true 时存在，
见 [eBPF RDMA 流归属设计](../design/rdma-flow.zh.md)。

## 示例

```yaml
apiVersion: unifabric.io/v1beta1
kind: RDMAEndpoint
metadata:
  name: vllm-decode-0
  namespace: tenant-a
  labels:
    app.kubernetes.io/managed-by: unifabric-agent
    unifabric.io/node: gpu-node-03
  ownerReferences:
    - apiVersion: v1
      kind: Pod
      name: vllm-decode-0
      uid: 64e05f6d-91a3-45e8-8c55-e988b2d16271
      controller: true
      blockOwnerDeletion: true
status:
  nodeName: gpu-node-03
  hostNetwork: true
  lastUpdateTime: "2026-09-23T03:12:41Z"
  interfaces:
    - name: ibs576
      rdmaDevice: mlx5_0
      ports:
        - port: 1
          linkLayer: InfiniBand
          lid: 17
          subnetPrefix: fe80000000000000
          gids:
            - index: 0
              gid: fe80:0000:0000:0000:a088:c203:0088:6a44
          queuePairs:
            - qpn: 1234
              pid: 48213
    - name: eth1
      rdmaDevice: mlx5_2
      ports:
        - port: 1
          linkLayer: Ethernet
          lid: 0
          gids:
            - index: 3
              gid: 0000:0000:0000:0000:0000:ffff:ac11:0367
```

## 资源信息

| 字段 | 值 |
| --- | --- |
| API group | `unifabric.io` |
| API version | `v1beta1` |
| Kind | `RDMAEndpoint` |
| 复数 | `rdmaendpoints` |
| 缩写 | `rep` |
| 作用域 | 命名空间级，与 Pod 同命名空间 |
| 子资源 | `status` |

打印列：`Node`、`HostNetwork`、`Updated`。

## Metadata

| 字段 | 含义 |
| --- | --- |
| `metadata.name` | Pod 名。 |
| `metadata.labels["unifabric.io/node"]` | 发布该对象的 Agent 所在节点，重启后的 Agent 据此接管对象。 |
| `metadata.ownerReferences` | 指向 Pod 的唯一 controller 引用，对象随 Pod 被垃圾回收。 |

## Status

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| `nodeName` | string | 运行该 Pod 并发布对象的节点。 |
| `hostNetwork` | bool | Pod 是否共享节点网络命名空间。是则其 RDMA 地址属于节点网卡，只能经本对象归属。 |
| `lastUpdateTime` | time | Agent 最后一次观察到该 Pod 的时间。读取端把超过 15 分钟未刷新的对象视为过期。 |
| `interfaces[]` | list | Pod 持有的 RDMA 设备。 |
| `interfaces[].name` | string | 绑定到设备的 netdev，Pod 内不可见时为空。 |
| `interfaces[].rdmaDevice` | string | ibdev 名，如 `mlx5_0`。 |
| `interfaces[].ports[]` | list | 设备的端口。 |
| `interfaces[].ports[].port` | int | 从 1 开始的端口号。 |
| `interfaces[].ports[].linkLayer` | enum | `InfiniBand` 或 `Ethernet`。 |
| `interfaces[].ports[].lid` | int | 子网管理器分配的 LID，Ethernet 上以及没有活动 SM 的 InfiniBand 上为 0。 |
| `interfaces[].ports[].subnetPrefix` | string | GID index 0 的高 64 位，16 个十六进制字符。LID 只在子网内唯一，必须与本字段一起读取。 |
| `interfaces[].ports[].gids[]` | list | 端口的非零 GID 及其 sysfs index，`fe80::` 这类只有前缀的条目不列出。 |
| `interfaces[].ports[].queuePairs[]` | list | 该 Pod 在此端口上进入过 RTR 的 QP。QPN 只在一块 HCA 端口内唯一，因此挂在端口之下。 |
| `interfaces[].ports[].queuePairs[].qpn` | int | QP 号。 |
| `interfaces[].ports[].queuePairs[].pid` | int | 创建该 QP 的进程在宿主机上的 tgid。 |

## 生命周期

- Pod 打开 RDMA 设备或其某个 QP 进入 RTR 后，最多 `endpoint.minInterval`
  （默认 3s）内创建。
- 内容变化时重写，否则最多每 5 分钟重写一次以刷新 `lastUpdateTime`。
  `endpoint.interval`（默认 30s）是兜底发布周期。
- 拥有 QP 的进程退出且 Agent 清理其 BPF map 条目后 `queuePairs` 相应缩减。
- Pod 不再持有设备且没有 QP 时被删除，日志为
  `rdma_endpoint_deleted reason=no_rdma_holder`，并随 Pod 被垃圾回收。
- 默认只发布 hostNetwork Pod 与尚未进入 Agent 索引的 Pod。
  `endpoint.allPods` 发布所有持有者，InfiniBand 环境必须打开。

## 查询

```bash
kubectl get rep -A
kubectl get rep -A -l unifabric.io/node=gpu-node-03
kubectl get rep -n tenant-a vllm-decode-0 -o yaml
```

## RBAC

Agent ClusterRole 对 `rdmaendpoints` 有 `create`、`delete`、`get`、`list`、
`watch` 与 `patch`，对 `rdmaendpoints/status` 有 `patch`。写入使用
Server-Side Apply，field manager 为 `unifabric-agent`。
