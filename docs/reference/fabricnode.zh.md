# FabricNode

English version: [fabricnode.md](./fabricnode.md)

## 概述

`FabricNode` 记录一个 Kubernetes Node 上观测到的 RDMA 网卡、LLDP 邻居和 RDMA Pod。
该资源由 Agent 创建和更新，用户只读取。它在拓扑发现中的角色见
[架构概览](../concepts/architecture.zh.md)。

## 资源信息

| 项 | 值 |
| --- | --- |
| API group | `unifabric.io` |
| API version | `v1beta1` |
| Kind | `FabricNode` |
| Scope | Cluster |
| 复数名 | `fabricnodes` |
| 单数名 | `fabricnode` |
| 缩写 | `fn` |
| Status subresource | 是 |
| Printer columns | `TotalNics`、`Ready`、`Role`、`NodeIP`、`Topologies` |

## 示例 YAML

Agent 生成形如下例的对象。用户不编写 `status`。

```yaml
apiVersion: unifabric.io/v1beta1
kind: FabricNode
metadata:
  # 名称与 Kubernetes Node 名称一致。Node 是属主，
  # 删除 Node 会同时删除 FabricNode。
  name: node1
# v1beta1 中 spec 没有字段。
spec: {}
status:
  conditions:
    # Ready 汇总其他全部 condition。
    - lastTransitionTime: "2026-07-24T01:00:00Z"
      message: All FabricNode conditions are ready
      reason: Ready
      status: "True"
      type: Ready
    # LLDPNeighborsReady 表示每个被选中的 RDMA 接口
    # 是否都有有效的 LLDP 邻居。
    - lastTransitionTime: "2026-07-24T01:00:00Z"
      message: All selected RDMA interfaces have LLDP neighbors
      reason: LLDPNeighborsReady
      status: "True"
      type: LLDPNeighborsReady
  # Agent 上报的 Node IP 和角色。
  nodeIP: 10.0.0.11
  nodeRole: GPU
  # 归入 scale-out 或 storage 网络的 RDMA 网卡总数。
  totalNics: 2
  # scale-out 网络中的 RDMA 网卡。
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
  # storage 网络中的 RDMA 网卡。
  storageNics:
    - name: eth5
      rdmaDeviceName: mlx5_4
      rdma: true
      ipv4: 192.168.5.11/24
      ipv6: ""
      state: up
      lldpNeighbor:
        hostname: storagesw
        mgmtIP: 10.0.5.1
        mac: 00:11:22:33:44:66
        port: Ethernet8
        description: storage switch
  # 本 Node 上使用 RDMA 设备的 Pod。
  rdmaPods:
    - namespace: training
      name: llm-job-worker-0
      containerList:
        - containerd://3f9c2b
      hostRDMA: false
      topOwner:
        apiVersion: batch/v1
        kind: Job
        namespace: training
        name: llm-job
  # 本 Node 当前携带其 domain label 的固定 Topology 名称。
  topologies:
    - scaleout
    - storage
```

## 字段定义

### Metadata

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `metadata.name` | string | 是 | 无 | 与 Kubernetes Node 名称一致。Node 是属主，删除 Node 后 `FabricNode` 随之删除。 |

该资源不定义必需的 label 或 annotation。

### Spec

`v1beta1` 中 `FabricNodeSpec` 没有字段。写 `spec: {}`。

### Status

