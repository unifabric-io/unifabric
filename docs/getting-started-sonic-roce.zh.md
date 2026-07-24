# 通用 SONiC RoCE

本文说明如何在 SONiC 交换机承载 RoCE 网络的集群中部署 Unifabric。该场景通过节点 RDMA
网卡、FabricNode LLDP 邻居信息和交换机侧 switch-agent 的 LLDP 快照发现 scale-out
leaf、spine、core 拓扑。

## 部署目标

完成部署后，集群中应达成以下目标：

- Node 被写入可供调度系统消费的分层拓扑 label。默认从最靠近 Node 的
  `scale-out.unifabric.io/tier-1` 开始，向上依次使用
  `scale-out.unifabric.io/tier-2`、`scale-out.unifabric.io/tier-3` 等 key。
- 节点 RDMA 状态可观测，能够通过 Unifabric Agent metrics 查看 RDMA device、port、
  priority 和 Pod 归属等指标。
- 可以通过 `FabricNode` 和 `Switch` CR 查看输入状态，通过 `Topology` CR 查看汇总后的只读拓扑；调度侧消费写回到 Node 上的拓扑 label。

> 内置 LLDP 自动发现生成的 label value 使用 `tierN-groupM` 格式，例如
> `scale-out.unifabric.io/tier-1=tier1-group1`。

## 前置条件

- 可以访问的 Kubernetes 集群。
- 已安装 `kubectl` 和 Helm 3。
- 交换机需要具备 Docker 或其他容器运行环境。如果交换机不能运行容器，使用
  [switch-agent systemd 安装方式](./usage/switch-agent-systemd.zh.md)。

## 在集群中安装 Unifabric

