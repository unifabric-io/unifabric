# Topology

English version: [topology.md](./topology.md)

`Topology` 是 Controller 生成的集群级只读拓扑视图。它没有 `spec`，只通过
`status` 展示性能域、父子关系、交换机成员和 Node 分组。

## 示例

```yaml
apiVersion: unifabric.io/v1beta1
kind: Topology
metadata:
  name: scaleout
status:
  domains:
    - name: tier2-group1
      tier: 2
      members:
        - spine01
    - name: tier1-group1
      tier: 1
      parent: tier2-group1
      members:
        - leaf01
        - leaf02
  nodes:
    - domainPath:
        - tier2-group1
        - tier1-group1
      nodes:
        - node1
        - node2
```

该示例表示：

- `leaf01` 和 `leaf02` 属于 `tier1-group1`。
- `spine01` 属于它们的上级 `tier2-group1`。
- `node1` 和 `node2` 共享同一条 scale-out 性能域路径。

## 资源信息

| 字段 | 值 |
| --- | --- |
| API group | `unifabric.io` |
| API version | `v1beta1` |
| Kind | `Topology` |
| Scope | Cluster |
| 资源名 | `topologies` |
| 单数名 | `topology` |
| 缩写 | `topo` |
| Status subresource | 是 |

## 固定资源名称

`metadata.name` 只允许以下值：

| 名称 | 说明 |
| --- | --- |
| `scaleout` | scale-out 网络拓扑 |
| `scaleup` | scale-up 网络拓扑 |
| `storage` | storage 网络拓扑 |

Controller 只在对应网络已经产生非空拓扑数据后创建对象。因此，没有任何拓扑输入时，
`kubectl get topo` 可以为空。

## Topology 字段

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `apiVersion` | string | `unifabric.io/v1beta1` |
| `kind` | string | `Topology` |
| `metadata` | `metav1.ObjectMeta` | 标准 Kubernetes metadata；名称必须是固定值之一 |
| `status` | `TopologyStatus` | Controller 维护的只读汇总状态 |

`Topology` 没有 `spec`。

## TopologyStatus

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `domains` | `[]TopologyDomain` | 否 | 拓扑中的性能域，按 `name` 唯一 |
| `nodes` | `[]TopologyNodeGroup` | 否 | 按完整性能域路径分组的 Kubernetes Nodes |

## TopologyDomain

| 字段 | 类型 | 必填 | 校验 | 说明 |
| --- | --- | --- | --- | --- |
| `name` | string | 是 | 长度 `1`～`63` | 性能域名称，也是 Node 拓扑 label 和 Switch domain label 的 value |
| `tier` | int32 | 是 | 最小值 `1` | 层级；`tier: 1` 最靠近 Node，数字越大越靠近上层 |
| `parent` | string | 否 | 无 | 直接上级性能域名称 |
| `members` | `[]string` | 否 | Set，元素唯一 | 属于该性能域的 Switch CR 名称 |

根性能域不包含 `parent`。Node-only 模式没有真实 Switch CR，因此 `members` 可以为空
或不显示。

## TopologyNodeGroup

| 字段 | 类型 | 必填 | 校验 | 说明 |
| --- | --- | --- | --- | --- |
| `nodes` | `[]string` | 是 | 至少 1 项，Set | 共享同一完整路径的 Kubernetes Node 名称 |
| `domainPath` | `[]string` | 是 | 至少 1 项 | 从最高 tier 到 tier 1 排列的性能域名称 |

例如，Node 位于 `tier1-group1`，其上级为 `tier2-group1`，则 `domainPath` 为：

```yaml
domainPath:
  - tier2-group1
  - tier1-group1
```

## 数据来源和维护方式

`TopologyStatusController` 从 Kubernetes 对象的 labels 汇总状态：

- Node 上匹配当前网络的 tier labels 决定性能域路径和 Node 分组。
- Switch 上的 `unifabric.io/domain` label 决定
  `status.domains[*].members`；`Switch.spec.role` 决定它属于哪个 Topology。

`Topology.status` 不直接作为拓扑发现的输入，也不会反向覆盖用户配置。不要手工修改
`status`。

在内置自动发现模式中，删除带有
`unifabric.io/auto-discovered-topology-labels` finalizer 的 `Topology` 是显式重置：
Controller 会先清理该网络对应的受管 Node 和 Switch 拓扑 labels，再允许对象删除。
如果发现输入仍然存在，后续协调会重新生成 labels 和 `Topology`。

## 常用命令

```bash
kubectl get topo
kubectl get topo scaleout -o yaml

kubectl get topo scaleout \
  -o jsonpath='{range .status.domains[*]}{.name}{"\t"}{.tier}{"\t"}{.parent}{"\n"}{end}'
```

拓扑 labels、聚合规则和重置行为请参阅
[Topology CRD 设计](../design/topology-crd.zh.md)。
