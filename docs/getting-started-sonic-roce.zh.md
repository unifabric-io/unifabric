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
- 可以通过 `FabricNode`、`Switch` 和 `SwitchGroup` CR 查询相应拓扑。

> 默认 hash 命名模式下，`SwitchGroup` 对象名和 Node label 值会带
> `leaf-`、`spine-`、`core-` 前缀，例如 `leaf-0a42746`。

## 前置条件

- Kubernetes 集群，包含 Linux worker 节点。
- 已安装 `kubectl` 和 Helm 3。
- 节点上存在 RDMA-capable network interfaces，并能在 `/sys/class/infiniband` 下看到。
- 交换机和节点侧 LLDP 可用，Agent 会读取 LLDP 邻居信息。
- Kubernetes Node 上的 Unifabric Agent 需要 privileged 权限，用于访问主机网络、RDMA 设备、`/proc` 和 container runtime 状态。
- 交换机上需要能够运行 switch-agent 容器，并允许使用 `--network host`、`--uts host`、`--privileged` 和宿主机 `/proc` 挂载。
- 集群需要安装 Prometheus Operator 和 Grafana Operator，如果未安装，请在安装 Unifabric 时候取消下发 ServiceMonitor
  和 GrafanaDashboard，避免下发 CRD 失败。

确认当前集群连接：

```bash
kubectl cluster-info
kubectl get nodes -o wide
```

## 在集群中安装 Unifabric

以下命令使用最新的 release 版本，启用 `Switch` / `SwitchGroup` 拓扑路径。未显式设置
`nodeTopologyDiscovery.scaleOutInterfaceSelector` 时，默认所有
未命中 storage / scale-up selector 的 RDMA 网卡都会参与 scale-out 拓扑发现。

```bash
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unifabric-io/unifabric/releases/latest | grep '"tag_name":' | cut -d '"' -f4)
CHART_VERSION="${LATEST_TAG#v}"

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
| `nvidiaTopograph.enable` | SONiC RoCE / LLDP 场景保持 `false`，拓扑由 Unifabric Agent / Controller 发现。 |
| `scaleOutDiscovery.switches.enabled` | 开启基于 `Switch` / `SwitchGroup` 的交换机拓扑发现。 |
| `scaleOutDiscovery.switches.ignoreSwitchPorts` | 可选：由 controller 在拓扑计算前忽略交换机本地端口，默认 `mgmt*`、`Management*`、`oob*`。 |
| `nodeTopologyDiscovery.scaleOutInterfaceSelector` | 可选：限制参与 scale-out 拓扑发现的 RDMA 网卡；未设置时，所有未命中 storage / scale-up selector 的 RDMA 网卡都会参与。 |
| `nodeTopologyDiscovery.storageInterfaceSelector` | 可选：选择存储 RDMA 网卡，并从 scale-out 拓扑中排除；指标中标记为 `kind=storage`。支持 `interface=eth9` 或 `cidr=172.20.0.0/16`。 |
| `nodeTopologyDiscovery.scaleUpInterfaceSelector` | 可选：选择 scale-up RDMA 网卡，并从 scale-out leaf 分组中排除；指标中标记为 `kind=scaleUp`。 |
| `nodeMetrics.enabled` | 开启 Agent Metrics 用于节点 RDMA 可观测。 |
| `nodeMetrics.serviceMonitor.enabled` | 创建 Prometheus Operator 使用的 `ServiceMonitor`。 |
| `grafanaDashboard.enabled` | 渲染内置 RDMA Dashboard。 |
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

在交换机上部署 switch-agent 的目的，是把交换机本地看到的 LLDP 邻居关系同步给集群中的 controller。节点侧 agent 只能看到节点连接到了哪台 leaf 交换机，而交换机侧 switch-agent 可以进一步补齐 leaf 上联到哪些 spine、spine 再连接到哪些 core 这部分链路信息。

有了这些交换机侧 LLDP 快照后，controller 才能识别整个 scale-out 网络里的 leaf、spine、core 分层，生成 `Switch` / `SwitchGroup` 拓扑视图，并把对应的拓扑 label 写回到 Kubernetes Node。

Helm 安装只会部署 Kubernetes 里的 controller 和 node agent，不会自动把 switch-agent 安装到交换机上。要启用这条交换机拓扑发现链路，还需要在交换机侧额外完成下面几步。

### 导出 switch-agent pinned mTLS 证书

```bash
mkdir -p ./tmp-switch-mtls

kubectl -n unifabric-system get secret switch-controller-mtls-agent -o jsonpath='{.data.tls\.crt}' | base64 -d > ./tmp-switch-mtls/tls.crt
kubectl -n unifabric-system get secret switch-controller-mtls-agent -o jsonpath='{.data.tls\.key}' | base64 -d > ./tmp-switch-mtls/tls.key
kubectl -n unifabric-system get secret switch-controller-mtls-agent -o jsonpath='{.data.peer\.crt}' | base64 -d > ./tmp-switch-mtls/peer.crt
```

导出后，先在交换机上准备目录：

```bash
sudo mkdir -p /opt/unifabric-switch-agent/mtls
```

然后把 `tls.crt`、`tls.key` 和 `peer.crt` 复制到交换机的
`/opt/unifabric-switch-agent/mtls/`。

### 在交换机上启动 switch-agent

默认情况下不需要创建 `switch-agent.yaml`。下面直接使用内建默认值启动 switch-agent。`SWITCH_AGENT_IMAGE` 需要替换成与你当前 controller 兼容的 switch-agent 镜像：

```bash
export SWITCH_AGENT_IMAGE="<your-switch-agent-image>"

