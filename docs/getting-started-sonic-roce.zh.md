# 在通用 SONiC RoCE 网络部署 Unifabric

English version: [getting-started-sonic-roce.md](./getting-started-sonic-roce.md)

## 简介与部署目标

本文指导你在由 SONiC 交换机承载 RoCE 网络的集群中部署 Unifabric 拓扑发现。RoCE 是
在融合以太网上运行的 RDMA。Unifabric 从节点 RDMA 网卡、`FabricNode` 的 LLDP 邻居,
以及可选的交换机侧 LLDP 快照中发现 leaf、spine、core 拓扑。LLDP 是一种设备向链路
对端通告自身信息的协议。

完成本文后,集群将呈现以下结果:

- Node 带有分层拓扑 label,例如 `scale-out.unifabric.io/tier-1` 表示离 Node 最近的
  一层,其上依次为 `tier-2`、`tier-3` 等。内置发现使用 `tierN-groupM` 形式的 label
  值,例如 `scale-out.unifabric.io/tier-1=tier1-group1`。
- `FabricNode` 和 `Switch` 资源暴露发现的输入状态,`Topology` 资源暴露聚合后的
  只读拓扑。
- 节点 RDMA 状态可以通过 Agent metrics 观测。

本文包含两个 case,开始前先选定一个:

| Case | 适用情况 | 需要准备 |
| --- | --- | --- |
| Case 1:仅节点 LLDP | 安全策略禁止在交换机上运行软件,或交换机由其他团队维护 | 节点通过节点侧 LLDP 发现其 leaf 交换机。管理员用 `Switch` 资源和邻居 annotation 声明 leaf 以上的层级。 |
| Case 2:交换机运行 `switch-agent`(推荐) | 运维团队可以在 SONiC 交换机上运行 Unifabric Switch Agent,并希望自动发现交换机间拓扑 | 交换机已启用 LLDP。运行 `unifabric-controller` 的 Kubernetes Node 可以访问每台交换机的 TCP `8090` 端口。 |

## 前提条件

继续之前,确认以下各项均成立:

- [ ] 已通过[基础安装](./getting-started.zh.md)完成 Unifabric 安装。
- [ ] SONiC 交换机承载 RoCE 网络,且每台交换机都启用了 LLDP。
- [ ] 节点有 RDMA 网卡,且设备在 `/sys/class/infiniband` 下可见。
- [ ] 仅 Case 2:每台交换机可以运行 Docker 或其他容器运行时。如果交换机无法运行
      容器,改用 [switch-agent systemd 安装](./usage/switch-agent-deployment.zh.md)。
- [ ] 仅 Case 2:运行 `unifabric-controller` 的 Node 可以访问每台交换机管理 IP 的
      TCP `8090` 端口。

## 安装

先完成[基础安装](./getting-started.zh.md)。基础安装部署了所有场景共用的
Controller、Agent 和观测组件。

基础安装默认已选择 `unifabric-roce` 作为 scale-out 拓扑写入器,通常不需要额外改动。
如需显式应用该场景配置,在可以用 `kubectl` 和 Helm 访问集群的主机上执行:

```bash
helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set topoDiscovery.scaleOut.mode=unifabric-roce \
  --wait
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

如果 RoCE 网络同时承载存储网络,在上面的命令中追加:

```bash
--set topoDiscovery.storage.mode=unifabric-roce
```

这些配置只选择 SONiC RoCE 场景的拓扑写入器。镜像和观测配置沿用基础安装。

## 环境配置

下面两个 case 使用同一个示例网络。`node1` 和 `node2` 通过 `leaf1` 和 `leaf2` 接入,
组成 `tier1-group1`;`node3` 和 `node4` 通过 `leaf3` 和 `leaf4` 接入,组成
`tier1-group2`;它们共同的上联 `spine1` 组成 `tier2-group1`。

![RoCE 交换机拓扑发现示例](images/sonic-roce-topology-example.jpg)

### Case 1:仅节点 LLDP

该 case 不在交换机上运行任何软件。当不存在同 role 的 `Switch` 资源时,Unifabric
进入 node-only 模式:把 `FabricNode` LLDP 上报的交换机 hostname 视为合成 leaf,
只生成 tier 1。

1. 等待 Agent 上报 LLDP 邻居。tier 1 不需要任何交换机侧操作。在示例网络中,发现
   会生成两个 tier 1 域,每个 Node 获得自己的 `scale-out.unifabric.io/tier-1`
   label。

2. 要描述 spine、core 等上层,为每台上层交换机创建一个 `Switch` 资源,并给每个
   资源添加 `unifabric.io/switch-neighbors` annotation。不要为 leaf 创建资源,也
   不要设置 `spec.mgmtIP`。对于示例网络,在可以用 `kubectl` 访问集群的主机上应用
   以下文件:

   ```yaml
   apiVersion: unifabric.io/v1beta1
   kind: Switch
   metadata:
     name: spine1
     annotations:
       unifabric.io/switch-neighbors: '["leaf1", "leaf2", "leaf3", "leaf4"]'
   spec:
     role: ScaleOut
   ```

   ```bash
   kubectl apply -f spine1.yaml
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

