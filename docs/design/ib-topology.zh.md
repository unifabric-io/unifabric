# InfiniBand 拓扑发现（unifabric-ib）

## 概述

本文档描述一种内置的 InfiniBand 网络拓扑发现模式，名为 `unifabric-ib`，可用于 `topoDiscovery.scaleOut.mode` 和 `topoDiscovery.storage.mode`。Unifabric Agent 使用 `ibnetdiscover` 发现 IB 交换机网络，把解析出的交换机与连接关系写入现有的 `Switch` CR。Unifabric Controller 根据网卡角色把每个 IB 子网归类为 scale-out 或 storage，复用现有路径计算 leaf、spine、core 分层，并把结果写入 Node 对象的标签和 `Topology` CRD（由拓扑状态控制器把 Node 标签聚合而来，供可视化使用），这些流程保持不变。

发现与解析逻辑沿用 NVIDIA Topograph 的 InfiniBand provider，包括 [dsx-ai-factory/topograph#560](https://github.com/dsx-ai-factory/topograph/pull/560) 引入的多端口探测。Unifabric 在一个关键点上做了扩展：Topograph 只选择单一计算网络并发布一套 fabric 标签，而 `unifabric-ib` 在一次发现中把网络同时归类为 scale-out 和 storage 两种角色，并发布两套标签。

## 动机

Unifabric 当前的 scale-out 拓扑标签有两种来源：`unifabric-roce`（通过交换机 Agent 的内置 LLDP 发现）和 `external`（由 NVIDIA Topograph 等外部系统写入标签）。storage 拓扑只支持 `unifabric-roce`。这两条现有路径都无法覆盖 InfiniBand 集群的需求：

1. `unifabric-roce` 依赖以太网 LLDP：交换机 Agent 采集 LLDP 邻居，发现路径（`pkg/agent/fabricnode`、`pkg/controller/switchtopology`）据此计算拓扑。InfiniBand 网络不提供 LLDP 邻居信息，这条路径在 IB 集群上没有输入。
2. `external` 模式可以委托给 Topograph 进行网络拓扑发现，但 Topograph 的 ScaleOut 网络检测仅支持单一网络类型。在计算与存储使用独立 IB 子网的集群中（例如，每台主机分别配备一块计算 IB 网卡和一块存储 IB 网卡），存储网络拓扑可能无法被完整发现。对于这类场景，可以考虑扩展 Topograph 的检测能力，或采用自研方案实现网络拓扑发现。

为此，本设计新增一种内置发现模式 `unifabric-ib`，让 IB 集群获得与 RoCE 集群相同的完整流程：自动发现、面向拓扑感知调度的 Node 标签，以及面向可视化的 `Topology` CRD，同时覆盖计算网络和存储网络。

### 目标

- 为 `topoDiscovery.scaleOut.mode` 和 `topoDiscovery.storage.mode` 增加 `unifabric-ib` 模式，两个小节可独立选用。
- 从 `ibnetdiscover` 输出发现完整的 IB 交换机树（leaf、spine、core，层数不限）。
- 复用现有网卡选择器的归类结果，将不同 IB 子网确定性地映射到 scale-out 和 storage 角色，不引入新的归类配置。
- 复用现有标签模板（`scale-out.unifabric.io/tier-{N}`、`storage.unifabric.io/tier-{N}`）和现有 `Topology` 聚合流程，保持不变。

### 非目标

- scale-up（NVLink）发现。`topoDiscovery.scaleUp.mode` 保持 `external`：在 MNNVL 系统上，NVLink 域信息由 GPU Operator 的 GPU Feature Discovery 写入 Node 标签（`nvidia.com/gpu.clique`），具体参考 [GPU Feature Discovery 文档](https://github.com/NVIDIA/k8s-device-plugin/blob/main/docs/gpu-feature-discovery/README.md)。

## 方案

方案由四部分组成。

1. **网卡上报。** Agent 复用现有的网卡上报逻辑：IB 网卡和其他 RDMA 网卡一样进入 `FabricNode.status` 的 `scaleOutNics`/`storageNics`（含现有的 `state` 字段，不活跃的端口也照常上报），仅为 `NicInfo` 增加两个可选字段：`linkLayer` 复用 `RDMAEndpoint` 已有的 `LinkLayer` 枚举（`InfiniBand`/`Ethernet`，取自 sysfs 的 `link_layer`），IB 发现路径只处理 `InfiniBand` 的网卡；`gids` 按 GID 表索引升序上报非零条目（冒号分隔的 sysfs 原样格式，与 `RDMAEndpoint` 的 GID 字符串格式一致），首元素即 `gids/0`：

   ```yaml
   apiVersion: unifabric.io/v1beta1
   kind: FabricNode
   metadata:
     # 与 Kubernetes Node 同名，集群级资源。
     name: node-a
   status:
     nodeRole: GPU
     nodeIP: 10.0.0.11
     totalNics: 2
     scaleOutNics:
       - name: ib0
         rdmaDeviceName: mlx5_0
         rdma: true
         state: up
         ipv4: ""
         ipv6: ""
         # 新增可选字段：链路层类型，枚举 InfiniBand/Ethernet
         linkLayer: InfiniBand
         # 新增可选字段：端口 GID 表中的非零条目，按表索引升序，
         # 首元素即 gids/0。控制器取首个 GID：高 64 位是子网前缀，
         # 用于把子网与网卡角色关联；低 64 位是端口 GUID，
         # 可与 ibnetdiscover 输出中的 H-<guid> 直接对应。
         # 取 gids/0 的依据：IBTA《InfiniBand Architecture Specification
         # Volume 1》§4.1.1，GID 表索引 0 是端口 GID（{SubnetPrefix[63:0],
         # GUID[63:0]}），为不变项（invariant），不能被删除或任意改写。
         gids:
           - "fe80:0000:0000:0000:a088:c203:0033:f66a"
     storageNics:
       - name: ib1
         rdmaDeviceName: mlx5_1
         rdma: true
         state: up
         ipv4: ""
         ipv6: ""
         linkLayer: InfiniBand
         gids:
           - "fe80:0000:0000:0001:a088:c203:0033:f66b"
   ```

2. **子网扫描与写入 `Switch` CR。** 由被选举的上报节点执行，解析结果直接写入现有 CRD，不引入新的上报结构。

   - 选举方式：`ibnetdiscover` 从子网内任意成员端口都能看到整个子网，每个子网只需要一次成功采集。选举的目的在于避免扫描风暴：`ibnetdiscover` 每次运行都会遍历子网内的全部交换机与端口，如果不选举、每个节点对每块网卡都发起扫描，一个 122 节点、每节点 8 块 IB 网卡的集群在每个采集周期会产生近千次全网扫描，重复查询同一批设备，给子网管理面（opensm 及各设备的 SMA）带来无谓压力（topograph#560 的评审讨论指出了同类问题，该 PR 的并发上限只能限制单节点内的并发，不能减少总扫描次数）；选举后每个子网每个周期只扫描一次，集群规模增长时管理面负载保持恒定。上报节点通过 `coordination.k8s.io/Lease` 选举产生：每个子网一个 Lease 对象，放在 unifabric 的系统命名空间下，名称由子网前缀派生（前缀含 `:`，需转换为合法对象名，如 `ib-reporter-fe80-0000-0000-0000`）；拥有该子网活跃端口的各节点 Agent 使用 client-go 的 leaderelection 包竞争该 Lease，`holderIdentity` 为节点名。只有持有 Lease 的 Agent 执行扫描，沿用现有的 Agent 重同步周期并加入抖动。续约超时后 Lease 自动可被其他候选节点接管，故障切换无需 Controller 参与；一个节点接入多个子网时会分别持有或竞争多个 Lease，互不影响。
   - 扫描方式：Agent 从 `/sys/class/infiniband/*/ports/*/state` 列出活跃 IB 端口（state 包含 `ACTIVE`，CA 名称按 `^[A-Za-z0-9_.-]+$` 校验，端口号按 `^[0-9]+$` 校验），按 `gids/0` 的子网前缀分组，对每个子网取一个端口运行 `ibnetdiscover --Ca <ca> --Port <port>`。`ibnetdiscover` 一次只扫描指定端口所在的子网，按端口探测而不是只用默认端口，确保存储和计算子网都被扫描到，不会因为默认端口接在存储子网上而漏掉计算子网（topograph#560 的核心修复）。
   - 解析方式：沿用 Topograph `pkg/ib` 的行格式。`Switch N "S-<guid>" # "<description>" ...` 开启一个交换机块，交换机名称从形如 `MF0;IB-ComputeLeaf-01:MQM8700/U1` 的描述中提取（`;` 与 `:` 之间的片段），描述中不含 ` ;:` 分隔符时按原文使用；交换机块内的 `[port] "S-<guid>"[port] # "<description>"` 和 `[port] "H-<guid>"[port] # "<description>"` 行分别记录交换机到交换机、交换机到主机网卡的连接；`Ca N "H-<guid>" # "node-a mlx5_0"` 把主机网卡 GUID 映射到节点名（第一个空白分隔的字段）。
   - 写入方式：每台 IB 交换机一个 `Switch` CR。ibnetdiscover 的连接关系与 LLDP 邻居同构，直接复用 `SwitchNeighbor`（`RemoteSystemType: KubernetesNode|Switch` + `RemoteSystemName` + `LinkCount`）；`Switch` 本来就支持无 `mgmtIP` 的交换机参与拓扑，IB 交换机不需要 switch-agent 接入，正是这种形态。网卡 GUID、端口、LID 和链路速率在写入时被丢弃；只保留节点名能匹配集群 Node 的连接（与 Topograph 相同的规则：未知主机不进入 leaf 分组）。每台交换机一个 CR，规模随网络自然扩展，不会出现单对象体积上限问题；带 `unifabric.io/discovered-by: unifabric-ib` 标签且不再出现在采集结果中的 `Switch` 由控制器按 TTL 回收。示例如下：

   ```yaml
   apiVersion: unifabric.io/v1beta1
   kind: Switch
   metadata:
     # 解析出的交换机名；无法解析出名称时回退为 GUID。
     name: ib-computeleaf-01
     labels:
       # 标记来源，与 LLDP 接入的交换机区分，供控制器过滤与垃圾回收。
       unifabric.io/discovered-by: unifabric-ib
     annotations:
       # 交换机 GUID 与所属子网前缀，用于跨采集合并与去重。
       unifabric.io/ib-switch-guid: "S-0000000000000011"
       unifabric.io/ib-subnet-prefix: "fe80:0000:0000:0000"
   # IB 来源的交换机不写 spec.role，保持为空。网络角色是子网级信息，
   # 由 IBTopologyDiscoveryController 按网卡角色归类后直接用于分层计算。
   spec: {}
   status:
     # 复用现有邻居结构：交换机到交换机、交换机到节点的连接。
     # 备注：字段名带有 LLDP 前缀，但 IB 来源的邻居并非来自 LLDP。
     # 是否把 lldpNeighbors 重构为 neighbors、lldpNeighborCount 重构为
     # neighborCount（保留旧字段做兼容过渡），会议讨论下。
     lldpNeighbors:
       - remoteSystemType: Switch
         remoteSystemName: ib-computespine-01
         linkCount: 1
       - remoteSystemType: KubernetesNode
         remoteSystemName: node-a
         linkCount: 1
       - remoteSystemType: KubernetesNode
         remoteSystemName: node-b
         linkCount: 1
     lldpNeighborCount: 3
   ```

3. **归类。** Controller 中新增 `IBTopologyDiscoveryController`，监听 IB 来源的 `Switch` 与 `FabricNode`，在采集结果变化时重新计算，把每个子网归类为 scale-out 或 storage。归类是子网级信息，不写入 `Switch.spec.role`，IB 来源的交换机 role 保持为空。

   - 合并与去重：不同上报节点对同一子网的采集按子网前缀与交换机 GUID 去重并做一致性校验；如果两次采集对同一节点给出冲突的 leaf 分组，reconcile 以上报 condition 的方式失败，而不是写入有歧义的标签（规则与 topograph#560 相同）。
   - 归类规则：子网角色直接继承成员网卡的角色。每个子网的成员主机端口在 `FabricNode.status` 中已被网卡选择器归入 `scaleOutNics` 或 `storageNics`，通过 `gids` 首元素（即 `gids/0`）即可把子网与网卡角色关联（高 64 位子网前缀确定子网归属，低 64 位端口 GUID 可与 `ibnetdiscover` 的 `H-<guid>` 精确对应）：成员网卡出现在 `storageNics` 的子网归为 storage，出现在 `scaleOutNics` 的子网归为 scale-out。不新增任何归类配置。
   - 错误处理：同一子网的成员网卡同时出现在 `scaleOutNics` 和 `storageNics`，或某个 `unifabric-ib` 小节没有匹配到任何子网，都会以 condition 形式报告错误。

4. **分层计算与标签。** 归类完成后，`Switch` 的邻居关系加上 `FabricNode` 提供与现有 `SwitchTopologyDiscoveryController` 相同的输入，分层计算按子网范围执行，Node 标签写入沿用现有流程与所有权规则（`manageScaleOut`/`manageStorage`），确保 `unifabric-ib` 与 `unifabric-roce` 不会争用同一套标签；`topologystatus` 本来就按标签列出 Node，不关心标签由谁写入，`Topology` CRD 与可视化无需任何改动。

### 配置

```yaml
topoDiscovery:
  scaleOut:
    mode: unifabric-ib
    nodeLabel:
      keyTemplate: "scale-out.unifabric.io/tier-{{ .Tier }}"

  storage:
    mode: unifabric-ib
    nodeLabel:
      keyTemplate: "storage.unifabric.io/tier-{{ .Tier }}"
```

- `unifabric-ib` 不引入新的 values 配置：计算网卡与存储网卡的区分沿用现有的网卡选择器（`fabricNode.scaleOutInterfaceSelector`/`storageInterfaceSelector`），子网归类直接继承网卡角色。
- 两个小节可以独立使用 `unifabric-ib`；主机同时有 IB 计算网络和以太网存储网络时，`scaleOut: unifabric-ib` 搭配 `storage: unifabric-roce` 是合法组合。
- 节点上需要 `ibnetdiscover`（标准 `infiniband-diags` 包）；Agent 镜像内置该工具，无需在宿主机安装。

### 用户故事

#### 故事 1 同一集群内的计算与存储 IB 网络

作为运行独立计算和存储 InfiniBand 子网的 GPU 集群管理员，我把两个模式都设为 `unifabric-ib`；计算网卡与存储网卡已由现有的网卡选择器区分，无需新增配置。Unifabric 为每个节点写入它在两个网络中的位置标签，Kueue 基于 scale-out 标签调度，存储网络以 `Topology` CRD 形式可见，全程无需部署任何外部系统。

### 约束与考量

- 发现过程以受限并发运行 `ibnetdiscover`，且只在被选举的上报节点上执行，集群规模增长时对网络的 SMP 负载保持恒定。
- 发现结果复用 `Switch` 与 `FabricNode` 两个现有 CRD，不新增上报结构；每台交换机一个 `Switch` CR，规模随网络自然扩展。
- 归类直接继承网卡角色，要求网卡选择器能区分计算网卡与存储网卡；同一 IB 子网同时承载计算和存储流量的场景无法拆分为两种角色。
- 解析规则与 Topograph `pkg/ib` 保持行为兼容，上游测试数据（例如 `tests/output/ibnetdiscover/compute-storage.out`）可复用为测试向量。

### 风险与缓解

- **采集冲突。** 两条链路对同一节点报告不同 leaf 分组时，reconcile 以 condition 失败，不写错误标签（规则取自 topograph#560）。
- **上报节点故障。** 当前上报节点停止上报时选举会重新选择；长期未被采集刷新的 IB 来源 `Switch` 按 TTL 回收，其标签由现有标签写入器排空。
- **链路降级或抖动。** 离开 ACTIVE 状态的端口退出枚举；在 TTL 内保留最后一次一致的采集结果，避免标签抖动。

## 实现步骤

按依赖顺序分为 Agent、Controller、测试三部分。

1. **Agent：网卡上报扩展。** 为 `NicInfo` 增加 `linkLayer` 和 `gids` 两个可选字段，采集逻辑并入现有网卡上报路径，从 sysfs 读取 `link_layer` 与 GID 表。自研。
2. **Agent：IB 端口枚举。** 从 `/sys/class/infiniband/*/ports/*/state` 枚举活跃端口并按 `gids/0` 前缀分组。自研。
3. **Agent：子网级 Lease 选举。** 每个子网一个 `coordination.k8s.io/Lease`，用 client-go leaderelection 竞争，持有者执行扫描。自研（Topograph 为中心化服务端架构，无此机制）。
4. **Agent：`ibnetdiscover` 执行与解析。** 持有 Lease 的 Agent 对每个子网运行一次 `ibnetdiscover --Ca --Port`，解析交换机块、连接行与 `Ca` 行。解析器代码参考 Topograph `pkg/ib` 实现（行格式、交换机名提取、节点名映射规则保持行为兼容）。
5. **Agent：写入 `Switch` CR。** 把解析结果映射为每台交换机一个 `Switch` CR（`SwitchNeighbor` 复用、`discovered-by` 标签、GUID 与子网前缀注解）。自研。
6. **Controller：`IBTopologyDiscoveryController`。** 监听 IB 来源的 `Switch` 与 `FabricNode`，做合并去重、一致性校验与子网归类。自研。
7. **Controller：分层计算与标签、TTL 回收。** 复用现有 `SwitchTopologyDiscoveryController` 流程与 `manageScaleOut`/`manageStorage` 所有权规则，新增 IB 来源 `Switch` 的 TTL 垃圾回收。自研（复用本仓库现有代码）。
8. **测试。** 解析器单元测试与 e2e 回放使用的 `ibnetdiscover` 测试数据（如 `compute-storage.out`）来自 Topograph `tests/output/ibnetdiscover/`。
9. **许可证与来源声明。** 对参考或来自 [github.com/dsx-ai-factory/topograph](https://github.com/dsx-ai-factory/topograph)（Apache-2.0）的结果解析代码（步骤 4）与测试数据（步骤 8），在文件头保留上游版权声明（`Copyright NVIDIA CORPORATION & AFFILIATES`），在 SPDX 标识（`Apache-2.0`）之外增加指向该仓库及其 LICENSE 的来源说明（仓库地址与原始文件路径）；不得移除或修改上游 LICENSE 要求的声明内容。

## 测试计划

- 针对解析器的单元测试，使用来自 Topograph 的测试数据，包括双子网的 `compute-storage` 场景。
- 针对 Controller 的测试：合并冲突、子网归类、角色冲突与无匹配的错误 condition，以及与 `unifabric-roce` 的标签所有权。
- 一个 e2e 流程：在假节点上通过 Agent 解析路径回放保存的 `ibnetdiscover` 采集结果，并验证 scale-out 和 storage 两个 `Topology` CRD，沿用现有 nvidia-topograph 集成测试场景的做法。
