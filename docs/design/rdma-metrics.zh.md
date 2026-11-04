# RDMA 指标设计

English version: [rdma-metrics.md](./rdma-metrics.md)

## 概要

每个节点上的 Agent 采集 RDMA 指标，并通过 controller-runtime 的 Prometheus 指标端点暴露。
采集器是节点本地的：它从 Linux sysfs、容器命名空间和 ethtool 读取主机和 Pod 的 RDMA 状态，
再用本地 `FabricNode` 快照中的拓扑和工作负载元数据给每个样本打标签。本文记录数据来源、
抓取流程、指标模型和背后的设计决策。如何启用抓取、看板和告警见
[主机与 Pod 的 RDMA 网卡指标使用指南](../usage/traffic-observability/rdma-metrics.zh.md)。

## 动机

RDMA 计数器位于每个设备的 sysfs 树和每个 Pod 的网络命名空间中。集群运维人员要回答
"哪个工作负载打满了这块网卡"，需要把这些计数器和 Kubernetes 元数据关联起来：Pod、顶层
工作负载所有者，以及接口所属的网络。通用的 node exporter 只读主机 sysfs，不了解 RDMA Pod、
SR-IOV 父接口，也不了解 Unifabric 的 scale-out 和 storage 分类。

Agent 已经在 `FabricNode` 中维护了所需的全部元数据（见 [FabricNode 设计](./fabricnode.md)）。
本设计在内存中复用该快照，一个采集器就能产出主机样本和 Pod 归属样本，并共享一套一致的标签。

用户故事：

- 作为可观测性工程师，我想要每个节点的 RDMA 设备、端口、接口和优先级计数器，用一套稳定的
  标签构建看板和告警。
- 作为平台运维人员，我希望样本归属到 Pod 和顶层工作负载所有者，把流量和错误映射到训练任务。
- 作为网络工程师，我想要一个区分 scale-out 和 storage 接口的 `kind` 标签，让网络级视图不
  混合流量类型。

### 目标

- 导出节点级 RDMA 设备、端口、接口和优先级计数器。
- 尽可能把 RDMA 指标归属到启用 RDMA 的 Pod。
- 增加拓扑标签，例如 `kind=scaleOut` 或 `kind=storage`。
- 为看板和告警提供稳定的 Prometheus 标签集。

### 非目标

- 不做集群级聚合。跨节点聚合由 Prometheus 完成。
- 不做历史存储。采集器只服务当前这次抓取。
- 不支持非 Linux。非 Linux 构建输出警告并跳过 RDMA 采集。
- 不保证不受支持的容器运行时上的 Pod 有样本。Pod 归属依赖 containerd。

## 方案

采集器运行在 Agent 内，注册到 controller-runtime 指标 registry。每次 Prometheus 抓取触发
一轮采集。采集只读本地输入：

- 来自 `fabricnode.Interface.GetFabricNode()` 的 `FabricNode` 内存快照。
- `FabricNode.status.rdmaPods`，提供 RDMA Pod 名称、容器 ID、host RDMA 模式和顶层所有者标签。
- `/sys/class/infiniband`，提供 RDMA 设备、端口、`counters`、`hw_counters` 和 traffic class。
- `/sys/class/net`，提供接口速率、MTU、运行状态、netdev 到 PCI 的映射以及 PF/VF 父接口解析。
- `/host/proc/<pid>/ns/mnt` 和 `/host/proc/<pid>/ns/net`，用于进入主机和 Pod 命名空间。
- `/host/run/containerd/io.containerd.runtime.v2.task/k8s.io/<container-id>/init.pid`，
  用于获取 containerd Pod 的 init PID。
- ethtool 统计，提供按优先级的 pause 和 discard 计数器。

每次 `Collector.Collect()` 调用：

1. 获取当前 `FabricNode` 快照。
2. 先采集主机样本。
3. 保留一份主机样本副本，供 host RDMA Pod 归属使用。
4. 从 `FabricNode.status.rdmaPods` 采集 Pod 样本。
5. 在 debug 级别记录警告。
6. 输出 `unifabric_node_info` 和所有 RDMA 样本。

如果没有当前 `FabricNode`，本次抓取返回空快照。

## 详细设计

### 主机指标

主机抓取在配置后进入主机 mount 命名空间，然后扫描 `/sys/class/infiniband`。对每个 RDMA
设备，采集器：

1. 先从 `gid_attrs/ndevs` 解析 netdev 名称，失败时回退到 PCI 设备映射。
2. 为 VF 设备解析 PF 接口。
3. 记录设备清单：设备名、provider、接口、父接口和端口。
4. 读取每个端口的 `hw_counters` 和 `counters`。
5. 读取接口指标：`port_speed_mbps`、`port_mtu` 和 `port_oper_state`。
6. 读取设备 traffic class，作为 `rdma_device_tos`。
7. 只对根接口读取 ethtool 优先级计数器。

`port_xmit_data` 和 `port_rcv_data` 的值在读取时乘以 4，与内核计数器的单位换算一致。

### Pod 指标

Pod 抓取依赖 `FabricNode.status.rdmaPods`。

对 host RDMA Pod，采集器把主机样本复制为 Pod 范围的样本，并附加 Pod 条目中的工作负载标签。
host RDMA Pod 不复制 `rdma_device_tos`。

对非 host RDMA Pod，采集器：

