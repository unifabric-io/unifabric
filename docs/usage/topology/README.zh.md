# 网络拓扑发现

English version: [README.md](./README.md)

## 简介

AI 集群依赖三张高速网络:

- scale-out 网络承载 GPU 跨节点通信。
- scale-up 网络连接节点内的 GPU。
- 存储网络承载到高速存储的流量。

一组训练或推理的 AI workload 需要在这些网络上做拓扑亲和调度。如果它们的 Pod 被随机
摊开到集群的不同节点,流量就会跨越 spine 层甚至 core 层,带来额外的网络时延,并且与
其他任务的流量相互干扰。例如推理的 PD 分离场景,Prefill 实例和 Decode 实例之间要
传输 KV cache。两者落在同一台 leaf 交换机下,传输只经过一跳;落在不同的 spine 域下,
传输要多经过两跳以上。

Kueue、Volcano、KAI Scheduler 等调度器都支持拓扑亲和调度,但它们不直接感知物理
网络。它们只读取 Node 上的拓扑 label。要让调度器工作,必须有人把每个节点在网络中的
位置通过 label 记录在 Node 对象中。这就遇到两个问题:

- 拓扑 label 没有可靠的来源。

  物理连接关系散落在多家厂商的交换机、网卡和网络管理平台里。各厂商的工具互不相通,
  也不与 Kubernetes 对接。在数百台节点的集群里,人工给每个节点打拓扑 label 繁琐且
  容易出错。交换机扩容或换线之后,label 不会跟着变,与实际拓扑不一致。
- 运维没有一张全局的网络拓扑图。

  节点、leaf、spine、core 之间的连接关系不在任何一个地方完整呈现。训练任务变慢、
  链路报错时,运维要先从交换机、网卡和厂商平台里一点点拼出"哪些节点在同一台 leaf
  下、哪条链路承载了哪些任务",再去定位问题。在数百台节点、几十台交换机的集群里,
  这一步本身就要花很长时间。

网络拓扑发现探测这三张网络中主机与交换机、交换机与交换机之间的连接关系,并把结果
写成两种标准的 Kubernetes 资源,正对这两个问题:

- 分层 Node label。

  Unifabric 实时发现每个节点所在的 leaf、spine、core 域并写入 Node label。调度器
  直接消费,把一个任务的 Pod 收敛在同一个交换机域内。物理拓扑变化时 label 自动更新。
- `Topology` 资源。

  交换机成为 `Switch` 资源,三张网络各自成为一个 `Topology` 资源,呈现每张网络的
  性能域和每个节点的路径。Grafana 把它们渲染成一张实时的全局拓扑图。某台交换机或
  某条链路出问题时,运维直接在图上看到它影响哪些节点、哪些任务,再去排查。

无论设备是 SONiC、NVIDIA Spectrum 还是 InfiniBand,结果都使用同一套 label 模型和
同一个 `Topology` 资源。调度器配置和运维流程不随厂商变化。

拓扑发现的范围如下:

| 拓扑关系 | 计算网络(scale-out) | 存储网络 | NVLink 网络(scale-up) |
| --- | --- | --- | --- |
| 每台主机所在的层级(属于哪个 leaf 域) | 支持 | 支持 | 支持(属于哪个 NVLink 域) |
| 每台 leaf、spine、core 交换机所在的层级 | 支持 | 支持 | 不适用 |

通过本篇文章你将学习到:

- 拓扑发现支持哪些网络、哪些发现方式、哪些网络设备。
- 四种发现方式各自如何完成拓扑发现。
- 如何查看结果:Node label、`Topology` 资源和 Grafana 拓扑图。
- Kueue、Volcano、KAI Scheduler 等调度器如何消费这些 label。

## 网络拓扑发现支持范围

每张网络在 Helm values 中有各自的发现模式。三张网络相互独立。例如,计算网络可以用InfiniBand 发现,同时存储网络用 LLDP 发现。