下面命令使用最新的 release 版本，或者您指定特定的版本，你可以访问 [releases](https://github.com/unifabric-io/unifabric/releases) 页面查看。

```bash
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unifabric-io/unifabric/releases/latest | grep '"tag_name":' | cut -d '"' -f4)
CHART_VERSION="${LATEST_TAG#v}"
```

然后执行安装命令。

```bash
helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --version "${CHART_VERSION}" \
  --namespace unifabric-system \
  --create-namespace \
  --set topoDiscovery.scaleOut.mode=unifabric-roce \
  --set topoDiscovery.storage.mode=unifabric-roce \
  --set nodeMetrics.enabled=true \
  --set nodeMetrics.serviceMonitor.enabled=true \
  --set grafanaDashboard.enabled=true \
  --wait
```

参数说明：

| Helm value | 用途 |
| --- | --- |
| `topoDiscovery.scaleOut.mode` | 开启 scale-out 内置发现。没有 Switch CR 时由 FabricNode LLDP 生成 tier1；任一 Switch 缺少 `unifabric.io/neighbors` 时使用交换机 LLDP 并忽略全部 annotation；所有 Switch 都有该 key 时，由 annotation 把上层 Switch CR 连接到节点发现出的虚拟 leaf。 |
| `topoDiscovery.storage.mode` | 设置为 `unifabric-roce`，开启 RoCE storage 拓扑发现。 |
| `switchSubscription.defaultGrpcPort` | Switch CR 未设置 `spec.grpcPort` 时使用的 switch-agent gRPC 端口，默认 `8090`。 |
| `switchSubscription.ignorePortPatterns` | 可选：拓扑计算前忽略的交换机本地端口 glob，默认 `mgmt*`、`Management*`、`oob*`。 |
| `switchSubscription.mtls.mode` | `auto` 自动生成 pinned mTLS Secret，`existing` 使用预先创建的 Secret，`disabled` 使用明文 gRPC；默认 `auto`。 |
| `fabricNode.refreshInterval` | Agent 刷新 RDMA 网卡和 LLDP 邻居的周期，默认 `1m`。 |
| `fabricNode.initialScanDelay` | Agent 首次扫描前等待 LLDP 学习的时间，默认 `1m`。 |
| `fabricNode.scaleOutInterfaceSelector` | 可选：限制参与 scale-out 拓扑发现的 RDMA 网卡；未设置时，所有未命中 storage / scale-up selector 的 RDMA 网卡都会参与。 |
| `fabricNode.storageInterfaceSelector` | 可选：选择存储 RDMA 网卡，并从 scale-out 拓扑中排除；指标中标记为 `kind=storage`。支持 `interface=eth9` 或 `cidr=172.20.0.0/16`。 |
| `fabricNode.scaleUpInterfaceSelector` | 可选：选择 scale-up RDMA 网卡，并从 scale-out leaf 分组中排除；指标中标记为 `kind=scaleUp`。 |
| `nodeMetrics.enabled` | 可选：开启 Agent Metrics 用于节点 RDMA 可观测。 |
| `nodeMetrics.serviceMonitor.enabled` | 可选：创建 Prometheus Operator 使用的 `ServiceMonitor`。 |
| `grafanaDashboard.enabled` | 可选：下发内置 RDMA Dashboard。 |
| `topoDiscovery.scaleOut.label.keyTemplate` | Scale-out label key 模板，默认 `scale-out.unifabric.io/tier-{{ .Tier }}`。模板必须且只能包含一个 `{{ .Tier }}`。 |
| `topoDiscovery.scaleUp.label.keyTemplate` | Scale-up label key 模板，默认 `scale-up.unifabric.io/tier-{{ .Tier }}`。 |
| `topoDiscovery.storage.label.keyTemplate` | Storage label key 模板，默认 `storage.unifabric.io/tier-{{ .Tier }}`。 |

更多 Helm 参数见 [chart/README.md](../chart/README.md)。

如果您位于中国地区，可以额外增加下面的参数，加速下载：

```bash
--set global.registry=m.daocloud.io \
--set controller.image.repository=ghcr.io/unifabric-io/unifabric-controller \
--set agent.image.repository=ghcr.io/unifabric-io/unifabric-agent
```

## 选择交换机拓扑发现方式

Helm 只在 Kubernetes 中部署 controller 和 node agent，不会自动在交换机上安装
`switch-agent`。Unifabric 按 `spec.role` 分别判断 scale-out 和 storage 的发现方式，可以根据
是否需要交换机侧自动 LLDP 发现，在下面两种情况中选择。

下图作为两种发现方式的共同示例：`node1`、`node2` 通过 `leaf1`、`leaf2` 组成
`tier1-group1`，`node3`、`node4` 通过 `leaf3`、`leaf4` 组成 `tier1-group2`，四台
leaf 的共同上联 `spine1` 组成 `tier2-group1`。

![RoCE 交换机拓扑发现示例](images/sonic-roce-topology-example.jpg)

### 情况 1：仅使用节点 LLDP

这种方式不需要在交换机上安装 `switch-agent`：

1. 不创建任何同 role 的 `Switch` CR 时，Unifabric 进入 Node-only 模式。FabricNode LLDP
   中的交换机 hostname 被视为虚拟 leaf，会自动发现生成 tier 1 的性能域。
2. 如果还需要描述 spine、core 等上层拓扑，只创建这些上层 `Switch` CR，并给每个 CR
   添加 `unifabric.io/neighbors` annotation。节点发现出的 leaf 仍保持为虚拟交换机，
   annotation 用于声明上层 Switch 到虚拟 leaf 或其他上层 Switch 的连接。

半自动模式要求同 role 的所有 `Switch` CR 都存在 `unifabric.io/neighbors` key，不需要为
虚拟 leaf 创建 CR，也不需要填写 `spec.mgmtIP`。

#### 示例 1：Node-only 发现 leaf

按照上图，FabricNode LLDP 分别上报 `node1`、`node2` 连接 `leaf1`、`leaf2`，
`node3`、`node4` 连接 `leaf3`、`leaf4`。不创建任何 ScaleOut `Switch` CR 时，四个
leaf hostname 都是虚拟交换机，只生成图中的两个 tier 1 性能域；因为没有声明
`spine1`，不会生成 `tier2-group1`：

```yaml
apiVersion: unifabric.io/v1beta1
kind: Topology
metadata:
  name: scaleout
status:
  domains:
    - name: tier1-group1
      tier: 1
    - name: tier1-group2
      tier: 1
  nodes:
    - domainPath:
        - tier1-group1
      nodes:
        - node1
        - node2
    - domainPath:
        - tier1-group2
      nodes:
        - node3
        - node4
```

此时四个 Node 只获得各自的 `scale-out.unifabric.io/tier-1` label。两个性能域的
`members` 都为空，因为虚拟 leaf 不对应真实的 `Switch` CR。

#### 示例 2：使用 annotation 补充 spine1

要得到图中的 `tier2-group1`，只创建上层 `spine1` 的 `Switch` CR，并在 annotation
中引用 FabricNode LLDP 上报的四个 leaf hostname：

```yaml
apiVersion: unifabric.io/v1beta1
kind: Switch
metadata:
  name: spine1
  annotations:
    unifabric.io/neighbors: '["leaf1", "leaf2", "leaf3", "leaf4"]'
spec:
  role: ScaleOut
```

不要创建 `leaf1` 到 `leaf4` 的 `Switch` CR，也不需要为 `spine1` 填写
`spec.mgmtIP`。应用后，四个 Node 会同时获得 tier 1 和 tier 2 label：

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
    - name: tier1-group1
      parent: tier2-group1
      tier: 1
    - name: tier1-group2
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

### 情况 2：使用 switch-agent 自动发现

需要从交换机 LLDP 自动发现 leaf、spine、core 的完整连接时，为每台参与计算的物理交换机
安装 `switch-agent`，并创建对应的 `Switch` CR。只要同 role 的任意一个 `Switch` CR
缺少 `unifabric.io/neighbors` key，就进入全自动模式：

- 所有实际参与的 leaf、spine、core 都必须存在真实的 `Switch` CR。
- Controller 使用 switch-agent 上报的 LLDP 快照。
- 所有 `unifabric.io/neighbors` annotation 都会被忽略。建议全自动模式下不要给任何
  Switch 添加该 annotation，避免配置含义不清。

按照上图，需要在 `leaf1` 到 `leaf4` 和 `spine1` 上分别部署 switch-agent，并创建五个
同名的 ScaleOut `Switch` CR。节点 LLDP 提供 Node 到 leaf 的连接，switch-agent LLDP
提供 leaf 到 spine 的连接。发现完成后，三个性能域的真实 Switch members 分别为：

- `tier1-group1`：`leaf1`、`leaf2`
- `tier1-group2`：`leaf3`、`leaf4`
- `tier2-group1`：`spine1`

下面的安装和接入步骤仅适用于情况 2。

需要向多台交换机批量分发证书和部署容器时，也可以使用项目提供的
[switch-agent 自动化部署脚本](./usage/deploy-switch-agent-script.zh.md)。脚本会在容器已存在时
删除并重建，不存在时直接部署；目标地址和认证信息由运行时传入。脚本默认使用 socket
挂载，也可以通过环境变量切换到 `hostProc`。

## 安装 switch-agent 和 Switch 资源

运行前需要确认下面几个条件：

- 交换机管理网可以被集群内 Unifabric controller 访问。
- 交换机可以运行 `switch-agent` 容器，并且可以拉取或预先导入对应镜像。
  如果 Docker 不可用，改用 [systemd](./usage/switch-agent-systemd.zh.md) 安装 release 二进制。
- 交换机已经开启 LLDP，且本机使用 `lldpcli show neighbors -f json0` 能看到预期邻居。
  -  `socket` 模式只要求容器能够挂载并访问 `/run/lldpd.socket`。
  - 如果交换机无法挂载或访问 `/run/lldpd.socket`，可以改用 `hostProc` 模式，该模式需要 privileged 权限，并挂载宿主机 `/proc`。


需要关注的影响面如下：

- `switch-agent` 会通过容器端口映射在交换机管理网暴露 gRPC 端口，默认是 `8090`，默认使用 pinned mTLS 加密。

### 导出 switch-agent pinned mTLS 证书

下面的证书导出命令在 Kubernetes 控制节点，或其他已经配置好 `kubectl` 集群访问权限的管理机上执行。

```bash
mkdir -p ./tmp-switch-mtls

kubectl -n unifabric-system get secret switch-controller-mtls-agent -o jsonpath='{.data.tls\.crt}' | base64 -d > ./tmp-switch-mtls/tls.crt
kubectl -n unifabric-system get secret switch-controller-mtls-agent -o jsonpath='{.data.tls\.key}' | base64 -d > ./tmp-switch-mtls/tls.key
kubectl -n unifabric-system get secret switch-controller-mtls-agent -o jsonpath='{.data.peer\.crt}' | base64 -d > ./tmp-switch-mtls/peer.crt
```

导出后，登录到目标交换机，在交换机上准备目录：

```bash
sudo mkdir -p /opt/unifabric-switch-agent/mtls
```

然后把 `tls.crt`、`tls.key` 和 `peer.crt` 复制到交换机的
`/opt/unifabric-switch-agent/mtls/`。

### 在交换机上启动 switch-agent

默认情况下，switch-agent 优先通过挂载出来的 `lldpd` socket 读取 LLDP，并使用镜像内置的 `lldpcli` `1.0.16`。下面的启动方式使用 Docker bridge 网络，通过 `-p 8090:8090` 把 gRPC 端口发布到交换机管理 IP，不需要 host network、host UTS 或 privileged 权限。

```bash
export SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:${LATEST_TAG}"
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

常用环境变量如下：

| 环境变量 | 默认值 | 含义 |
| --- | --- | --- |
| `UNIFABRIC_SWITCH_AGENT_LOG_LEVEL` | `info` | 日志级别，支持 `debug`、`info`、`warn`、`error`。 |
| `UNIFABRIC_SWITCH_AGENT_SWITCH_NAME` | `$hostname` | 交换机在 LLDP 快照里上报的本机名。 |
| `UNIFABRIC_SWITCH_AGENT_LISTEN_ADDRESS` | `:8090` | switch-agent 对外监听的 gRPC 地址。 |
| `UNIFABRIC_SWITCH_AGENT_MTLS_ENABLED` | `true` | 是否启用 pinned mTLS。必须与 controller 的 mTLS 配置一致。 |
| `UNIFABRIC_SWITCH_AGENT_MTLS_CERT_FILE` | `/etc/unifabric/switch-mtls/tls.crt` | switch-agent 证书路径。 |
| `UNIFABRIC_SWITCH_AGENT_MTLS_KEY_FILE` | `/etc/unifabric/switch-mtls/tls.key` | switch-agent 私钥路径。 |
| `UNIFABRIC_SWITCH_AGENT_MTLS_PEER_CERT_FILE` | `/etc/unifabric/switch-mtls/peer.crt` | 用于固定 controller 身份的对端证书路径。 |
| `UNIFABRIC_SWITCH_AGENT_LLDP_REFRESH_INTERVAL` | `10s` | 本地 LLDP 快照刷新周期。 |
| `UNIFABRIC_SWITCH_AGENT_LLDP_COLLECTION_MODE` | `socket` | LLDP 采集方式。默认 `socket` 。如果交换机无法挂载 `lldpd` socket，可以改用宿主机 `/proc` 命名空间方式，见 [switch-agent hostProc LLDP 采集方式](./usage/switch-agent-host-proc.zh.md)。 |
| `UNIFABRIC_SWITCH_AGENT_LLDP_SOCKET_PATH` | `/run/lldpd.socket` | `socket` 模式使用的 lldpd Unix socket 路径。 |
| `UNIFABRIC_SWITCH_AGENT_LLDP_CLI_VERSION` | `1.0.16` | `socket` 模式下使用的镜像内 CLI 版本。SONiC 202006 到 202311 填 `1.0.4`；其他版本保持默认 `1.0.16`。 |

启动后可以先检查容器是否处于运行状态，是否有报错日志：

```bash
docker ps | grep unifabric-switch-agent
docker logs --tail 100 unifabric-switch-agent
```

### 创建 Switch 资源

等交换机上的 switch-agent 都启动后，再为每台交换机各自创建一份 `Switch` YAML。Unifabric Controller 会按 `spec.mgmtIP` 建立连接读取 LLDP 信息。

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

`spec` 需要关注的字段如下：

- `mgmtIP`：连接 switch-agent 时必填；仅通过 label 补全拓扑成员的 Switch 可以不填写。
- `role`：可选，用于标识该交换机属于 scale-out、scale-up 还是 storage 网络。支持 `ScaleOut`、`ScaleUp`、`Storage`，不填写时默认是 `ScaleOut`。
- `grpcPort`：可选，switch-agent 的 gRPC 端口，默认是 `8090`。

`role` 可以按交换机连接的网卡和网络用途判断：

- `ScaleOut`：交换机连接的是主机侧用于跨节点 GPU 训练或业务通信的 RDMA 网卡，这些网卡会参与 scale-out leaf / spine / core 拓扑计算。
- `ScaleUp`：交换机连接的是 GPU 侧直出的 scale-up 网卡，用于同一个 scale-up 域内的 GPU 间通信，不参与 scale-out 拓扑计算。
- `Storage`：交换机连接的是主机侧用于存储访问的网卡，这些网卡应与 `fabricNode.storageInterfaceSelector` 选择的 storage NIC 对应。

准备好后，再 Kubernetes 控制节点执行 `kubectl apply -f <switch>.yaml` 下发交换机 CR。

然后你可以通过 `kubectl get switch` 查看邻居信息是否同步，你可以在下面验证部署的章节看到输出示例。

## 验证部署

等待 controller 和 agent 就绪：

```bash
kubectl -n unifabric-system get pods
kubectl -n unifabric-system rollout status deployment/unifabric-controller
kubectl -n unifabric-system rollout status daemonset/unifabric-agent
```

查看 `FabricNode`：

```bash
kubectl get fabricnodes
kubectl get fabricnode <node-name> -o yaml
```

重点检查：

- `status.scaleOutNics` 是否包含预期的 scale-out RDMA 网卡。
- `status.storageNics` 是否只包含存储网络接口。
- `status.scaleOutNics[*].lldpNeighbor.hostname` 是否存在。
- `status.conditions` 中 `Ready` 和 `LLDPNeighborsReady` 是否为 `True`。

查看交换机状态和 Node label：

```bash
kubectl get switches -o wide
kubectl get switch <switch-name> -o yaml
kubectl get nodes -L scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2,scale-out.unifabric.io/tier-3,kubernetes.io/hostname
```

例如，在当前实验环境中可以看到类似输出：

```bash
$ kubectl get switch
NAME     MGMTIP            ROLE       HEALTHY   NEIGHBORS
leaf1    192.168.122.72    ScaleOut   true      2
leaf2    192.168.122.80    ScaleOut   true      2
spine1   192.168.122.163   ScaleOut   true      2
```

重点检查：

- `Switch.status.healthy` 和 conditions 是否符合当前连接状态。该字段仅用于健康状态展示，不过滤已有拓扑数据。
- `Switch.status.lldpNeighborCount` 是否大于 `0`。
- 节点上是否出现连续的 `scale-out.unifabric.io/tier-N` 标签，tier1 最靠近 Node。

配置 Kueue、Volcano 或 KAI Scheduler 时，应只使用上述命令中已经真实写到 Node 上的 label。
如果当前网络拓扑只有 leaf 层，或集群中没有任何 Switch CR 而进入 Node-only 模式，那么只有 tier1 label 是正常的。

验证 RDMA metrics 资源：

```bash
kubectl -n unifabric-system get service unifabric-agent-metrics
kubectl -n unifabric-system get servicemonitor unifabric-agent-metrics
```

直接检查 Agent metrics 端点：

```bash
POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-agent -o jsonpath='{.items[0].status.podIP}')
curl -s "http://${POD_IP}:8082/metrics" | grep '^unifabric_'
```

检查 RDMA metrics 中的网卡分类：

```bash
curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="scaleOut"'
```

### 查看 Topology CR

Controller 只会为已经生成有效 Domain 和 Node 分组的来源创建集群级 Topology CR。
Topology 没有 `spec`，`status` 是从 Node 和 Switch labels 汇总出的只读视图：

```bash
kubectl get topologies
```

例如，同时发现 scale-out 和 storage 拓扑时，可以看到：

```text
NAME       AGE
scaleout   34s
storage    34s
```

没有 `scaleup` 表示当前没有生成有效的 scale-up Domain，并非部署异常。

查看 scale-out 拓扑：

```bash
kubectl get topology scaleout -o yaml
```

下面的实际输出包含一个由 `leaf01` 到 `leaf04` 组成的 tier 1 Domain，以及一个由
`spine01`、`spine02` 组成的 tier 2 Domain：

```yaml
apiVersion: unifabric.io/v1beta1
kind: Topology
metadata:
  name: scaleout