1. 把每个 containerd 容器 ID 解析为其 init PID。
2. 进入 Pod 的 mount 命名空间。
3. 读取 Pod 可见的 `/sys/class/infiniband` 树。
4. 在 Pod 命名空间内采集 RDMA 计数器、接口指标、traffic class 和 ethtool 优先级计数器。
5. 在第一个产出样本的容器后停止。

不受支持的容器运行时会被跳过并记录警告，因为 `containerInitPID()` 目前只支持
`containerd://` 形式的 ID。

### 指标模型

每个 RDMA 样本都是 Prometheus gauge。指标名加 `unifabric_` 前缀，除非样本名已带该前缀。
示例：

- `unifabric_port_rcv_data`
- `unifabric_port_xmit_packets`
- `unifabric_port_speed_mbps`
- `unifabric_port_mtu`
- `unifabric_port_oper_state`
- `unifabric_rdma_device_tos`
- `unifabric_rx_pause`
- `unifabric_tx_pause`
- `unifabric_rx_discards`
- `unifabric_tx_discards`

采集器还会输出 `unifabric_node_info{unifabric_node_name="<node>"} 1`。

所有 RDMA 样本共享以下标签集：

| 标签 | 含义 |
| --- | --- |
| `node_name` | FabricNode 名称。 |
| `device` | RDMA 设备，例如 `mlx5_0` 或 `rxe_eth1`。 |
| `ifname` | 与 RDMA 设备关联的接口。 |
| `parent_ifname` | VF 设备的 PF 接口；根设备与 `ifname` 相同。 |
| `port` | RDMA 端口名（如适用）。 |
| `priority` | ethtool 优先级计数器的流量优先级。 |
| `pod_name`、`pod_namespace` | Pod 归属样本填写。 |
| `topowner_*` | 来自 `FabricNode.status.rdmaPods` 的顶层工作负载所有者。 |
| `host_rdma` | 归属的 Pod 是否使用 host RDMA 模式。 |
| `is_root` | `ifname == parent_ifname` 是否成立。 |
| `kind` | `scaleOut`、`storage`、`scaleUp` 或空。 |
| `scope` | `host` 或 `pod`。 |
| `source` | `hw_counters`、`counters`、`interface`、`device` 或 `ethtool`。 |

### 接口 kind 标签

`kind` 由 `fabricNode` 选择器按以下顺序推导：

1. 匹配 `storageInterfaceSelector`。
2. 匹配 `scaleUpInterfaceSelector`。
3. 匹配 `scaleOutInterfaceSelector`。
4. 未配置 scale-out 选择器且以上都未匹配时，默认 `scaleOut`。

匹配先检查 `parent_ifname`，再检查 `ifname`。支持的选择器形式为
`interface=eth*,!eth9` 和 `cidr=172.17.0.0/16`。

### Kubernetes 暴露

Agent 指标端点由 `agent.config.metrics.bindAddress` 配置，默认 `:8082`。Helm chart 可以
渲染 Agent 指标 `Service` 和 RDMA Grafana 看板，由 `nodeMetrics` 和 `grafanaDashboard`
值控制。只有 `agent.enabled`、`nodeMetrics.enabled` 和全局 `serviceMonitor.enabled` 都为
true 时，才渲染 Agent `ServiceMonitor`。一个实现细节：Go Agent 启动时总是注册 RDMA 采集器。
`nodeMetrics.enabled` 控制的是围绕抓取和看板的 Helm 资源，不控制 Agent 内部的采集器注册。
启用步骤和验证命令见 [主机与 Pod 的 RDMA 网卡指标使用指南](../usage/traffic-observability/rdma-metrics.zh.md)。

### 失败处理

RDMA 抓取是尽力而为的：

- 忽略缺失的可选计数器目录。
- 计数器目录的权限错误和不存在错误不视为致命。
- 跳过非法或不可读的计数器值。
- 命名空间、PID、sysfs 和 ethtool 失败记为警告，在 debug 级别输出日志。
- 一次抓取出错只影响该轮指标输出，不会停止 Agent。

### 运行要求

- Linux 节点，且 RDMA 设备在 `/sys/class/infiniband` 下可见。
- Agent 能访问 chart 挂载的主机 `/proc` 和 containerd 运行时状态。
- Agent 容器为特权容器，以进入命名空间和访问主机设备。
- 非 host RDMA Pod 归属需要 containerd 运行时。
- Pod 和工作负载标签需要最新的 `FabricNode.status.rdmaPods`。

## 备选方案

以下备选方案根据本设计所反对的决策事后重建，并非决策时的记录。

- 独立的 exporter DaemonSet。否决，因为 Agent 已经持有 `FabricNode` 快照、特权和主机挂载。
  第二个特权 DaemonSet 会把这三者都重复一遍。
- 抓取时通过 Kubernetes API 做 Pod 归属。否决，因为抓取不能依赖 API server 延迟。内存中的
  `FabricNode` 快照让抓取本地化且开销低。
- 基于 CRI 的通用运行时归属层。未实现。init PID 查找是 containerd 特定的，其他运行时被跳过
  并记录警告。这是已知限制，不是被否决的选项。
- 把 sysfs 计数器输出为 Prometheus counter 类型。否决，因为内核值可能随设备和命名空间在
  采集器控制之外重置。gauge 暴露原始值，速率计算交给 PromQL。

## 实现历史

- 2026-04-19：初始实现，包含 RDMA 拓扑和指标支持。
- 2026-07-17：随拓扑 status 汇总与自动 label 发现一起更新。
- 2026-10-13：文档按设计模板重构。备选方案一节为事后重建。