| 网络 | Helm value | 发现方式 |
| --- | --- | --- |
| 计算(scale-out) | `topoDiscovery.scaleOut.mode` | `unifabric-roce`:LLDP 发现(方式一)或人为定义拓扑(方式四)。`nv-topograph`:NetQ 导入(方式二)或 InfiniBand(方式三)。 |
| 存储 | `topoDiscovery.storage.mode` | `unifabric-roce`:LLDP 发现(方式一)或人为定义拓扑(方式四)。 |
| NVLink(scale-up) | `topoDiscovery.scaleUp.mode` | `nv-topograph`:NVIDIA Topograph 从 GPU Operator 读取 NVLink 域(方式三),或通过 NetQ 从 NVIDIA NMX 读取(方式二)。`manual`:由管理员写入 label。 |

每种方式支持的网络设备:

| 方式 | 交换机 | 交换机侧要求 | 集群侧要求 |
| --- | --- | --- | --- |
| 方式一:LLDP 发现(switch-agent) | SONiC 交换机、运行 Cumulus Linux 的 NVIDIA Spectrum 交换机。其他能运行 Docker 或 Linux 二进制的以太网交换机。 | 启用 LLDP。有 Docker 或 Linux shell 供 switch-agent 运行。 | Controller 能访问交换机管理 IP 的 TCP `8090` 端口。 |
| 方式二:NetQ 导入 | 由 NetQ 管理的 NVIDIA Spectrum-X 交换机。 | fabric 由 NetQ 管理。 | Topograph Pod 能访问 NetQ API。 |
| 方式三:InfiniBand | NVIDIA Quantum InfiniBand 交换机。 | 运行 Subnet Manager。 | GPU 节点上能运行 `ibnetdiscover`。 |
| 方式四:人为定义拓扑 | 任何发送 LLDP 的以太网交换机。交换机 OS 不限。 | 启用 LLDP。无其他要求。 | 无。 |

如果交换机不发送 LLDP,节点看不到邻居。Unifabric 不能发现任何交换机,Node 上没有
该网络的拓扑 label。

## 前提条件

- 已按[基础安装](../../getting-started.zh.md)完成部署,并按其中「其他网络环境」小节
  为各网络设置了 `topoDiscovery.*.mode`。
- `kubectl get fabricnodes` 中每个节点的 `READY` 为 `True`,网卡归类正确。基础安装
  的验证小节已覆盖这一步。
- 集群到设备的网络路径已打通。方式一需要运行 `unifabric-controller` 的节点能访问
  交换机管理 IP,管理 IP 通常在带外管理网络。方式二需要运行 Topograph Pod 的节点能
  访问 NetQ API。确认交换机和集群的防火墙允许这条路径。
- 仅拓扑可视化需要:集群已安装 Grafana Operator 及其 CRD。

## 拓扑发现方式

