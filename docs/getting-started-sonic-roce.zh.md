# 通用 SONiC RoCE

本文说明如何在 SONiC 交换机承载 RoCE 网络的集群中部署 Unifabric。该场景通过节点 RDMA
网卡、FabricNode LLDP 邻居信息和交换机侧 switch-agent 的 LLDP 快照发现 scale-out
leaf、spine、core 拓扑。

## 部署目标

完成部署后，集群中应达成以下目标：

- Node 被写入可供调度系统消费的拓扑 label，默认包括
  `unifabric.io/scale-out-leaf`、`unifabric.io/scale-out-spine` 和
  `unifabric.io/scale-out-core`。
- 节点 RDMA 状态可观测，能够通过 Unifabric Agent metrics 查看 RDMA device、port、
  priority 和 Pod 归属等指标。
- 可以通过 `FabricNode` 和 `Switch` CR 查看输入状态；调度侧消费写回到 Node 上的拓扑 label。

> 默认 hash 命名模式下，Node label 值会带
> `leaf-`、`spine-`、`core-` 前缀，例如 `leaf-0a42746`。

## 前置条件

- 可以访问的 Kubernetes 集群。
- 已安装 `kubectl` 和 Helm 3。
- 交换机需要具备 Docker 或其他容器运行环境。

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
  --set scaleOutDiscovery.switches.enabled=true \
  --set nodeMetrics.enabled=true \
  --set nodeMetrics.serviceMonitor.enabled=true \
  --set grafanaDashboard.enabled=true \
  --wait
```

参数说明：

| Helm value | 用途 |
| --- | --- |
| `scaleOutDiscovery.switches.enabled` | 开启基于交换机邻居的拓扑发现。 |
| `scaleOutDiscovery.switches.ignoreSwitchPorts` | 可选：由 controller 在拓扑计算前忽略交换机本地端口，默认 `mgmt*`、`Management*`、`oob*`。 |
| `nodeTopologyDiscovery.scaleOutInterfaceSelector` | 可选：限制参与 scale-out 拓扑发现的 RDMA 网卡；未设置时，所有未命中 storage / scale-up selector 的 RDMA 网卡都会参与。 |
| `nodeTopologyDiscovery.storageInterfaceSelector` | 可选：选择存储 RDMA 网卡，并从 scale-out 拓扑中排除；指标中标记为 `kind=storage`。支持 `interface=eth9` 或 `cidr=172.20.0.0/16`。 |
| `nodeTopologyDiscovery.scaleUpInterfaceSelector` | 可选：选择 scale-up RDMA 网卡，并从 scale-out leaf 分组中排除；指标中标记为 `kind=scaleUp`。 |
| `nodeMetrics.enabled` | 可选：开启 Agent Metrics 用于节点 RDMA 可观测。 |
| `nodeMetrics.serviceMonitor.enabled` | 可选：创建 Prometheus Operator 使用的 `ServiceMonitor`。 |
| `grafanaDashboard.enabled` | 可选：下发内置 RDMA Dashboard。 |
| `topologyLabels.scaleOutLeaf` | leaf Node label key，默认 `unifabric.io/scale-out-leaf`。 |
| `topologyLabels.scaleOutSpine` | spine Node label key，默认 `unifabric.io/scale-out-spine`。 |
| `topologyLabels.scaleOutCore` | core Node label key，默认 `unifabric.io/scale-out-core`。 |

更多 Helm 参数见 [chart/README.md](../chart/README.md)。

如果您位于中国地区，可以额外增加下面的参数，加速下载：

```bash
--set global.registry=m.daocloud.io \
--set controller.image.repository=ghcr.io/unifabric-io/unifabric-controller \
--set agent.image.repository=ghcr.io/unifabric-io/unifabric-agent \
--set agent.lldp.image.repository=ghcr.io/unifabric-io/unifabric-agent \
```

## 在交换机中安装 switch-agent 和 Switch 资源

部署交换机侧 `switch-agent` 的目的，是采集交换机本地的 LLDP 邻居关系，并同步给集群中的 Unifabric controller，用于增强 scale-out 拓扑识别。

如果不部署 `switch-agent`，Unifabric 仍可以通过节点侧 `FabricNode` LLDP 邻居信息识别节点连接到的 leaf；但 leaf 到 spine、core 的上联关系只在交换机侧可见，因此无法可靠识别 spine 和 core 分层。对于较大的多交换机集群，仅依赖节点侧 LLDP 可能导致拓扑分组不完整或不够准确；对于单台交换机或单层 leaf 的小集群，leaf 识别通常已经足够。

部署 `switch-agent` 后，交换机侧 LLDP 快照可以补齐 leaf、spine、core 之间的上联关系。controller 会基于这些关系识别完整的 scale-out 网络分层，并将拓扑 label 写回 Kubernetes Node。

Helm 安装只会部署 Kubernetes 里的 controller 和 node agent，不会自动把 `switch-agent` 安装到交换机上。需要纳入 scale-out 拓扑计算的每台 leaf、spine、core 交换机，都需要单独运行一份 `switch-agent`，并在集群里创建对应的 `Switch` 资源。

运行前需要确认下面几个条件：

- 交换机管理网可以被集群内 Unifabric controller 访问。
- 交换机可以运行 `switch-agent` 容器，并且可以拉取或预先导入对应镜像。
- 交换机已经开启 LLDP，且本机使用 `lldpcli show neighbors -f json0` 能看到预期邻居。
  -  `socket` 模式只要求容器能够挂载并访问 `/run/lldpd.socket`。
  - 如果交换机无法挂载或访问 `/run/lldpd.socket`，可以改用 `hostProc` 模式，该模式需要 privileged 权限，并挂载宿主机 `/proc`。


需要关注的影响面如下：

- `switch-agent` 会通过容器端口映射在交换机管理网暴露 gRPC 端口，默认是 `8090`，默认使用 pinned mTLS 加密。

下面是实际接入步骤。

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
| `UNIFABRIC_SWITCH_AGENT_SWITCH_NAME` | `$hostname` | 交换机在 LLDP 快照里上报的本机名。 |
| `UNIFABRIC_SWITCH_AGENT_LISTEN_ADDRESS` | `:8090` | switch-agent 对外监听的 gRPC 地址。 |
| `UNIFABRIC_SWITCH_AGENT_LLDP_REFRESH_INTERVAL` | `10s` | 本地 LLDP 快照刷新周期。 |
| `UNIFABRIC_SWITCH_AGENT_LLDP_COLLECTION_MODE` | `socket` | LLDP 采集方式。默认 `socket` 。如果交换机无法挂载 `lldpd` socket，可以改用宿主机 `/proc` 命名空间方式，见 [switch-agent hostProc LLDP 采集方式](./usage/switch-agent-host-proc.zh.md)。 |
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
  mgmtIP: <leaf1-mgmt-ip>
  role: ScaleOut
  grpcPort: 8090
```

