# FabricNode 设计

English version: [fabricnode.md](./fabricnode.md)

## 概要

`FabricNode` 是 Unifabric 的节点本地网络状态资源。每个 Kubernetes Node 上的 Agent 维护一个
`FabricNode`，把发现的 RDMA 拓扑、LLDP 邻居数据、RDMA Pod 元数据、节点角色和节点 IP 写入
`status`。Controller 和外部消费方读取 `FabricNode`，而不是直接抓取主机网络状态。这把发现
留在节点本地，同时暴露一个集群级 API。当前字段以
[FabricNode 参考手册](../reference/fabricnode.md)为准。

## 动机

拓扑发现和 RDMA 指标都需要每个节点的事实：哪些接口支持 RDMA、每个接口连到哪台交换机、
哪些 Pod 使用 RDMA。这些事实只存在于主机上，分布在 netlink、`lldpcli` 和容器运行时中。
在本设计之前，没有集群 API 承载它们。一个想拿到节点侧 LLDP 数据的控制器必须自己对每台主机
做特权探测，指标采集器也没有稳定的来源做 Pod 和工作负载归属。

本设计把主机探测集中到每个节点一个 Agent，并把结果发布为类型化的集群级资源。Controller 把
`FabricNode` 作为拓扑计算的节点侧输入（见[横向扩展拓扑设计](./scaleout-topology.md)和
[Topology CRD 设计](./topology-crd.md)）。指标采集器在内存中消费同一份数据（见
[RDMA 指标设计](./rdma-metrics.md)）。

用户故事：

- 作为 Controller 开发者，我想要一个带网卡和 LLDP 数据的类型化节点资源，让拓扑计算读
  API server 而不是主机状态。
- 作为可观测性工程师，我希望 Agent 记录哪些 Pod 使用 RDMA，让指标带上 Pod 和工作负载
  所有者标签。
- 作为集群运维人员，我想选择哪些接口属于 scale-out 和 storage 网络，让混合网卡的主机上报
  正确的拓扑列表。

### 目标

- 每个 Node 发布一个集群级 `FabricNode`，只由本地 Agent 拥有和写入。
- 发现支持 RDMA 的接口及其地址、链路状态和 LLDP 邻居。
- 通过可配置的选择器把接口分类到 scale-out 和 storage 列表。
- 记录启用 RDMA 的 Pod，包含容器 ID 和顶层工作负载所有者元数据。
- 上报节点角色、节点 IP 和就绪 conditions。

### 非目标

- `FabricNode` 不计算拓扑。Controller 从它推导性能域和 labels。
- `FabricNode` 不保存指标样本。采集器读取内存快照。
- 用户不编辑 `FabricNode`。资源的 `spec` 为空，status 由 Agent 拥有。
- 本版本不上报 scale-up 网卡。scale-up 选择器会被校验并把匹配接口从 `scaleOutNics` 排除，
  但不写任何 status 字段。

## 方案

`FabricNode` 是 `spec` 为空的集群级资源。Agent 是唯一写入方。每次协调扫描主机状态，构建
期望的 status，只在观测状态变化时写入。Agent 同时保留一份本地深拷贝，作为 RDMA 指标采集器
的快照，抓取时不调用 API server。

```yaml
apiVersion: unifabric.io/v1beta1
kind: FabricNode
metadata:
  name: node-gpu-1
status:
  conditions:
    - type: Ready
      status: "True"
      reason: Ready
      message: All FabricNode conditions are ready
    - type: LLDPNeighborsReady
      status: "True"
      reason: LLDPNeighborsReady
      message: All selected RDMA interfaces have LLDP neighbors
  totalNics: 4
  nodeRole: GPU
  nodeIP: 192.168.200.6
  scaleOutNics:
    - name: eth1
      rdma: true
      rdmaDeviceName: mlx5_0
      state: up
      ipv4: 172.17.1.11/24
      ipv6: ""
      lldpNeighbor:
        hostname: gpu-leaf-1
        mgmtIP: 10.10.10.1
        mac: 00:11:22:33:44:55
        port: Ethernet1/1
        description: GPU leaf switch
    - name: eth2
      rdma: true
      rdmaDeviceName: mlx5_1
      state: up
      ipv4: 172.17.2.11/24
      ipv6: ""
      lldpNeighbor:
        hostname: gpu-leaf-2
        mgmtIP: 10.10.10.2
        mac: 00:11:22:33:44:66
        port: Ethernet1/1
        description: GPU leaf switch
  storageNics:
    - name: eth9
      rdma: true
      rdmaDeviceName: mlx5_8
      state: up
      ipv4: 172.18.9.11/24
      ipv6: ""
      lldpNeighbor:
        hostname: storage-leaf-1
        mgmtIP: 10.10.20.1
        mac: 00:11:22:33:55:55
        port: Ethernet9/1
        description: Storage leaf switch
    - name: eth10
      rdma: true
      rdmaDeviceName: mlx5_9
      state: up
      ipv4: 172.18.10.11/24
      ipv6: ""
      lldpNeighbor:
        hostname: storage-leaf-2
        mgmtIP: 10.10.20.2
        mac: 00:11:22:33:55:66
        port: Ethernet9/1
        description: Storage leaf switch
  rdmaPods:
    - namespace: training
      name: worker-0
      containerList:
        - containerd://8f3e2d3a2f5b6c7d
      hostRDMA: false
      topOwner:
        apiVersion: kubeflow.org/v1
        kind: PyTorchJob
        namespace: training
        name: resnet
```

主要的 status 分组：

