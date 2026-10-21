# Switch

English version: [switch.md](./switch.md)

## 概述

`Switch` 声明一台物理交换机，并保存 Controller 观测到的 switch-agent 连接状态和
LLDP 邻居。它在拓扑发现中的角色见[架构概览](../concepts/architecture.zh.md)。

## 资源信息

| 项 | 值 |
| --- | --- |
| API group | `unifabric.io` |
| API version | `v1beta1` |
| Kind | `Switch` |
| Scope | Cluster |
| 复数名 | `switches` |
| 单数名 | `switch` |
| 缩写 | `sw` |
| Status subresource | 是 |
| Printer columns | `MgmtIP`、`Role`、`Healthy`、`Neighbors` |

## 示例 YAML

用户编写 `metadata` 和 `spec`，Controller 写入 `status`。

```yaml
apiVersion: unifabric.io/v1beta1
kind: Switch
metadata:
  name: leaf01
  labels:
    # 交换机所属性能域。由拓扑发现写入，或由管理员手工设置。
    unifabric.io/domain: tier1-group1
  annotations:
    # 仅半自动模式使用：直连交换机名称，JSON 字符串数组。
    # 禁止填写 Node 名称。
    unifabric.io/switch-neighbors: '["spine01"]'
spec:
  # switch-agent 订阅的连接地址。省略则只声明交换机，不建立订阅。
  mgmtIP: 10.0.0.1
  # 交换机所属网络域。
  role: ScaleOut
  # 覆盖该交换机的 switch-agent gRPC 端口。
  grpcPort: 8090
status:
  conditions:
    # Connected 表示 gRPC 流的状态。
    - lastTransitionTime: "2026-07-24T01:00:00Z"
      message: controller is receiving LLDP snapshots from switch agent
      reason: StreamReady
      status: "True"
      type: Connected
    # Ready 表示已存 LLDP 数据的新鲜度。
    - lastTransitionTime: "2026-07-24T01:00:00Z"
      message: accepted LLDP snapshot generation 7
      reason: SnapshotAccepted
      status: "True"
      type: Ready
  # 最新接受快照上报的 hostname。
  hostname: leaf01
  healthy: true
  # 按远端系统去重后的 LLDP 邻居。
  lldpNeighborCount: 2
  lldpNeighbors:
    - remoteSystemType: KubernetesNode
      remoteSystemName: node1
      linkCount: 1
    - remoteSystemType: Switch
      remoteSystemName: spine01
      linkCount: 2
```

## 字段定义

### Metadata

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `metadata.name` | string | 是 | 无 | 交换机名称。拓扑发现用它匹配 LLDP 的系统名。 |
| `metadata.labels["unifabric.io/domain"]` | label | 否 | 无 | 交换机所属性能域，例如 `tier1-group1`。由发现写入；手工设置时值必须是已存在的域。 |
| `metadata.annotations["unifabric.io/switch-neighbors"]` | annotation | 否 | 无 | 半自动模式下的直连交换机名称，JSON 字符串数组。禁止填写 Kubernetes Node 名称。 |

同一 `role` 的交换机按 `unifabric.io/switch-neighbors` key 是否存在选择发现模式。
所有交换机都有该 key 时进入半自动模式；任一交换机缺少该 key 时进入全自动模式，并
忽略所有该 annotation。空值和 `[]` 都视为 key 存在。

### Spec

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `mgmtIP` | string | 否 | 无 | switch-agent 订阅的连接地址。省略时 Controller 不建立订阅，交换机仍可通过 metadata 提供拓扑成员和手工邻接。 |
| `role` | string | 否 | `ScaleOut` | 交换机所属网络域。可选值为 `ScaleOut`、`ScaleUp`、`Storage`。 |
| `grpcPort` | integer | 否 | 无 | 覆盖该交换机的 switch-agent gRPC 端口，范围 `1`～`65535`。省略时使用 Controller 配置的 `defaultGrpcPort`（chart 默认 `8090`）。 |

### Status

`status` 由 Controller 维护。不要手工编辑。删除 `spec.mgmtIP` 后，Controller 停止
订阅并清空整个 `status`。

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `hostname` | string | 否 | 无 | 最新接受快照上报的系统名。 |
| `healthy` | boolean | 否 | 无 | 接受快照后为 `true`，订阅失败后为 `false`。 |
| `conditions` | `[]metav1.Condition` | 否 | 无 | 流和数据新鲜度的 condition，按 `type` 唯一。见 [Conditions](#conditions)。 |
| `lldpNeighborCount` | integer | 否 | 无 | 已保存的 LLDP 邻居条目数量。 |
| `lldpNeighbors` | `[]SwitchNeighbor` | 否 | 无 | 该交换机观测到的唯一远端邻居。见 [SwitchNeighbor](#switchneighbor)。 |

#### SwitchNeighbor

`lldpNeighbors` 的元素类型。

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `remoteSystemType` | string | 否 | 无 | 远端系统是 Kubernetes Node 还是另一台交换机。可选值为 `KubernetesNode`、`Switch`。 |
| `remoteSystemName` | string | 是 | 无 | 该 LLDP 条目对应的远端系统名称。 |
| `linkCount` | integer | 否 | 无 | 到该远端系统的物理链路（本地端口）数量。同一远端系统的多条链路合并为这一条记录。 |

#### Conditions

Controller 设置两种 condition 类型。下表穷举每个 status、reason 及其触发条件。

`Connected`：

| Status | Reason | 触发条件 |
| --- | --- | --- |
| `True` | `StreamReady` | Controller 正在接收 switch-agent 的 LLDP 快照。 |
| `False` | `DialFailed` | gRPC 连接或流失败，且不属于下面两类。message 包含错误。 |
| `False` | `AuthenticationFailed` | 订阅失败且 gRPC code 为 `FailedPrecondition`、`Unauthenticated` 或 `PermissionDenied`，或错误与 TLS、证书相关。 |
| `False` | `SnapshotRejected` | switch-agent 以 gRPC `InvalidArgument` 拒绝了请求。 |

`Ready`：

| Status | Reason | 触发条件 |
| --- | --- | --- |
| `True` | `SnapshotAccepted` | Controller 接受并保存了最新 LLDP 快照。message 包含快照 generation。 |
| `False` | `DataStale` | 订阅失败，已存 LLDP 数据不再新鲜。与 `Connected=False` 同时设置。 |

两次接受的快照之间邻居集合变化时，Controller 还会发出 reason 为 `NeighborsChanged`
的 `Normal` Kubernetes Event。邻居以 `remoteSystemType` 加 `remoteSystemName` 标识，
仅 `linkCount` 变化不触发该事件。

switch-agent 的部署和接入方法见
[switch-agent 部署指南](../usage/switch-agent-deployment.zh.md)。