`spec` 需要关注的字段如下：

- `mgmtIP`：必填，controller 连接 switch-agent 时使用的管理地址。
- `role`：可选，用于标识该交换机属于 scale-out、scale-up 还是 storage 网络。支持 `ScaleOut`、`ScaleUp`、`Storage`，不填写时默认是 `ScaleOut`。
- `grpcPort`：可选，switch-agent 的 gRPC 端口，默认是 `8090`。

`role` 可以按交换机连接的网卡和网络用途判断：

- `ScaleOut`：交换机连接的是主机侧用于跨节点 GPU 训练或业务通信的 RDMA 网卡，这些网卡会参与 scale-out leaf / spine / core 拓扑计算。
- `ScaleUp`：交换机连接的是 GPU 侧直出的 scale-up 网卡，用于同一个 scale-up 域内的 GPU 间通信，不参与 scale-out 拓扑计算。
- `Storage`：交换机连接的是主机侧用于存储访问的网卡，这些网卡应与 `nodeTopologyDiscovery.storageInterfaceSelector` 选择的 storage NIC 对应。

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
kubectl get nodes -L unifabric.io/scale-out-leaf,unifabric.io/scale-out-spine,unifabric.io/scale-out-core,kubernetes.io/hostname
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

- `Switch.status.healthy` 是否为 `true`。
- `Switch.status.lldpNeighborCount` 是否大于 `0`。
- 节点上是否出现 `unifabric.io/scale-out-leaf`、`unifabric.io/scale-out-spine`、`unifabric.io/scale-out-core` 这些标签。

配置 Kueue、Volcano 或 KAI Scheduler 时，应只使用上述命令中已经真实写到 Node 上的 label。
如果当前网络拓扑只有 leaf 层，没有上层交换机，那么 spine/core label 为空是正常的。

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

## 常见问题

### `FabricNode` 没有 scale-out NIC

- 确认节点上能在 `/sys/class/infiniband` 下看到 RDMA 设备。
- 如果显式设置了 `nodeTopologyDiscovery.scaleOutInterfaceSelector`，确认它匹配实际接口名或 CIDR。
- 确认没有把 scale-out 接口误匹配为 storage 或 scale-up；这些接口会从 scale-out 分组中排除。

### `LLDPNeighborsReady=False`

- 确认交换机已开启 LLDP。
- 确认节点侧 `lldpd` 正常工作，Agent Pod 内可以读取 LLDP 信息。
- 确认 `nodeTopologyDiscovery.initialScanDelay` 足够长，避免 Agent 首次扫描早于 LLDP 学习完成。

### 没有 Node label

- 确认 `scaleOutDiscovery.switches.enabled=true`。
- 确认 `FabricNode.status.nodeRole` 不是 `Storage`。
- 确认至少一个 `scaleOutNics` 同时满足 `state=up` 且有 `lldpNeighbor.hostname`。
- 确认 `Switch.status.healthy=true`，并且 `Switch.status.lldpNeighbors` 中已经有邻居数据。
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
kubectl delete crd fabricnodes.unifabric.io switches.unifabric.io
```

## 下一步

- 返回 [文档索引](./README.zh.md)。
- 阅读 [Kueue TAS 工作负载示例](./usage/workload-tas.zh.md)。
- 查看 [Helm values 参考](../chart/README.md)。
