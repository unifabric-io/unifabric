# Topology CRD 设计

English version: [topology-crd.md](./topology-crd.md)

## 概要

`Topology` 是 Unifabric 的集群级只读拓扑视图，服务于查询、可视化、诊断和可观测性。
Controller 从 Kubernetes Node labels 汇总性能域、父子关系和 Node 路径，写入三个固定对象
（`scaleout`、`scaleup`、`storage`）的 `status`。该资源没有 `spec`，用户不能写入其 status。
本文记录 API 契约、label 契约和协调（reconciliation）设计。当前字段以
[Topology 参考手册](../reference/topology.md)为准。在 Grafana 中查看拓扑的方法见
[拓扑可视化使用指南](../usage/topology-visualization.md)。

## 动机

Unifabric 从多个来源发现 RDMA 拓扑：内置 LLDP 发现、NVIDIA Topograph 和管理员维护的 labels。
在本设计之前，没有任何一个 API 暴露汇总结果。调度器直接读 Node labels，但运维人员和工具没有
一个集群级对象来展示性能域、层级关系和每个 Node 的路径。每个消费方都要从原始 labels 重建树。

第二个问题是稳定性。已发现的性能域名称必须在设备断连、Controller 重启和 Leader 切换后保持不变。
如果没有显式的锁定规则，一台重连的交换机可能拿到新名称，破坏所有保存了旧名称的消费方。

用户故事：

- 作为调度器运维人员，我想要一个只读资源，列出性能域和 Node 路径，让我看到 Node labels
  表达的树结构。
- 作为平台运维人员，我希望性能域名称在重启和故障后保持稳定，这样看板和放置策略不会抖动。
- 作为集群管理员，我想把 NVIDIA Topograph labels 和手工创建的 `Switch` 资源结合起来，
  用一个 API 覆盖混合发现来源。

### 目标

- 用一个只读 API 表达 scale-out、scale-up 和 storage 拓扑。
- 以 Node labels 作为 `domains` 和 `nodes` 的唯一来源，固定的 Switch
  `unifabric.io/domain` label 只用于补充 `switchMember`。
- 表达性能域层级、每个性能域内的交换机，以及每个 Node 的完整性能域路径。
- 支持内置交换机发现、NVIDIA Topograph 和管理员维护的 labels。
- 已分配的性能域名称在普通协调、设备断连、Controller 重启和 Leader 切换时保持稳定。

### 非目标

- `Topology` 不定义资源匹配条件。
- `Topology` status 不会反向创建或修改 Node、Switch、FabricNode 的 labels。
  删除 `Topology` 触发的显式重置是唯一例外。
- `Topology` 不保存 LLDP、端口、链路、带宽、时延或路由等原始数据。
- 本设计不定义调度器插件、工作负载放置或拓扑约束。
- 用户不能直接维护 `status.domains` 或 `status.nodes`。

## 方案

设计把工作拆分为两个控制器和一个 label 契约。

`AutoDiscoveredTopologyLabelController` 监听 `Switch` 和 `FabricNode` 资源，执行
`pkg/topology.DiscoverTopology()`，写两类 labels。每个 Node 的路径每层各写一个 tier label，
由可配置的 Go 模板渲染，例如 `scale-out.unifabric.io/tier-{{ .Tier }}`。每个 Switch 写一个
固定的 `unifabric.io/domain` label。

`TopologyStatusController` 监听这些 labels，重建性能域树，并写固定 `Topology` 对象的 status。
它是 `Topology` status 的唯一写入方。NVIDIA Topograph 和管理员同样写 Node labels，该控制器
用同一方式读取。因此不存在中间的 labels 到 CRD 的控制器，也不存在从 status 回写 labels 的循环。

数据流如下：

```text
Switch + FabricNode -> label 控制器 -> Node tier labels + Switch domain label
NVIDIA Topograph  ----------------------^
管理员            ----------------------^
Node + Switch labels -> status 控制器 -> Topology status -> UI、kubectl、调度器
```

## 详细设计

### API 草案

`Topology` 是集群级资源，没有 `spec`，核心 status 字段如下：

| 字段 | 含义 |
| --- | --- |
| `status.domains[*].name` | 性能域名称，必须是合法的 Kubernetes label value。 |
| `status.domains[*].tier` | 从 label key 解析出的层级。Tier 1 最靠近 Node。 |
| `status.domains[*].parent` | 可选的直接父域。根域没有该字段。 |
| `status.domains[*].switchMember` | 该域内 `Switch` 资源的名称列表。 |
| `status.nodes[*].name` | 稳定的分组标识，形如 `node-group<N>`。 |
| `status.nodes[*].nodes` | 共享同一路径的 Node 名称，按字典序排序。 |
| `status.nodes[*].switchDomainPath` | 性能域名称，从最高层级排到 tier 1。 |

```yaml
apiVersion: unifabric.io/v1beta1
kind: Topology
metadata:
  name: scaleout
status:
  domains:
    - name: tier2-group1
      tier: 2
      switchMember:
        - spine1
    - name: tier1-group1
      tier: 1
      parent: tier2-group1
      switchMember:
        - leaf1
        - leaf2
  nodes:
    - name: node-group1
      nodes: [node1, node2]
      switchDomainPath:
        - tier2-group1
        - tier1-group1
```