status:
  domains:
    - members:
        - spine01
        - spine02
      name: tier2-group1
      tier: 2
    - members:
        - leaf01
        - leaf02
        - leaf03
        - leaf04
      name: tier1-group1
      parent: tier2-group1
      tier: 1
  nodes:
    - domainPath:
        - tier2-group1
        - tier1-group1
      nodes:
        - node1
        - node2
        - node3
        - node4
```

查看 storage 拓扑：

```bash
kubectl get topology storage -o yaml
```

该环境中的 storage 网络只有一层，因此只生成 tier 1 Domain：

```yaml
apiVersion: unifabric.io/v1beta1
kind: Topology
metadata:
  name: storage
status:
  domains:
    - members:
        - storagesw
      name: tier1-group1
      tier: 1
  nodes:
    - domainPath:
        - tier1-group1
      nodes:
        - node1
        - node2
        - node3
        - node4
```

`domains[*].members` 是该性能域中的 Switch CR 名称，`nodes[*].domainPath` 按最高 tier 到 tier1 排列。Node-only 模式下只包含 tier1，且因为没有 Switch CR，`members` 为空或不显示。

## 常见问题

### `FabricNode` 没有 scale-out NIC

- 确认节点上能在 `/sys/class/infiniband` 下看到 RDMA 设备。
- 如果显式设置了 `fabricNode.scaleOutInterfaceSelector`，确认它匹配实际接口名或 CIDR。
- 确认没有把 scale-out 接口误匹配为 storage 或 scale-up；这些接口会从 scale-out 分组中排除。

### FabricNode 状态 `LLDPNeighborsReady=False`

- 确认交换机已开启 LLDP。
- 确认节点侧 `lldpd` 正常工作，Agent Pod 内可以读取 LLDP 信息。
- 确认 `fabricNode.initialScanDelay` 足够长，避免 Agent 首次扫描早于 LLDP 学习完成。

### 没有 Node label

- 确认 `topoDiscovery.scaleOut.mode=unifabric-roce`。
- 确认 `FabricNode.status.nodeRole` 不是 `Storage`。
- 确认至少一个 `scaleOutNics` 同时满足 `state=up` 且有 `lldpNeighbor.hostname`。
- 如果集群中没有任何 Switch CR，确认 FabricNode LLDP hostname 能形成预期的 tier1 分组。
- 全自动模式下，确认 FabricNode LLDP hostname 能匹配 Switch CR name 或 `Switch.status.hostname`，并确认 Switch status 中已有 LLDP 邻居。
- 半自动模式下，确认同 role 的每个 Switch 都存在 annotation key，邻居名称能解析到节点发现出的 leaf 或其他带 annotation 的 Switch CR。
- 查看 controller 日志：

  ```bash
  kubectl -n unifabric-system logs deployment/unifabric-controller
  ```

- 查看交换机上的 switch-agent 日志：

  ```bash
  docker logs --tail 100 unifabric-switch-agent
  ```

### RDMA metrics 没有采集到 RoCE 网卡

- 确认 Agent Pod 正常运行，并且节点上能在 `/sys/class/infiniband` 下看到 RDMA 设备。
- 确认 `nodeMetrics.enabled=true`。
- 如果使用 Prometheus Operator，确认 `nodeMetrics.serviceMonitor.enabled=true` 且
  `ServiceMonitor` 能被 Prometheus selector 选中。
- 先直接访问 Agent metrics 端点，确认 `unifabric_` 指标存在，再排查 Prometheus target discovery。

## 卸载

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

如不再需要 CRD，可手动删除：

```bash
kubectl delete crd fabricnodes.unifabric.io switches.unifabric.io topologies.unifabric.io
```

## 下一步

- 返回 [文档索引](./README.zh.md)。
- 阅读 [Kueue TAS 工作负载示例](./usage/workload-tas.zh.md)。
- 查看 [Helm values 参考](../chart/README.md)。
