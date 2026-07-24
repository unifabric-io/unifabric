# Switch

English version: [switch.md](./switch.md)

`Switch` 表示一台物理交换机。用户声明 `metadata` 和 `spec`；配置 switch-agent
连接后，Controller 将连接状态和 LLDP 快照写入 `status`。switch-agent 不直接访问
Kubernetes API。

## 示例

### 全自动 LLDP 发现

该模式下 Switch CR 对应真实交换机，并配置 switch-agent 地址：

```yaml
apiVersion: unifabric.io/v1beta1
kind: Switch
metadata:
  name: leaf01
spec:
  mgmtIP: 10.0.0.1
  role: ScaleOut
  grpcPort: 8090
```

### 半自动发现

Node LLDP 已发现 `leaf01` 和 `leaf02` 时，只需为更上层的交换机创建 CR：

```yaml
apiVersion: unifabric.io/v1beta1
kind: Switch
metadata:
  name: spine01
  annotations:
    unifabric.io/neighbors: '["leaf01", "leaf02"]'
spec:
  role: ScaleOut
```

## 资源信息

| 字段 | 值 |
| --- | --- |
| API group | `unifabric.io` |
| API version | `v1beta1` |
| Kind | `Switch` |
| Scope | Cluster |
| 资源名 | `switches` |
| 单数名 | `switch` |
| 缩写 | `sw` |
| Status subresource | 是 |

## Switch 字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `apiVersion` | string | `unifabric.io/v1beta1` |
| `kind` | string | `Switch` |
| `metadata` | `metav1.ObjectMeta` | 标准 Kubernetes metadata；支持 Unifabric 的 label 和 annotation |
| `spec` | `SwitchSpec` | 用户声明的交换机连接和网络角色 |
| `status` | `SwitchStatus` | Controller 维护的观测状态，只读 |

## SwitchSpec

| 字段 | 类型 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- | --- |
| `mgmtIP` | string | 否 | 无 | Controller 连接 switch-agent 的管理地址；不填写时不建立订阅 |
| `role` | string | 否 | `ScaleOut` | 网络角色：`ScaleOut`、`ScaleUp` 或 `Storage` |
| `grpcPort` | int32 | 否 | 全局端口 | 覆盖该交换机的 switch-agent gRPC 端口，范围 `1`～`65535` |

未设置 `grpcPort` 时使用 `switchSubscription.defaultGrpcPort`，其 chart 默认值为
`8090`。

不设置 `mgmtIP` 的 Switch 仍可通过 metadata 声明半自动邻接关系，或补充
`Topology.status.domains[*].members`，但 Controller 不会连接 switch-agent。

## SwitchStatus

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `hostname` | string | 否 | switch-agent 最新快照上报的 hostname |
| `healthy` | boolean | 否 | 当前订阅和 LLDP 数据是否健康 |
| `conditions` | `[]metav1.Condition` | 否 | 连接和数据新鲜度状态，按 `type` 唯一 |
| `lldpNeighborCount` | int32 | 否 | 已保存的唯一 LLDP 邻居数量 |
| `lldpNeighbors` | `[]SwitchNeighbor` | 否 | 最新接受的 LLDP 邻居 |

已知的 condition 类型和 reason：

| 类别 | 值 |
| --- | --- |
| Type | `Connected`、`Ready` |
| 正常 reason | `StreamReady`、`SnapshotAccepted` |
| 异常 reason | `DialFailed`、`AuthenticationFailed`、`SnapshotRejected`、`DataStale` |

## SwitchNeighbor

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `remoteSystemType` | string | 否 | 邻居类型：`KubernetesNode` 或 `Switch` |
| `remoteSystemName` | string | 是 | 邻居的 Node 名称或交换机名称 |

## Labels 和 annotations

| Key | 类型 | 说明 |
| --- | --- | --- |
| `unifabric.io/domain` | Label | Switch 所属性能域，例如 `tier1-group1` |
| `unifabric.io/neighbors` | Annotation | 半自动模式下的直连交换机名称，值为 JSON 字符串数组 |

内置拓扑发现可以写入 `unifabric.io/domain`。管理员也可手工设置该 label 来补充
`Topology.status.domains[*].members`，但 value 必须匹配一个已有性能域。

`unifabric.io/neighbors` 的值示例：

```yaml
metadata:
  annotations:
    unifabric.io/neighbors: '["leaf01", "leaf02"]'
```

同一 `role` 的 Switch 按 annotation **key 是否存在**选择发现模式：

- 所有 Switch 都存在该 key：进入半自动模式，使用 Node LLDP 生成虚拟 leaf，
  使用 annotation 连接更上层交换机。
- 任一 Switch 缺少该 key：进入全自动模式，使用交换机上报的 LLDP，并忽略所有
  `unifabric.io/neighbors` annotation。
- 空值和 `[]` 也表示 key 存在。

## 常用命令

```bash
kubectl get sw
kubectl get sw leaf01 -o yaml

kubectl get sw leaf01 \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{"\t"}{.reason}{"\n"}{end}'

kubectl get sw leaf01 \
  -o jsonpath='{range .status.lldpNeighbors[*]}{.remoteSystemType}{"\t"}{.remoteSystemName}{"\n"}{end}'
```

发现模式和名称匹配规则请参阅
[Scale-Out 网络拓扑发现设计](../design/scaleout-topology.zh.md)。