按[网络拓扑发现支持范围](#网络拓扑发现支持范围)中的表格为每张网络选择发现方式。各方式只是拓扑来源不同。
结果是一样的,都在[验证](#验证)中查看。

### 方式一:LLDP 发现(switch-agent)

LLDP 是一种协议,设备用它向链路对端的设备通告自己的身份和端口。每个节点上的
Unifabric Agent 读取每张 RDMA 网卡的 LLDP 邻居。这给出节点到 leaf 交换机的连接。
每台交换机上的 Unifabric switch-agent 读取交换机的 LLDP 邻居。这给出交换机之间的
连接。Controller 合并两者,计算出 leaf、spine、core 层级。

| 项目 | 说明 |
| --- | --- |
| 覆盖网络 | 计算、存储 |
| 拓扑来源 | 节点侧和交换机侧的 LLDP 邻居 |
| 能发现的层级 | 节点、leaf、spine、core。全部层级。 |
| Helm 配置 | `topoDiscovery.scaleOut.mode=unifabric-roce`(默认)或 `topoDiscovery.storage.mode=unifabric-roce`。存储网络还需要网卡 selector。参见[基础安装](../../getting-started.zh.md#安装)。 |

能在交换机上运行容器或二进制时,使用本方式。SONiC 和 Cumulus Linux 都支持。如果
安全策略禁止,或交换机 OS 没有 Docker 也没有 Linux shell,改用方式四。

限制:

- 本方式没有 NVLink 网络的拓扑来源。`topoDiscovery.scaleUp.mode` 保持 `manual`。

#### 配置

导出 mTLS 证书,在每台交换机上运行 switch-agent,为每台交换机创建一个 `Switch`
资源。参见 [switch-agent 接入](./switch-agent-onboarding.zh.md)。

### 方式二:NetQ 导入

Spectrum-X 是 NVIDIA 面向 AI 集群的以太网 RoCE 网络平台。NVIDIA NetQ 管理它的
fabric,并且已经掌握完整拓扑。Unifabric 通过 NVIDIA Topograph 从 NetQ 导入拓扑。
Topograph 是 NVIDIA 开源的服务,从厂商数据源读取网络拓扑并写入 Node label。

| 项目 | 说明 |
| --- | --- |
| 覆盖网络 | 计算。NetQ 可访问 NVIDIA NMX 时,还覆盖 NVLink。 |
| 拓扑来源 | NVIDIA Topograph 通过 NetQ API 读取 fabric 拓扑。NVLink 网络从 NMX 读取 NVLink 域。 |
| 能发现的层级 | 由 NetQ 中的 fabric 数据决定。通常是 leaf、spine 及以上全部层级。 |
| Helm 配置 | `topoDiscovery.scaleOut.mode=nv-topograph`、`nvidiaTopograph.provider.name=netq` 和 NetQ 凭据。NVLink 网络加 `topoDiscovery.scaleUp.mode=nv-topograph`。参见[基础安装](../../getting-started.zh.md#spectrum-x-网络)。 |

限制:

- 本方式不创建 `Switch` 资源。`Topology` 资源和 Node label 完整,但拓扑可视化中每个
  性能域的交换机成员卡片为空。
- 本方式不覆盖存储网络。如果存储网络运行在 Spectrum 交换机上,为它使用方式一或
  方式四。

> **注意:** Spectrum 交换机运行 Cumulus Linux,支持 LLDP 和 Docker。如果没有 NetQ,
> 可以在 Spectrum 交换机上改用方式一。

#### 配置

Node label 和 `Topology` 资源在基础安装完成后出现。不需要更多步骤。如果需要拓扑
可视化显示交换机,参见
[为 Topograph 场景补充 Switch 资源](./topograph-switch-resources.zh.md)。

### 方式三:InfiniBand

InfiniBand 是独立于以太网的高速互联技术。Subnet Manager 维护整个 fabric 的拓扑。
Unifabric 使用 NVIDIA Topograph 的 `infiniband-k8s` provider。Topograph 在 GPU 节点
的 Pod 中运行 `ibnetdiscover` 读取完整的 fabric 拓扑。NVLink 网络则读取 GPU Operator
写在每个节点上的 `nvidia.com/gpu.clique` label。

| 项目 | 说明 |
| --- | --- |
| 覆盖网络 | 计算、NVLink |
| 拓扑来源 | 计算网络用 `ibnetdiscover`。NVLink 网络用 GPU Operator 的 clique label。 |
| 能发现的层级 | 计算:leaf、spine、core。NVLink:tier 1,即 NVLink 域。 |
| Helm 配置 | `topoDiscovery.scaleOut.mode=nv-topograph` 和 `topoDiscovery.scaleUp.mode=nv-topograph`。参见[基础安装](../../getting-started.zh.md#infiniband-网络)。 |

计算网络的 label 值是由 InfiniBand 交换机 GUID 派生的名称,例如
`S-fc6a1c03006636c0`。

限制:

- 与方式二相同,本方式不创建 `Switch` 资源,拓扑可视化中的交换机成员卡片为空。
- 本方式不覆盖存储网络。如果存储网络是 RoCE,为它使用方式一或方式四。
- `infiniband-k8s` provider 需要 `ibnetdiscover` 成功。在没有 InfiniBand 的集群上,
  它不能单独产出 NVLink 域。

#### 配置

与方式二相同:不需要更多步骤。如果需要拓扑可视化显示交换机,参见
[为 Topograph 场景补充 Switch 资源](./topograph-switch-resources.zh.md)。

### 方式四:人为定义拓扑

本方式不在交换机上部署任何组件。每个节点上的 Unifabric Agent 仍然读取每张 RDMA
网卡的 LLDP 邻居,所以节点到 leaf 交换机的连接自动发现。leaf 以上的连接,由你为
每台 spine 和 core 交换机创建 `Switch` 资源,并在 annotation 中声明它的邻居。
Controller 把发现的 leaf 层与声明的上层合并。

| 项目 | 说明 |
| --- | --- |
| 覆盖网络 | 计算、存储 |
| 拓扑来源 | 节点侧 LLDP 邻居,加上 `Switch` 资源上的邻居声明 |
| 能发现的层级 | leaf 自动发现。spine 和 core 来自声明。 |
| Helm 配置 | 与方式一相同:`topoDiscovery.scaleOut.mode=unifabric-roce` 或 `topoDiscovery.storage.mode=unifabric-roce`。 |

安全策略禁止在交换机上运行软件、交换机由其他团队管理、或交换机 OS 不能运行
switch-agent 时,使用本方式。交换机只需要发送 LLDP。

限制:

- 声明的连接不会随物理连接变化更新。新增交换机或换线之后,更新声明。
- leaf 层仍然来自 LLDP。如果交换机不发送 LLDP,本方式不可用。
- 本方式没有 NVLink 网络的拓扑来源。

#### 配置

为 spine 和 core 交换机创建带 `unifabric.io/switch-neighbors` annotation 的 `Switch`
资源。参见[人为定义拓扑](./manual-topology.zh.md)。

## 验证

### 查看 Topology 与 Node label

本节以一个集群为例。计算网络是 SONiC RoCE。`node1` 和 `node2` 同时接入 `leaf1` 和
`leaf2`。`node3` 和 `node4` 同时接入 `leaf3` 和 `leaf4`。四台 leaf 交换机都接入
`spine1`:

![SONiC RoCE 示例拓扑](../../images/sonic-roce-topology-example.jpg)

发现完成后,查看计算网络的 label:

```bash
kubectl get nodes \
  -L scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2
```

```text
NAME    STATUS   ROLES    AGE   VERSION   SCALE-OUT.UNIFABRIC.IO/TIER-1   SCALE-OUT.UNIFABRIC.IO/TIER-2
node1   Ready    <none>   30d   v1.32.0   tier1-group1                    tier2-group1
node2   Ready    <none>   30d   v1.32.0   tier1-group1                    tier2-group1
node3   Ready    <none>   30d   v1.32.0   tier1-group2                    tier2-group1
node4   Ready    <none>   30d   v1.32.0   tier1-group2                    tier2-group1
```

`node1` 和 `node2` 的 `tier-1` 值相同,所以它们在同一个 leaf 域。四个节点的 `tier-2`
值相同,所以它们共享一个 spine 域。如果节点只有 `tier-1` 没有 `tier-2`,说明 leaf
以上的层级没有被发现。参见[故障排查](#故障排查)。

存储网络和 NVLink 网络使用同一套模型,各有自己的 label 前缀。示例中存储网络只有
一层 leaf:

```bash
kubectl get nodes -L storage.unifabric.io/tier-1
```

```text
NAME    STATUS   ROLES    AGE   VERSION   STORAGE.UNIFABRIC.IO/TIER-1
node1   Ready    <none>   30d   v1.32.0   tier1-group1
node2   Ready    <none>   30d   v1.32.0   tier1-group1
node3   Ready    <none>   30d   v1.32.0   tier1-group2
node4   Ready    <none>   30d   v1.32.0   tier1-group2
```

NVLink 网络执行 `kubectl get nodes -L scale-up.unifabric.io/tier-1`。值是 Topograph
从 GPU Operator clique label 或 NMX 得出的 NVLink 域 ID。值相同的节点共享一个
NVLink 域。

三张网络的 label 模型:

| Node label | 含义 |
| --- | --- |
| `scale-out.unifabric.io/tier-1` | 节点直连的计算网络 leaf 交换机域 |
| `scale-out.unifabric.io/tier-2` | leaf 之上的计算网络 spine 域 |
| `scale-out.unifabric.io/tier-3` | spine 之上的计算网络 core 域 |
| `storage.unifabric.io/tier-N` | 存储网络的对应层级。规则与计算网络相同。 |
| `scale-up.unifabric.io/tier-1` | 节点内的 NVLink 域 |

tier 数字越大,表示越靠近网络上层。两个节点的某个 label 值相同,表示它们在同一个
性能域。LLDP 发现把域命名为 `tier<N>-group<M>`。Topograph 使用自己的命名,例如
InfiniBand 交换机的 `S-fc6a1c03006636c0`。

Helm value `topoDiscovery.<fabric>.nodeLabel.keyTemplate` 定义 label key。默认值为
`scale-out.unifabric.io/tier-{{ .Tier }}`、`storage.unifabric.io/tier-{{ .Tier }}` 和
`scale-up.unifabric.io/tier-{{ .Tier }}`。模板可以修改,但必须且只能包含一个
`{{ .Tier }}`。

> **注意:** 如果修改了 key 模板,同时修改调度器配置中的 label key。否则调度器读不到
> 任何拓扑信息。

每张网络有一个 `Topology` 资源:`scaleout`、`storage` 和 `scaleup`。它汇总性能域的
层级关系和每个节点的域路径:

```bash
kubectl get topologies.unifabric.io
kubectl get topologies.unifabric.io scaleout -o yaml
```

```text
NAME       AGE
scaleout   30d
scaleup    30d
storage    30d
```

```yaml
apiVersion: unifabric.io/v1beta1
kind: Topology
metadata:
  name: scaleout
status:
  domains:
    - members:
        - spine1
      name: tier2-group1
      tier: 2
    - members:
        - leaf1
        - leaf2
      name: tier1-group1
      parent: tier2-group1
      tier: 1
    - members:
        - leaf3
        - leaf4
      name: tier1-group2
      parent: tier2-group1
      tier: 1
  nodes:
    - domainPath:
        - tier2-group1
        - tier1-group1
      nodes:
        - node1
        - node2
    - domainPath:
        - tier2-group1
        - tier1-group2
      nodes:
        - node3
        - node4
```

在 `status.domains` 中,`members` 列出该域包含的交换机,`parent` 指向上一层的域。在
`status.nodes` 中,每一组节点共享一条自上而下的域路径。`storage` 资源的结构相同。
`scaleup` 资源只有 tier 1 域,没有交换机成员。字段的完整说明参见
[Topology 参考](../../reference/topology.zh.md)。

#### 在调度器中使用 Node label

拓扑感知调度器把 Node label 组织成自上而下的层级。然后它按层级放置一个任务的 Pod。
下表把 Unifabric 的 label 对应到各调度器的配置入口。调度器本身的安装和配置,以其
官方文档为准:

| 调度器 | 配置入口 | 填写的 Unifabric label |
| --- | --- | --- |
| Kueue Topology Aware Scheduling | `Topology` 资源的 `spec.levels` | 自上而下:`scale-out.unifabric.io/tier-3`、`tier-2`、`tier-1`,最后是 `kubernetes.io/hostname` |
| Volcano network topology aware scheduling | `HyperNode` 资源 | 每个 tier 创建一个 HyperNode。成员选择器引用对应的 label。 |
| KAI Scheduler topology aware scheduling | `Topology` 资源的 `spec.levels` | 与 Kueue 相同 |

参考:[Kueue TAS](https://kueue.sigs.k8s.io/docs/concepts/topology_aware_scheduling/)、
[Volcano network topology](https://volcano.sh/en/docs/network_topology_aware_scheduling/)、
[KAI Scheduler](https://github.com/NVIDIA/KAI-Scheduler)。

### 查看拓扑可视化

Grafana 拓扑仪表盘把每个 `Topology` 资源渲染成交互式图形。图形与上面的 Node label
对应。每个 tier 1 域是一个 leaf 分组。每个 tier 2 域是 leaf 分组之上的 spine 分组。
每个节点挂在它所属的 leaf 分组下。

![拓扑可视化仪表盘](../../images/topology-visualization.png)

仪表盘的安装、外部 Grafana 的接入以及图形的详细解读,参见
[拓扑可视化](./topology-visualization.zh.md)。

## 故障排查

### Node 上没有任何拓扑 label

先确认前提条件成立:

1. 方式一和方式四,确认 `FabricNode` 的网卡已收到 LLDP 邻居:

   ```bash
   kubectl get fabricnode node1 -o jsonpath='{range .status.scaleOutNics[*]}{.name}{"\t"}{.lldpNeighbor.hostname}{"\t"}{.lldpNeighbor.port}{"\n"}{end}'
   ```

   ```text
   eth1	leaf1	Ethernet0
   eth2	leaf2	Ethernet0
   ```

   如果 hostname 列为空,确认交换机已启用 LLDP,并且节点网卡对应的交换机端口发送
   LLDP 报文。`FabricNode` 的 `LLDPNeighborsReady` condition 也会报告
   `LLDPNeighborMissing` 原因。

2. 方式二和方式三,确认 `mode` 为 `nv-topograph`,并且 Topograph Deployment 在运行:

   ```bash
   kubectl -n unifabric-system get deployment unifabric-nvidia-topograph
   kubectl -n unifabric-system logs deployment/unifabric-nvidia-topograph --tail 100
   ```

   方式二,检查 NetQ 凭据和 API 地址,并确认 Topograph Pod 能访问 NetQ API。方式三,
   确认日志显示 `ibnetdiscover` 正常运行。

3. 查看 Controller 日志:

   ```bash
   kubectl -n unifabric-system logs deployment/unifabric-controller --tail 200
   ```

### Node label 只有 tier 1

节点侧 LLDP 只发现 leaf 层。这个现象说明 leaf 以上的信息没有到达 Controller:

- 方式一:确认每台交换机都有 `Switch` 资源,并且 `kubectl get switch -o wide` 中
  `HEALTHY` 为 `true`。如果为 `false`,确认运行 Controller 的节点能访问交换机的
  `8090` 端口,并且 mTLS 证书匹配。参见
  [switch-agent 接入](./switch-agent-onboarding.zh.md#故障排查)。
- 方式四:确认 spine 层及以上每台交换机都有 `Switch` 资源,并且每个都有
  `unifabric.io/switch-neighbors` annotation。参见
  [人为定义拓扑](./manual-topology.zh.md#故障排查)。
- 如果物理网络只有 leaf 一层,只有 tier 1 是预期结果。

### 拓扑可视化中交换机成员为空

这是方式二和方式三的预期行为。参见
[为 Topograph 场景补充 Switch 资源](./topograph-switch-resources.zh.md)。

## 参考

- [拓扑发现概念](../../concepts/topology-discovery.zh.md):逻辑拓扑、发现方式与发现结果。
- [switch-agent 接入](./switch-agent-onboarding.zh.md):方式一。
- [人为定义拓扑](./manual-topology.zh.md):方式四。
- [为 Topograph 场景补充 Switch 资源](./topograph-switch-resources.zh.md):方式二和方式三的可选步骤。
- [拓扑可视化](./topology-visualization.zh.md):Grafana 仪表盘的安装与解读。
- [Topology 参考](../../reference/topology.zh.md)、[FabricNode 参考](../../reference/fabricnode.zh.md)、[Switch 参考](../../reference/switch.zh.md)。
- [Scale-out 拓扑发现设计](../../design/scaleout-topology.zh.md)、[Topology CRD 设计](../../design/topology-crd.zh.md)。
- [Helm values 参考](../../../chart/README.md):`topoDiscovery.*` 全部参数。