- `conditions`：Kubernetes 风格的 conditions。`LLDPNeighborsReady` 表示所有被选中且处于
  `up` 状态的 RDMA 接口是否都有 LLDP 邻居数据。`Ready` 是聚合结论，只有其他 conditions
  全部为真时才为真。
- `totalNics`：scale-out 和 storage 拓扑列表中的网卡数量。
- `scaleOutNics` 和 `storageNics`：为各自网络选中的 RDMA 接口。每个条目包含接口名、RDMA
  设备名、IPv4 和 IPv6 CIDR、链路状态，以及 LLDP 邻居的主机名、管理 IP、MAC、端口和描述。
- `rdmaPods`：本节点上启用 RDMA 的 Pod，包含容器 ID、host RDMA 模式和顶层所有者引用。
- `nodeRole`：Agent 上报的 `GPU` 或 `Storage`。
- `nodeIP`：通过向配置的探测地址打开 UDP socket 检测到的节点 IP。

## 详细设计

### 协调流程

Agent 在 `pkg/agent/fabricnode` 中协调 `FabricNode`。启动时 Agent：

1. 从配置、`NODE_NAME` 或 `/etc/hostname` 解析节点名。
2. 创建 controller-runtime manager，并把 Pod 缓存限制在当前节点。
3. 启动周期性 Reconcile 循环。
4. 监听本地 Pod，在 Pod 状态变化时触发 Reconcile。

每次 Reconcile 中 Agent：

1. `FabricNode` 不存在时创建它。
2. 在 `initialScanDelay` 之后更新拓扑状态。
3. 对非存储 Agent 更新 RDMA Pod 状态。
4. 更新节点角色和节点 IP。
5. 只在观测状态变化时写 status。
6. 保存本地深拷贝供指标采集器使用。

存储节点的 Agent 退出时删除自己的 `FabricNode`。停机的存储节点因此不会在集群中留下过期的
上报状态。

### 网卡拓扑发现

拓扑发现通过 netlink 扫描主机网络接口：

1. 用 `netlink.LinkList` 列出链路。
2. 只保留类型为 `device` 的链路。
3. 跳过 SR-IOV VF。
4. 只保留能通过 `rdmamap` 映射到 RDMA 设备的接口。
5. 从 netlink 读取 IPv4 和 IPv6 地址。
6. 通过 `lldpcli` 读取 LLDP 邻居。
7. 构建 `NicInfo`。
8. 应用拓扑选择器，更新匹配的 status 列表。

当任何被选中且处于 `up` 状态的 RDMA 接口缺少 LLDP 邻居数据时，`LLDPNeighborsReady` 为 false。
没有被选中 RDMA 接口的节点是合法的：它上报 `totalNics: 0`，不会使该 condition 失败。相连的
交换机必须开启 LLDP，主机必须能通过 `lldpcli` 读到 LLDP 数据。

### RDMA Pod 状态

Agent 记录使用 RDMA 的 Pod，让指标能携带 Pod 和工作负载所有者标签。满足任一条件即视为
RDMA Pod：

- 使用了分配的 SR-IOV RDMA 设备。
- 运行在 host RDMA 模式。

对每个匹配的 Pod，Agent 记录命名空间和名称、容器 ID、host RDMA 标志和顶层所有者引用。
RDMA 指标采集器读取最新的内存快照，用 `scaleOutNics` 和 `storageNics` 给接口打 `kind`
标签，用 `rdmaPods` 把主机和容器命名空间的计数器归属到正确的对象。

### 接口选择器

Helm values 的 `fabricNode` 段配置选择器。支持的形式：

- `interface=eth*,!eth9`
- `cidr=172.17.0.0/16`

| 配置 | 写入字段 | 行为 |
| --- | --- | --- |
| `scaleOutInterfaceSelector` | `status.scaleOutNics` | 为空时包含所有未被 storage 或 scale-up 匹配的 RDMA 接口。 |
| `storageInterfaceSelector` | `status.storageNics` | 匹配的接口从 `scaleOutNics` 排除。 |
| `scaleUpInterfaceSelector` | 无 | 预留。会被校验并从 `scaleOutNics` 排除，但本版本不写 status 字段。 |

### 升级兼容性

Agent 是唯一写入方，写入是跳过相同状态的完整期望态 status 更新。新版本 Agent 从主机状态
重建同样的 status，不需要迁移步骤。能容忍可选字段缺失的消费方跨版本可用。

## 备选方案

以下备选方案根据本设计所反对的决策事后重建，并非决策时的记录。

- 各控制器直接探测主机状态。否决，因为每个消费方都需要在每个节点上拿特权主机访问，探测逻辑
  会重复。每节点一个 Agent 集中了特权并发布类型化 API。
- 命名空间级资源或用户可编辑的 `spec`。否决，因为数据是每个 Node 的观测状态，天然只有一个
  写入方。空 `spec` 加 Agent 拥有的 `status` 消除了写冲突。
- 为停机的存储节点保留过期的 `FabricNode`。否决，因为过期的存储条目会误导拓扑计算。存储
  Agent 退出时删除自己的资源。
- Agent 使用集群范围的 Pod 缓存。否决，因为 Agent 只需要本节点的 Pod。限制缓存可约束内存
  和 API 负载。

## 实现历史

- 2026-04-19：初始实现，包含 RDMA 拓扑和指标支持。
- 2026-07-17：随拓扑 status 汇总与自动 label 发现一起更新。
- 2026-10-13：文档按设计模板重构。备选方案一节为事后重建。
