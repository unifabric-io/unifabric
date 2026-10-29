# 自定义网络拓扑

English version: [custom-network-topology.md](./custom-network-topology.md)

## 简介

自定义网络拓扑是面向不能运行 switch-agent 的交换机的发现方式。原因可以是安全策略、
交换机由其他团队管理,或交换机 OS 没有 Docker 也没有 Linux shell。交换机只需要在
面向节点的端口发送 LLDP。

在本方式中,每个节点上的 Unifabric Agent 仍然读取每张 RDMA 网卡的 LLDP 邻居。
Controller 由此发现 leaf 交换机以及节点到它们的连接。对于 leaf 以上的层级,你为每台
spine 和 core 交换机创建一个 `Switch` 资源。在它的 `unifabric.io/switch-neighbors`
annotation 中,列出与它直接相连的下一层交换机。Controller 把发现的 leaf 层与声明的
上层合并。它写出与其他方式相同的 Node label 和 `Topology` 资源。

| 项目 | 说明 |
| --- | --- |
| 适用网络 | Scale-out 网络、Storage 网络 |
| 拓扑来源 | 节点侧的 LLDP 邻居,加上 `Switch` 资源上的邻居声明 |
| 能发现的层级 | leaf 层自动发现。spine 层和 core 层来自声明。 |
| 交换机侧要求 | 面向节点的端口启用 LLDP。交换机 OS 不限。交换机上不运行任何软件。 |
| 集群侧要求 | 无。 |
| Helm 配置 | 与 [RoCE 网络中 LLDP 发现](./roce-lldp-discovery.zh.md)相同:`topoDiscovery.scaleOut.mode=unifabric-roce` 或 `topoDiscovery.storage.mode=unifabric-roce`。 |

限制:

- 声明的连接不会随物理连接变化而更新。新增交换机或换线之后,更新声明。
- leaf 层只能来自节点侧的 LLDP。你不能为 leaf 或节点写声明,也没有其他方式告诉
  Controller 节点接在哪台 leaf 上。如果交换机面向节点的端口不发送 LLDP,包括所有
  交换机都不能启用 LLDP 的情况,节点找不到自己的 leaf,也就得不到任何拓扑 label。
  此时不能使用本方式,Unifabric 目前也没有其他可用的方式。