该结构不限制深度。两层、三层和更深的网络使用同一组字段。内置发现用 `tier${N}-group${M}`
命名性能域，其中 `M` 在同一拓扑和同一层级内对分组计数。

### Label 契约

Helm values 中的 `topoDiscovery` 为每个拓扑选择一个写入方和一个 label key 模板。模板是
Go `text/template`，必须且只能包含一个 `{{ .Tier }}` 动作，其余为固定文本。启动校验遍历
模板解析树来强制这一点，并渲染示例层级以确认输出是合法的 label key。Controller 从同一棵
解析树派生锚定匹配器，用于从 label key 反向解析层级。

同一个 Node 上的层级从 tier 1 连续编号。不同 Node 的最高层级可以不同。Switch 只携带固定的
`unifabric.io/domain` label，通过 `spec.role` 选择所属网络。Controller 用 label value 匹配
由 Node 推导出的性能域来推断 Switch 的层级。

### 名称稳定与锁定

一旦 Node 或 Switch 携带了受管 label，该 key 和 value 就是锁定的分配结果。普通协调只补齐
缺失的 labels，不覆盖、不迁移、不重命名、不删除已有的 labels。设备下电、LLDP 观测缺失和
输入删除都不会清理 labels。Controller 不保存命名计数器。每次协调扫描现有 labels，找出每个
层级的最大分组序号，从 `M+1` 开始分配新名称。

删除内置发现所拥有的 `Topology` 是唯一的完整重置。label 控制器持有的 finalizer 清空所有
受管 labels 后放行删除。发现流程重新计算，结果非空时重建固定对象，分组重新从 `group1` 编号。

### 单写入方规则

每个拓扑 label key 同一时刻只有一个写入方。内置发现拥有它生成的 labels。外部写入方（例如独立部署的
NVIDIA Topograph）只在 `mode=external` 的拓扑上拥有配置的 keys。只有当没有自动写入方时，管理员才接管一个
拓扑。与其他 field manager 的冲突会被上报，不会强制覆盖。

### 控制器交互与升级兼容性

label 控制器用输入指纹作为更新谓词。`Switch` 指纹覆盖 `spec.role`、`status.hostname`、
`status.healthy` 和归一化后的 LLDP 邻居。`FabricNode` 指纹覆盖 `status.nodeRole` 和拓扑
计算消费的网卡字段。仅 label 变化的更新不会重新触发发现。labels 通过 metadata-only Patch
写入，并带 resource-version 前置条件。

status 控制器每次协调重建完整快照，原子替换 status。输入冲突时保留上一次有效的 `domains`
和 `nodes`，消费方不会看到部分结果。输入相同时不产生 API 写操作。

固定 `Topology` 对象延迟创建，只在出现非空有效结果后创建。删除所有相关 labels 是一个合法的
空结果：status 被清空，但已创建的对象保留。这保证升级安全。新版本 Controller 读取相同的
labels，无需迁移步骤即可复现相同的 status。

## 风险与缓解

- label 写入可能再次入队发现控制器，放大 API 负载。缓解：基于指纹的更新谓词和基于 diff 的写入。
- 部分 Patch 失败可能让一个分组只命名了一半。缓解：幂等协调重新计算最大序号并复用已锁定的名称。
- 同一发现分组内锁定名称冲突可能破坏树结构。缓解：通过 Event、日志和指标上报冲突，保留所有
  现有 labels，不做任何自动写入。
- 已删除分组的名称会被复用，可能让保存旧名称的消费方困惑。缓解：将该行为写入文档。只有当所有
  携带该名称的对象都被实际删除后才会复用。

## 备选方案

- 带资源匹配条件的可写 `spec`。否决，因为一个拓扑有两个写入方会产生冲突，而且所有消费方需要的
  是计算结果而不是匹配规则。只读 status 模型保证唯一权威写入方。
- 在 Switch 上使用按层级区分的 label keys，与 Node tier labels 对称。否决，因为一个 Switch
  恰好处于一个层级，且层级已可从匹配到的 Node 性能域推导。一个固定的 `unifabric.io/domain`
  key 更简单，且与角色无关。
- 用 ConfigMap 或 status 持久化命名计数器。否决，因为部分失败后计数器会和实际 labels 漂移。
  从实时 labels 重算最大序号始终与已写入状态一致。
- 一个控制器同时负责发现和 status 汇总。否决，因为 NVIDIA Topograph 和管理员在没有发现流程时
  也会写 labels。独立的 status 控制器统一消费所有 label 来源，并避免写循环。
- 启动时立即创建三个固定 `Topology` 对象。否决，因为空对象会误导消费方，并使"删除即重置"
  语义复杂化。延迟创建把对象的存在与真实拓扑数据绑定。

## 实现历史

- 2026-07-13：Topology CRD 设计文档提交。
- 2026-07-17：Topology status 汇总与自动 label 发现实现。
- 2026-08-11：`Topology` status 字段重命名，增加邻居链路与计数跟踪。本文的早期版本使用重命名前
  的字段名。
- 2026-08-24：在 Topology API 之上增加 Grafana 拓扑可视化。