> **警告:**不要在 `unifabric.io/switch-neighbors` 中写入 Kubernetes Node 名称。
> 该 annotation 只列出交换机邻居。Node 到交换机的链路来自 `FabricNode` LLDP。
> 数组中出现 Node 名称会破坏计算出的拓扑。

> **说明:**模式选择只判断每个同 role `Switch` 上是否存在该 annotation key。
> 空值或 `[]` 也会选中此模式,但不提供任何邻居链路。

应用后,四个 Node 都会获得 tier 1 和 tier 2 两个 label。[验证](#验证)章节给出了
预期的 `Topology` 内容。

### Case 2:使用 switch-agent 自动发现

该 case 在每台参与的交换机上运行 `switch-agent`,通过双向认证 TLS(mTLS)向
Controller 上报 LLDP 快照。只要任意一个同 role 的 `Switch` 资源缺少
`unifabric.io/switch-neighbors` key,Unifabric 就进入全自动模式,并忽略所有该
annotation。为保证配置无歧义,在所有 `Switch` 上都省略该 annotation。

> **说明:**要一次部署到多台交换机,使用
> [switch-agent 自动化部署脚本](./usage/switch-agent-deployment.zh.md),
> 不必执行下面的手动步骤。

1. 导出 pinned mTLS 证书。在可以用 `kubectl` 访问集群的主机上执行:

   ```bash
   mkdir -p ./tmp-switch-mtls

   kubectl -n unifabric-system get secret switch-controller-mtls-agent -o jsonpath='{.data.tls\.crt}' | base64 -d > ./tmp-switch-mtls/tls.crt
   kubectl -n unifabric-system get secret switch-controller-mtls-agent -o jsonpath='{.data.tls\.key}' | base64 -d > ./tmp-switch-mtls/tls.key
   kubectl -n unifabric-system get secret switch-controller-mtls-agent -o jsonpath='{.data.peer\.crt}' | base64 -d > ./tmp-switch-mtls/peer.crt
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

2. 登录目标交换机,创建证书目录:

   ```bash
   sudo mkdir -p /opt/unifabric-switch-agent/mtls
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

   然后把 `tls.crt`、`tls.key` 和 `peer.crt` 复制到交换机的
   `/opt/unifabric-switch-agent/mtls/` 目录。

3. 在交换机上确认 LLDP 能看到预期邻居:

   ```bash
   lldpcli show neighbors -f json0
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

4. 在交换机上启动 `switch-agent`。下面的命令使用 Docker bridge 网络,通过
   `-p 8090:8090` 发布 gRPC 端口。它通过挂载的 `lldpd` socket 读取 LLDP,不需要
   host network,也不需要特权权限。

   ```bash
   export SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>"
   export SWITCH_NAME="$(hostname)"

   docker pull "${SWITCH_AGENT_IMAGE}"

   docker rm -f unifabric-switch-agent 2>/dev/null || true

   docker run -d \
     --name unifabric-switch-agent \
     --restart unless-stopped \
     -p 8090:8090 \
     -e UNIFABRIC_SWITCH_AGENT_SWITCH_NAME="${SWITCH_NAME}" \
     -v /run/lldpd.socket:/run/lldpd.socket \
     -v /opt/unifabric-switch-agent/mtls:/etc/unifabric/switch-mtls:ro \
     "${SWITCH_AGENT_IMAGE}" \
     /usr/bin/unifabric/switch-agent
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

   > **说明:**如果交换机运行 SONiC 202006 到 202311,追加
   > `-e UNIFABRIC_SWITCH_AGENT_LLDP_CLI_VERSION=1.0.4`。其他版本保持默认值
   > `1.0.16`。

   > **说明:**如果交换机无法挂载 `/run/lldpd.socket`,改用
   > [hostProc 采集模式](./usage/switch-agent-deployment.zh.md)。该模式需要特权
   > 权限并挂载宿主机 `/proc`。其他环境变量参见
   > [switch-agent 参考](./usage/switch-agent-deployment.zh.md)。

5. 确认容器正在运行且日志没有错误:

   ```bash
   docker ps | grep unifabric-switch-agent
   docker logs --tail 100 unifabric-switch-agent
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

6. 为每台交换机创建一个 `Switch` 资源。Controller 连接 `spec.mgmtIP` 读取 LLDP
   快照。对于示例网络,创建五个资源:`leaf1` 到 `leaf4` 和 `spine1`。每个资源
   形如:

   ```yaml
   apiVersion: unifabric.io/v1beta1
   kind: Switch
   metadata:
     name: leaf1
   spec:
     mgmtIP: 192.0.2.11
     role: ScaleOut
     grpcPort: 8090
   ```

   `role` 按交换机服务的网络设置。`ScaleOut` 表示交换机连接用于跨节点流量的主机
   RDMA 网卡。`Storage` 表示交换机连接 `fabricNode.storageInterfaceSelector`
   选中的网卡。`ScaleUp` 表示交换机连接 GPU 侧互联网卡。默认值为 `ScaleOut`。
   `grpcPort` 默认为 `8090`。

   在可以用 `kubectl` 访问集群的主机上应用文件:

   ```bash
   kubectl apply -f leaf1.yaml
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

## 验证

确认 Controller 和 Agent 已就绪:

```bash
kubectl -n unifabric-system rollout status deployment/unifabric-controller
kubectl -n unifabric-system rollout status daemonset/unifabric-agent
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

查看一个 `FabricNode`:

```bash
kubectl get fabricnode <node-name> -o yaml
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

确认输出同时满足以下各项:

- `status.scaleOutNics` 包含预期的 scale-out RDMA 网卡。
- `status.scaleOutNics[*].lldpNeighbor.hostname` 存在。
- `status.conditions` 中的 `Ready` 和 `LLDPNeighborsReady` 为 `True`。

对于 Case 2,查看 `Switch` 资源:

```bash
kubectl get switch
```

```text
NAME     MGMTIP            ROLE       HEALTHY   NEIGHBORS
leaf1    192.168.122.72    ScaleOut   true      2
leaf2    192.168.122.80    ScaleOut   true      2
spine1   192.168.122.163   ScaleOut   true      2
```

确认每台交换机的 `HEALTHY` 为 `true`,且 `NEIGHBORS` 大于 `0`。

查看 Node label:

```bash
kubectl get nodes -L scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2,kubernetes.io/hostname
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

确认每个 Node 有连续的 `scale-out.unifabric.io/tier-N` label,tier 1 离 Node
最近。在 Case 1 且没有上层 `Switch` 资源时,只有 tier 1 是预期结果。

查看聚合拓扑。只有发现产生有效结果后,Controller 才会创建 `Topology` 资源:

```bash
kubectl get topology scaleout -o yaml
```

对于 Case 2 的示例网络,status 包含两个 tier,`switchMember` 列出真实的 `Switch`
名称:

```text
<!-- TODO: 待真实环境采集输出 -->
```

对于 Case 1 的 node-only 模式,只出现 tier 1 域,且 `switchMember` 为空,因为
合成 leaf 没有对应的 `Switch` 资源。

## 故障排查

### `FabricNode` 没有 scale-out 网卡

先确认前提条件成立:

1. 确认节点上的 RDMA 设备在 `/sys/class/infiniband` 下可见。
2. 如果设置了 `fabricNode.scaleOutInterfaceSelector`,确认它匹配真实的接口名或
   CIDR。
3. 确认 storage 或 scale-up selector 没有匹配到 scale-out 接口。被匹配的接口会
   离开 scale-out 分组。

### `LLDPNeighborsReady` 为 `False`

1. 确认交换机已启用 LLDP。
2. 确认节点侧 `lldpd` 正常工作,且 Agent Pod 能读取 LLDP 数据。
3. 确认 `fabricNode.initialScanDelay` 足够长。首次扫描在 LLDP 学习完成前运行会
   上报邻居缺失。

### Node 没有拓扑 label

1. 确认 `topoDiscovery.scaleOut.mode=unifabric-roce`。
2. 确认 `FabricNode.status.nodeRole` 不是 `Storage`。
3. 确认至少一个 `scaleOutNics` 条目同时有 `state=up` 和
   `lldpNeighbor.hostname`。
4. 在 Case 2 中,确认每个 LLDP hostname 都匹配一个 `Switch` 名称,且交换机
   status 中有 LLDP 邻居。
5. 在 Case 1 且存在上层时,确认每个同 role `Switch` 都带有该 annotation key,
   且其邻居名称能解析为节点发现的 leaf 或其他带 annotation 的 `Switch` 资源。
6. 查看 Controller 日志:

   ```bash
   kubectl -n unifabric-system logs deployment/unifabric-controller
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

7. 在 Case 2 中,在交换机上查看 switch-agent 日志:

   ```bash
   docker logs --tail 100 unifabric-switch-agent
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

## 卸载

删除 Helm release:

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

如果不再需要 CRD,删除它们。此操作会同时删除所有 `FabricNode`、`Switch` 和
`Topology` 资源:

```bash
kubectl delete crd fabricnodes.unifabric.io switches.unifabric.io topologies.unifabric.io
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

对于 Case 2,在每台交换机上删除 agent 容器:

```bash
docker rm -f unifabric-switch-agent
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

## 下一步

- 在调度器中使用这些 label:参见 [Kueue TAS 工作负载示例](./usage/workload-tas.zh.md)。
- 渲染拓扑:参见[拓扑可视化指南](./usage/topology-visualization.zh.md)。
- 查阅字段:参见 [FabricNode 参考](./reference/fabricnode.zh.md)。
- 调整安装配置:参见 [Helm values 参考](../chart/README.md)。