- Controller 只检查声明的格式和名称,不检查声明是否与物理连接一致。重叠或错误的声明
  通常不会报错,而是生成错误的 label。参见[验证](#验证)和[故障排查](#故障排查)。
- 当前暂不支持多平面 (multi-plane) 网络。参见
  [网络拓扑发现](./README.zh.md#查看-topology-与-node-label)。

各网络和各网络设备应使用哪种发现方式,参见
[网络拓扑发现](./README.zh.md#按网络选择发现方式)。

## 层级与声明规则

Controller 按与节点的距离计算每台交换机的层级:

```text
                    core1                     tier 3 (core)
                  /       \
            spine1         spine2             tier 2 (spine)
           /      \       /      \
        leaf1   leaf2   leaf3   leaf4         tier 1 (leaf)
          |       |       |       |
        node1   node2   node3   node4         节点
```

- leaf 是与节点直接相连的交换机,即 tier 1。节点通过 LLDP 发现它们。
- spine 与 leaf 相连,即 tier 2。
- core 与 spine 相连,即 tier 3。

层级只由连接关系决定,与交换机名称和 `Switch` 资源中的 `role` 无关。`role` 只选择
网络:`ScaleOut` 或 `Storage`。

每种交换机的声明方法:

| 交换机 | 是否创建 `Switch` 资源 | `unifabric.io/switch-neighbors` 的内容 | 上图中的例子 |
| --- | --- | --- | --- |
| leaf | 否 | 无 | 无 |
| spine | 是 | 与它直接相连的 leaf。使用节点侧 LLDP 看到的 leaf hostname。 | `spine1`: `["leaf1", "leaf2"]` |
| core | 是 | 与它直接相连的 spine。使用 spine 的 `Switch` 资源名称。 | `core1`: `["spine1", "spine2"]` |

按网络的层数:

- 只有 leaf 一层:不需要创建 `Switch` 资源。节点只有 tier 1 label。
- leaf 和 spine 两层:为每台 spine 创建 `Switch` 资源,并列出它的 leaf。spine 必须
  有声明。没有声明时,节点只有 tier 1 label。
- leaf、spine 和 core 三层:为每台 spine 和每台 core 创建 `Switch` 资源。spine 列出
  它的 leaf,core 列出它的 spine。

规则:

- 只声明相邻两层之间的连接。不要声明同层交换机之间的连接,例如 spine 与 spine。
  如果两台 spine 都已列出 leaf,这条连接会被忽略。如果其中一台没有列出 leaf,它会
  被算到上一层,成为 core。不要跳层声明,例如让 core 直接列出 leaf,否则该 core
  会被算到 spine 层。
- 连接没有方向。spine 与 core 之间的连接写在 core 上或 spine 上效果相同,每条连接
  只需声明一次。建议统一写在上一层的交换机上,即上表中的方式。
- 同一网络中,每个 `Switch` 资源都必须带有该 annotation。没有下层邻居需要声明时,
  值写 `'[]'`。

## 前提条件

- 已按[基础安装](../../getting-started.zh.md)部署 Unifabric,并且 `topoDiscovery.scaleOut.mode`
  或 `topoDiscovery.storage.mode` 为 `unifabric-roce`。
- 交换机面向节点的端口已启用 LLDP。节点必须看到它们的 leaf 交换机。用下面的命令确认。
  hostname 列不能为空:

  ```bash
  kubectl get fabricnode node1 -o jsonpath='{range .status.scaleOutNics[*]}{.name}{"\t"}{.lldpNeighbor.hostname}{"\t"}{.lldpNeighbor.port}{"\n"}{end}'
  ```

  ```text
  eth1	leaf1	Ethernet0
  ```

- 操作员必须了解 leaf、spine、core 交换机之间的物理连接。
- 同 role 的 `Switch` 都不运行 switch-agent。annotation key 为整个 role 选择自定义网络拓扑。
  如果该 role 中一个 `Switch` 有该 key 而另一个没有,Controller 忽略该 role 的全部
  annotation。

## 配置与使用

示例使用[层级与声明规则](#层级与声明规则)中的三层网络。

1. 不要为 leaf 交换机创建 `Switch` 资源。节点 LLDP 已经发现它们。Controller 用每台
   leaf 的 LLDP hostname 作为它的名称。

2. 为每台 spine 交换机创建一个 `Switch` 资源。在 annotation 中列出与它直接相连的
   leaf。Scale-out 网络把 `role` 设为 `ScaleOut`,Storage 网络设为 `Storage`:

   ```bash
   kubectl apply -f - <<'EOF'
   apiVersion: unifabric.io/v1beta1
   kind: Switch
   metadata:
     name: spine1
     annotations:
       unifabric.io/switch-neighbors: '["leaf1", "leaf2"]'
   spec:
     role: ScaleOut
   ---
   apiVersion: unifabric.io/v1beta1
   kind: Switch
   metadata:
     name: spine2
     annotations:
       unifabric.io/switch-neighbors: '["leaf3", "leaf4"]'
   spec:
     role: ScaleOut
   EOF
   ```

   annotation 的值是带引号的 JSON 字符串数组。每个名称必须是节点发现的某台 leaf 的
   LLDP hostname,或另一个同 role `Switch` 资源的名称。不要使用 Kubernetes Node 名称。

3. 如果网络有 core 层,为每台 core 交换机创建一个 `Switch` 资源。在 annotation 中
   列出与它直接相连的 spine。网络只有 leaf 和 spine 两层时,跳过本步骤:

   ```bash
   kubectl apply -f - <<'EOF'
   apiVersion: unifabric.io/v1beta1
   kind: Switch
   metadata:
     name: core1
     annotations:
       unifabric.io/switch-neighbors: '["spine1", "spine2"]'
   spec:
     role: ScaleOut
   EOF
   ```

> 警告:声明的连接不会随物理连接变化而更新。新增交换机或换线之后,更新 annotation。
> 否则 Node label 与真实拓扑不一致。

> 对同 role 的 `Switch` 资源,不要既运行 switch-agent 又设置该 annotation。

## 验证

Controller 不检查声明是否与物理连接一致。完成配置后,按下面的步骤把结果与真实拓扑
逐项对照。

1. 确认 `Switch` 资源存在。因为没有 switch-agent 向 Controller 上报,`HEALTHY` 和
   `NEIGHBORS` 为空:

   ```bash
   kubectl get switches -o wide
   ```

   ```text
   NAME     MGMTIP   ROLE       HEALTHY   NEIGHBORS
   core1             ScaleOut
   spine1            ScaleOut
   spine2            ScaleOut
   ```

2. 确认 `Topology` 资源上没有 Warning Event:

   ```bash
   kubectl describe topologies.unifabric.io scaleout
   ```

   如果 `Events` 中有 `AutoDiscoveryConflict` 或 `TopologyLabelConflict`,参见
   [故障排查](#故障排查)。

3. 查看 `Topology` 资源串起来的整体拓扑:

   ```bash
   kubectl get topologies.unifabric.io scaleout -o yaml
   ```

   ```yaml
   status:
     domains:
       - name: tier3-group1
         switchMember:
           - core1
         tier: 3
       - name: tier2-group1
         parent: tier3-group1
         switchMember:
           - spine1
         tier: 2
       - name: tier2-group2
         parent: tier3-group1
         switchMember:
           - spine2
         tier: 2
       - name: tier1-group1
         parent: tier2-group1
         tier: 1
       - name: tier1-group2
         parent: tier2-group1
         tier: 1
       - name: tier1-group3
         parent: tier2-group2
         tier: 1
       - name: tier1-group4
         parent: tier2-group2
         tier: 1
     nodes:
       - name: node-group1
         nodes:
           - node1
         switchDomainPath:
           - tier3-group1
           - tier2-group1
           - tier1-group1
       # node-group2 到 node-group4 省略
   ```

   逐项对照:

   - 每层域的数量与真实网络一致。上例中有 1 个 core 域、2 个 spine 域和 4 个 leaf 域。
   - 每个 spine 域和 core 域的 `switchMember` 只包含应在同一个域中的交换机。如果
     本应分开的 spine 出现在同一个域中,通常是声明有重叠。
   - 每个域的 `parent` 指向正确的上层域。
   - 每个节点的 `switchDomainPath` 长度等于网络层数。路径比预期短,说明它的 leaf
     没有被任何 spine 声明。

   leaf 交换机没有 `Switch` 资源,所以 leaf 域没有 `switchMember`。用 Node label 和
   `FabricNode` 中的 LLDP 邻居确认 leaf 域:

   ```bash
   kubectl get nodes -L scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2,scale-out.unifabric.io/tier-3
   ```

   Node label 和 `Topology` 资源的说明,参见
   [网络拓扑发现](./README.zh.md#查看-topology-与-node-label)。

## 故障排查

### 查看错误

Controller 只能识别下面几类错误:

- annotation 不是 JSON 字符串数组,或数组中有空名称。
- 邻居名称既不是节点发现的 leaf hostname,也不是同 role 的 `Switch` 资源名称。
- `Switch` 把自己列为邻居。
- 计算出的拓扑存在环、断层,或与已写入的 label 冲突。

出现这些错误时,Controller 停止本次协调,已有的 label 保持不变。错误出现在
`Topology` 资源的 Event 和 Controller 日志中:

```bash
kubectl describe topologies.unifabric.io scaleout
kubectl -n unifabric-system logs deployment/unifabric-controller --tail 200 | grep -i topology
```

### 拓扑与真实网络不一致

下面这些声明错误不会报错,只会生成错误的拓扑。按[验证](#验证)中的方法对照后,
检查对应的声明:

| 现象 | 常见原因 |
| --- | --- |
| 本应分开的多台 spine 出现在同一个域中 | 同一台 leaf 被多台 spine 声明,而这些 spine 实际不共享该 leaf。 |
| 某个节点进入了错误的 spine 域 | 它的 leaf 被写到了错误的 spine 下。 |
| 某些节点只有 tier 1 label | 它们的 leaf 没有被任何 spine 声明。 |
| core 与 spine 出现在同一层,没有 tier 3 | core 直接列出了 leaf。 |
| 某台 spine 出现在 tier 3,部分节点多出一层 | 这台 spine 没有列出 leaf,只列出了另一台 spine。 |
| 某台交换机不在任何域中 | 它与任何 leaf 都不连通,例如 annotation 为空且没有其他交换机列出它。 |

改正 annotation 后,Controller 不会修改已经写入的 label。已写入的 label 是锁定的,
新结果与它们冲突时,Controller 报告 `AutoDiscoveryConflict` 并保留旧 label。删除
`Topology` 资源,让 Controller 清除它写入的 label 并重新计算:

```bash
kubectl delete topologies.unifabric.io scaleout
```

Controller 清除 label 后,按当前声明重新写入 label,并重新创建 `scaleout` 资源。
重新计算期间,调度器读不到这张网络的拓扑 label。Storage 网络删除 `storage` 资源。

### Node label 只有 tier 1

- 确认 spine 层及以上的每台交换机都有 `Switch` 资源。
- 确认同 role 的每个 `Switch` 都有 `unifabric.io/switch-neighbors` annotation。一个
  没有该 key 的 `Switch` 会把整个 role 改回 switch-agent 发现。
- 确认没有为 leaf 交换机创建 `Switch` 资源,也没有 `Switch` 资源与节点发现的某台
  leaf 同名。
- 确认 annotation 是带引号的 JSON 字符串数组。数组只能包含交换机名称,不能包含
  Kubernetes Node 名称。
- 确认每台 leaf 都被某台 spine 声明。
- 按[查看错误](#查看错误)确认 Controller 没有报错。
- 如果物理网络只有 leaf 一层,只有 tier 1 是预期结果。

## 参考

- [网络拓扑发现](./README.zh.md):各发现方式,以及如何查看结果。
- [RoCE 网络中 LLDP 发现](./roce-lldp-discovery.zh.md):用 switch-agent 自动发现。
- [Switch 参考](../../reference/switch.zh.md):`Switch` 资源的 `spec`、`status` 和
  annotation。
- [Scale-out 拓扑设计](../../design/scaleout-topology.zh.md):Controller 如何合并节点
  LLDP 与邻居声明。
- [Topology CRD 设计](../../design/topology-crd.zh.md):label 锁定与通过删除 `Topology`
  重置的机制。