docker pull "${SWITCH_AGENT_IMAGE}"

docker rm -f unifabric-switch-agent 2>/dev/null || true

docker run -d \
  --name unifabric-switch-agent \
  --restart unless-stopped \
  --network host \
  --uts host \
  --privileged \
  -v /proc:/host/proc:ro \
  -v /opt/unifabric-switch-agent/mtls:/etc/unifabric/switch-mtls:ro \
  "${SWITCH_AGENT_IMAGE}" \
  /usr/bin/unifabric/switch-agent
```

如果启动时没有显式传入 `-config`，并且当前目录下也不存在 `config.yaml`，switch-agent 会自动使用以下内建默认值：

| 配置项 | 默认值 | 含义 |
| --- | --- | --- |
| `switchName` | 自动读取交换机 hostname | 交换机在 LLDP 快照里上报的本机名。 |
| `listenAddress` | `:8090` | switch-agent 对外监听的 gRPC 地址。 |
| `mtls.enabled` | `true` | 是否启用 pinned mTLS。 |
| `mtls.certFile` | `/etc/unifabric/switch-mtls/tls.crt` | switch-agent 服务端证书路径。 |
| `mtls.keyFile` | `/etc/unifabric/switch-mtls/tls.key` | switch-agent 服务端私钥路径。 |
| `mtls.peerCertFile` | `/etc/unifabric/switch-mtls/peer.crt` | 用于校验 controller 客户端证书的对端证书路径。 |
| `lldp.refreshInterval` | `10s` | 本地 LLDP 快照刷新周期。 |
| `ignoreSwitchPorts` | 由 controller 的 `scaleOutDiscovery.switches.ignoreSwitchPorts` 统一处理，不需要在 switch-agent 侧配置 | 交换机侧不做本地端口过滤，端口忽略规则由 controller 统一应用。 |

如果你想自定义配置，可以创建 `/opt/unifabric-switch-agent/config/switch-agent.yaml`，内容例如：

```yaml
logLevel: info
listenAddress: :8090
mtls:
  enabled: true
  certFile: /etc/unifabric/switch-mtls/tls.crt
  keyFile: /etc/unifabric/switch-mtls/tls.key
  peerCertFile: /etc/unifabric/switch-mtls/peer.crt
lldp:
  refreshInterval: 10s
```

然后在上面的 `docker run` 命令里额外挂载 `/opt/unifabric-switch-agent/config/switch-agent.yaml:/etc/unifabric/switch-agent.yaml:ro`，并把启动命令改成 `/usr/bin/unifabric/switch-agent -config /etc/unifabric/switch-agent.yaml`。

启动后可以先检查：

```bash
docker ps | grep unifabric-switch-agent
docker logs --tail 100 unifabric-switch-agent
```

### 创建 Switch 资源

等交换机上的 switch-agent 都启动后，再为每台交换机各自创建一份 `Switch` YAML。`metadata.name` 不需要和交换机 hostname 一致；controller 会按 `spec.mgmtIP` 建立连接，并把 switch-agent 实际上报的交换机名当作拓扑归一化别名。

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
- `role`：可选，支持 `ScaleOut`、`ScaleUp`、`Storage`，不填写时默认是 `ScaleOut`。
- `grpcPort`：可选，switch-agent 的 gRPC 端口，默认是 `8090`。

准备好后，对每台交换机分别执行 `kubectl apply -f <switch>.yaml` 即可。

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

查看交换机状态、`SwitchGroup` 和 Node label：

```bash
kubectl get switches -o wide
kubectl get switchgroups -o wide
kubectl get switch <switch-name> -o yaml
kubectl get nodes -L unifabric.io/scale-out-leaf,unifabric.io/scale-out-spine,unifabric.io/scale-out-core,kubernetes.io/hostname
```

例如，在当前实验环境中可以看到类似输出：

```bash
$ kubectl get switch
NAME     MGMTIP            HEALTHY   NEIGHBORS
leaf1    192.168.122.72    true      2
leaf2    192.168.122.80    true      2
spine1   192.168.122.163   true      2

$ kubectl get switchgroups.unifabric.io
NAME            TIER   HEALTHY   AGE
leaf-469c8f8    1      true      61m
leaf-96f1a98    1      true      61m
spine-f24a8f0   2      true      61m
```

重点检查：

- `Switch.status.healthy` 是否为 `true`。
- `Switch.status.lldpNeighborCount` 是否大于 `0`。
- `SwitchGroup` 是否已经创建。
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

### 没有 `SwitchGroup` 或 Node label

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
kubectl delete crd fabricnodes.unifabric.io switches.unifabric.io switchgroups.unifabric.io
```

## 下一步

- 返回 [文档索引](./README.zh.md)。
- 阅读 [Kueue TAS 工作负载示例](./usage/workload-tas.zh.md)。
- 查看 [Helm values 参考](../chart/README.md)。
