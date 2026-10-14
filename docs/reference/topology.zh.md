# Topology

English version: [topology.md](./topology.md)

## 概述

`Topology` 是集群级的只读拓扑视图，由 Controller 根据 Node 和 Switch 的 label 汇总
生成。它在拓扑发现中的角色见[架构概览](../concepts/architecture.zh.md)。

## 资源信息

| 项 | 值 |
| --- | --- |
| API group | `unifabric.io` |
| API version | `v1beta1` |
| Kind | `Topology` |
| Scope | Cluster |
| 复数名 | `topologies` |
| 单数名 | `topology` |
| 缩写 | `topo` |
| Status subresource | 是 |

## 示例 YAML

Controller 生成形如下例的对象。用户不编写它。

```yaml
apiVersion: unifabric.io/v1beta1
kind: Topology
metadata:
  # 名称必须是 scaleout、scaleup 或 storage。
  name: scaleout
  # 内置发现模式下，该 finalizer 使删除成为对
  # 托管拓扑 label 的显式重置。
  finalizers:
    - unifabric.io/auto-discovered-topology-labels
status:
  # 由 label 汇总的性能域，按 name 唯一。
  domains:
    # 根域没有 parent。
    - name: tier2-group1
      tier: 2
      switchMember:
        - spine01
    - name: tier1-group1
      tier: 1
      parent: tier2-group1
      switchMember:
        - leaf01
        - leaf02
  # 按完整域路径分组的 Kubernetes Node。
  nodes:
    - name: node-group1
      nodes:
        - node1
        - node2
      # 从最高 tier 排到最靠近 Node 的 tier。
      switchDomainPath:
        - tier2-group1
        - tier1-group1
```

该示例表示：`leaf01` 和 `leaf02` 属于 `tier1-group1`；`spine01` 属于其父域
`tier2-group1`；`node1` 和 `node2` 共享同一条 scale-out 域路径。

## 字段定义

### Metadata

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `metadata.name` | string | 是 | 必须是 `scaleout`、`scaleup` 或 `storage`。CEL 校验规则拒绝其他名称。 |
| `metadata.finalizers` | `[]string` | 否 | 内置发现模式下 Controller 会添加 `unifabric.io/auto-discovered-topology-labels`。此时删除对象会先清除该网络下托管的 Node 和 Switch 拓扑 label。 |

三个固定名称对应三个网络：

| 名称 | 网络 |
| --- | --- |
| `scaleout` | Scale-out 网络拓扑 |
| `scaleup` | Scale-up 网络拓扑 |
| `storage` | 存储网络拓扑 |

### Spec

`Topology` 没有 `spec`。Node 和 Switch 的 label 是它唯一的事实来源。

### Status

`status` 由 Controller 维护。不要手工编辑。所有字段默认均为空，因此下表省略默认值列。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `domains` | `[]TopologyDomain` | 否 | 由 label 汇总的拓扑域森林，按 `name` 唯一。见 [TopologyDomain](#topologydomain)。 |
| `nodes` | `[]TopologyNodeGroup` | 否 | 按完整域路径分组的 Kubernetes Node。见 [TopologyNodeGroup](#topologynodegroup)。 |

#### TopologyDomain

`domains` 的元素类型。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `name` | string | 是 | 对应 Kubernetes 拓扑 label 中存储的值，长度 1～63。 |
| `tier` | integer | 是 | 域的层级，最小为 `1`。Tier 1 是最靠近 Kubernetes Node 的域。 |
| `parent` | string | 否 | 直接包含本域的性能域名称。根域没有 `parent`。 |
| `switchMember` | `[]string`（set） | 否 | 携带本域的 `Switch` 资源名称。域内没有 Switch CR 时为空，例如 Node-only 模式。 |

#### TopologyNodeGroup

`nodes` 的元素类型。

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `name` | string | 是 | 该分组的稳定标识，形如 `node-group<N>`。已有分组在多次调和间保持名称不变，新分组取最小的未用编号。 |
| `nodes` | `[]string`（set） | 是 | 域路径相同的 Kubernetes Node，至少一项。 |
| `switchDomainPath` | `[]string` | 是 | 域名称列表，从最高 tier 排到最靠近 Node 的 tier，至少一项。 |

该资源不定义 condition。

只有某网络存在非空拓扑数据时，Controller 才创建对应 `Topology`，因此没有拓扑输入的
集群上 `kubectl get topo` 可能为空。

拓扑的查看和导出方法见[拓扑可视化指南](../usage/topology-visualization.zh.md)。
