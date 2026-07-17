# FabricNode

English version: [fabricnode.md](./fabricnode.md)

`FabricNode` 记录 Kubernetes Node 上观测到的 RDMA 网卡、LLDP 邻居和 RDMA Pod。
节点 Agent 创建并更新该资源；用户通常只读取它，不应手工维护其 `status`。

## 示例

以下对象由 Agent 生成：

```yaml
apiVersion: unifabric.io/v1beta1
kind: FabricNode
metadata:
  name: node1
spec: {}
status:
  conditions:
    - lastTransitionTime: "2026-07-24T01:00:00Z"
      message: All FabricNode conditions are ready
      reason: Ready
      status: "True"
      type: Ready
    - lastTransitionTime: "2026-07-24T01:00:00Z"
      message: LLDP neighbors are ready
      reason: LLDPNeighborsReady
      status: "True"
      type: LLDPNeighborsReady
  nodeIP: 10.0.0.11
  nodeRole: GPU
  totalNics: 1
  scaleOutNics:
    - name: eth1
      rdmaDeviceName: mlx5_0
      rdma: true
      ipv4: 192.168.1.11/24
      ipv6: ""
      state: up
      lldpNeighbor:
        hostname: leaf01
        mgmtIP: 10.0.0.1
        mac: 00:11:22:33:44:55
        port: Ethernet1
        description: leaf switch
```

## 资源信息

| 字段 | 值 |
| --- | --- |
| API group | `unifabric.io` |
| API version | `v1beta1` |
| Kind | `FabricNode` |
| Scope | Cluster |
| 资源名 | `fabricnodes` |
| 单数名 | `fabricnode` |
| 缩写 | `fn` |
| Status subresource | 是 |

资源名称与对应的 Kubernetes Node 名称一致，并以该 Node 作为 owner。Node 删除后，
对应的 `FabricNode` 也会被清理。

## FabricNode 字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `apiVersion` | string | `unifabric.io/v1beta1` |
| `kind` | string | `FabricNode` |
| `metadata` | `metav1.ObjectMeta` | 标准 Kubernetes metadata |
| `spec` | `FabricNodeSpec` | 期望状态；当前为空 |
| `status` | `FabricNodeStatus` | Agent 维护的观测状态，只读 |

## FabricNodeSpec

`FabricNodeSpec` 当前没有可配置字段。

## FabricNodeStatus

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `conditions` | `[]metav1.Condition` | 否 | 当前资源状态，按 `type` 唯一 |
| `totalNics` | integer | 是 | 归类到 scale-out 或 storage 网络的 RDMA 网卡总数 |
| `rdmaPods` | `[]RdmaPod` | 否 | 当前节点上使用 RDMA 的 Pod |
| `scaleOutNics` | `[]NicInfo` | 否 | scale-out 网络的 RDMA 网卡 |
| `storageNics` | `[]NicInfo` | 否 | storage 网络的 RDMA 网卡 |
| `nodeRole` | string | 否 | 节点角色：`GPU` 或 `Storage` |
| `nodeIP` | string | 否 | 节点 IP |

已知的 condition 类型：

| Type | 说明 |
| --- | --- |
| `Ready` | 汇总其他 condition；所有 condition 就绪时为 `True` |
| `LLDPNeighborsReady` | 需要 LLDP 的接口是否都已发现有效邻居 |

`LLDPNeighborsReady=False` 的常见 reason 是 `LLDPNeighborMissing`；
采集失败时 reason 为 `DiscoveryFailed`。

## NicInfo

`scaleOutNics` 和 `storageNics` 的元素类型。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `name` | string | 是 | Linux 网络接口名，例如 `eth1` 或 `ib0` |
| `rdmaDeviceName` | string | 是 | RDMA 设备名，例如 `mlx5_0` |
| `rdma` | boolean | 是 | 是否支持 RDMA |
| `ipv4` | string | 是 | CIDR 格式的 IPv4；没有地址时为空字符串 |
| `ipv6` | string | 是 | CIDR 格式的 IPv6；没有地址时为空字符串 |
| `state` | string | 是 | 链路状态，例如 `up`、`down` 或 `unknown` |
| `lldpNeighbor` | `LLDPNeighbor` | 否 | 该接口发现的 LLDP 邻居 |

拓扑发现只使用符合当前网络角色、状态为 `up`，且 LLDP 邻居包含 `hostname` 和
`port` 的网卡。

## LLDPNeighbor

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `hostname` | string | 是 | 邻居设备 hostname |
| `mgmtIP` | string | 是 | 邻居管理 IP |
| `mac` | string | 是 | 邻居 MAC 地址 |
| `port` | string | 是 | 邻居端口，例如 `Ethernet32` |
| `description` | string | 是 | 邻居设备描述 |

这些字段在 `lldpNeighbor` 对象存在时由 schema 要求；采集不到的值可以是空字符串。

## RdmaPod

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `namespace` | string | 是 | Pod namespace |
| `name` | string | 是 | Pod 名称 |
| `containerList` | `[]string` | 否 | 使用 RDMA 的容器 ID |
| `hostRDMA` | boolean | 否 | Pod 是否使用 host RDMA 模式 |
| `topOwner` | `OwnerRef` | 否 | Pod 顶层工作负载 owner |

`OwnerRef` 可以包含 `apiVersion`、`kind`、`namespace` 和 `name`。

## 常用命令

```bash
kubectl get fn
kubectl get fn node1 -o yaml

kubectl get fn node1 \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{"\t"}{.reason}{"\n"}{end}'

kubectl get fn node1 \
  -o jsonpath='{range .status.scaleOutNics[*]}{.name}{"\t"}{.state}{"\t"}{.lldpNeighbor.hostname}{"\n"}{end}'
```

拓扑计算如何使用这些字段，请参阅
[Scale-Out 网络拓扑发现设计](../design/scaleout-topology.zh.md)。