`status` 由 Agent 维护。不要手工编辑。

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `conditions` | `[]metav1.Condition` | 否 | 无 | 当前 condition，按 `type` 唯一。见 [Conditions](#conditions)。 |
| `totalNics` | integer | 是 | 无 | 归入 scale-out 或 storage 网络的 RDMA 网卡总数。 |
| `rdmaPods` | `[]RdmaPod` | 否 | 无 | 当前运行在该 Node 上的 RDMA Pod。见 [RdmaPod](#rdmapod)。 |
| `scaleOutNics` | `[]NicInfo` | 否 | 无 | scale-out 网络中的 RDMA 网卡。见 [NicInfo](#nicinfo)。 |
| `storageNics` | `[]NicInfo` | 否 | 无 | storage 网络中的 RDMA 网卡。见 [NicInfo](#nicinfo)。 |
| `nodeRole` | string | 否 | 无 | Agent 上报的 Node 角色。可选值为 `GPU` 和 `Storage`。 |
| `nodeIP` | string | 否 | 无 | Node IP 地址。 |
| `topologies` | `[]string`（set） | 否 | 无 | 本 Node 当前携带其 domain label 的固定 `Topology` 名称，例如 `["scaleout", "storage"]`。来源于 Node 自身的 label，不由 Agent 上报。 |

#### NicInfo

`scaleOutNics` 和 `storageNics` 的元素类型。

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `name` | string | 是 | 无 | Linux 接口名，例如 `eth0`、`ib0`。 |
| `rdmaDeviceName` | string | 是 | 无 | RDMA 设备名，例如 `mlx5_0`。 |
| `rdma` | boolean | 是 | 无 | 该接口是否支持 RDMA。 |
| `ipv4` | string | 是 | 无 | CIDR 格式的 IPv4 地址，例如 `172.17.0.1/32`。无地址时为空字符串。 |
| `ipv6` | string | 是 | 无 | CIDR 格式的 IPv6 地址，例如 `fe80::1/64`。无地址时为空字符串。 |
| `state` | string | 是 | 无 | 接口当前状态，例如 `up`、`down`、`unknown`。 |
| `lldpNeighbor` | `LLDPNeighbor` | 否 | 无 | 与该网卡相连的邻居设备。见 [LLDPNeighbor](#lldpneighbor)。 |

#### LLDPNeighbor

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `hostname` | string | 是 | 无 | 邻居设备的 hostname。 |
| `mgmtIP` | string | 是 | 无 | 邻居设备的管理 IP 地址。 |
| `mac` | string | 是 | 无 | 邻居设备的 MAC 地址。 |
| `port` | string | 是 | 无 | 邻居设备上的端口名，例如 `Ethernet32`。 |
| `description` | string | 是 | 无 | 邻居设备的描述。 |

当存在 `lldpNeighbor` 对象时，schema 要求这些字段都出现。Agent 采集不到的值为空
字符串。

#### RdmaPod

`rdmaPods` 的元素类型。

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `namespace` | string | 是 | 无 | RDMA Pod 的命名空间。 |
| `name` | string | 是 | 无 | RDMA Pod 的名称。 |
| `containerList` | `[]string` | 否 | 无 | RDMA Pod 中容器的 ID 列表。 |
| `hostRDMA` | boolean | 否 | `false` | Pod 是否运行在 host RDMA 模式。该模式通常挂载 `/dev/infiniband`、开启 privileged 并设置 `hostNetwork: true`。 |
| `topOwner` | `OwnerRef` | 否 | 无 | RDMA Pod 的顶层属主。见 [OwnerRef](#ownerref)。 |

#### OwnerRef

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `apiVersion` | string | 否 | 无 | 属主的 API version。 |
| `kind` | string | 否 | 无 | 属主的 Kind，例如 `Job`。 |
| `namespace` | string | 否 | 无 | 属主的命名空间。 |
| `name` | string | 否 | 无 | 属主的名称。 |

#### Conditions

Agent 设置两种 condition 类型。下表穷举每个 status、reason 及其触发条件。

`LLDPNeighborsReady`：

| Status | Reason | 触发条件 |
| --- | --- | --- |
| `True` | `LLDPNeighborsReady` | 每个被选中的 RDMA 接口都有 LLDP 邻居，或没有接口需要 LLDP。 |
| `False` | `LLDPNeighborMissing` | 至少一个被选中的接口为 `up` 但没有 LLDP 邻居 hostname。message 列出这些接口。 |
| `Unknown` | `DiscoveryFailed` | Agent 列举网卡失败或执行 LLDP 发现失败。message 包含错误。 |

`Ready`：

| Status | Reason | 触发条件 |
| --- | --- | --- |
| `True` | `Ready` | 其他 condition 全部为 `True`。 |
| `False` | `ConditionNotReady` | 至少一个其他 condition 为 `False`。message 指出该 condition。 |
| `Unknown` | `ConditionUnknown` | 至少一个其他 condition 为 `Unknown`，且没有 `False`。 |

`LLDPNeighborsReady` 状态变化时 Agent 还会发出 Kubernetes Event。发现失败的 Event
reason 是 `LLDPDiscoveryFailed`，而不是 `DiscoveryFailed`。

这些字段的使用方法见 [SONiC RoCE 指南](../getting-started-sonic-roce.zh.md)。
