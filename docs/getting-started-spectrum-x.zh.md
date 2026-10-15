# 在 Spectrum-X 网络部署 Unifabric

English version: [getting-started-spectrum-x.md](./getting-started-spectrum-x.md)

## 简介与部署目标

本文指导你在使用 Spectrum-X 交换机的集群中部署 Unifabric 拓扑发现。在该环境中,
NVIDIA Topograph 通过 NetQ API 读取 fabric 拓扑,Unifabric 把结果写入 Node
label。NetQ 是 NVIDIA 面向 Spectrum-X 网络的网络管理平台。

完成本文后,集群将呈现以下结果:

- Node 带有供调度器使用的 `scale-out.unifabric.io/tier-N` 拓扑 label。
- 节点 RDMA 状态可以通过 Agent metrics 观测,包括 RDMA device、port、priority
  和 Pod 归属等指标。

如果网络不符合该环境,选择其他指南:

- 如果没有部署 NetQ,且网络是 InfiniBand,阅读
  [在 InfiniBand 网络部署 Unifabric](./getting-started-infiniband.zh.md)。
- 如果网络是 SONiC 交换机承载的 RoCE,阅读
  [在通用 SONiC RoCE 网络部署 Unifabric](./getting-started-sonic-roce.zh.md)。

## 前提条件

继续之前,确认以下各项均成立:

- [ ] 已通过[基础安装](./getting-started.zh.md)完成 Unifabric 安装。
- [ ] Kubernetes 集群包含目标 GPU 节点。
- [ ] 节点上的 RDMA 设备在 `/sys/class/infiniband` 下可见。
- [ ] Spectrum-X fabric 已由 NetQ 管理,且 NetQ 中有对应的 fabric 拓扑数据。
- [ ] 集群可以访问 NetQ API。
- [ ] 已准备 NetQ API URL、用户名和密码。

确认可以访问集群:

```bash
kubectl cluster-info
kubectl get nodes -o wide
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

## 安装

先完成[基础安装](./getting-started.zh.md)。基础安装部署了所有场景共用的
Controller、Agent 和观测组件。本文只在下一节补充 NetQ 凭据和 NVIDIA Topograph
的 NetQ provider 配置。

## 环境配置

先创建 namespace 和 NetQ 凭据 Secret。下面的命令不会把凭据写入本地文件、Helm
values 或 ConfigMap。Secret 必须包含名为 `credentials.yaml` 的 key。在可以用
`kubectl` 访问集群的主机上执行:

```bash
kubectl create namespace unifabric-system --dry-run=client -o yaml | kubectl apply -f -

read -r -p "NetQ username: " NETQ_USERNAME
read -r -s -p "NetQ password: " NETQ_PASSWORD
printf '\n'
printf 'username: %s\npassword: %s\n' "${NETQ_USERNAME}" "${NETQ_PASSWORD}" |
  kubectl -n unifabric-system create secret generic netq-credentials \
    --from-file=credentials.yaml=/dev/stdin
unset NETQ_USERNAME NETQ_PASSWORD
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

生产集群还必须为 Kubernetes Secret 启用静态加密,并限制读取该 Secret 的 RBAC
权限。

然后选择 NetQ provider。该命令只修改场景相关的 values,保留基础安装中的其他
values:

```bash
helm upgrade unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --namespace unifabric-system \
  --reuse-values \
  --set topoDiscovery.scaleOut.mode=nv-topograph \
  --set nvidiaTopograph.provider.name=netq \
  --set-string nvidiaTopograph.provider.params.apiUrl=https://netq.example.com \
  --set-string nvidiaTopograph.credentialsSecretName=netq-credentials \
  --wait
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

参数说明:

| Helm value | 用途 |
| --- | --- |
| `topoDiscovery.scaleOut.mode` | 设置为 `nv-topograph`,由 NVIDIA Topograph 写入 scale-out label。 |
| `nvidiaTopograph.provider.name` | 设置为 `netq`,表示通过 NetQ API 发现拓扑。 |
| `nvidiaTopograph.provider.params.apiUrl` | NetQ API URL。 |
| `nvidiaTopograph.credentialsSecretName` | 已有 NetQ 凭据 Secret 的名称,Secret 必须包含 `credentials.yaml` key。凭据仅以只读方式挂载到 Topograph Pod。 |

## 验证

检查 topograph 组件、NetQ provider 配置和 Node label:

```bash
kubectl -n unifabric-system get pods
kubectl get pods -n unifabric-system -o wide
kubectl -n unifabric-system describe secret netq-credentials
kubectl get fabricnodes.unifabric.io
kubectl get nodes -L scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2,scale-out.unifabric.io/tier-3,kubernetes.io/hostname
kubectl get topo
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

配置 Kueue、Volcano 或 KAI Scheduler 时,只使用已经真实写到 Node 上的 label。

检查 RDMA metrics 资源:

```bash
kubectl -n unifabric-system get service unifabric-agent-metrics
kubectl -n unifabric-system get servicemonitor unifabric-agent-metrics
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

直接查询 Agent metrics 端点:

```bash
POD_IP=$(kubectl -n unifabric-system get pod -l app.kubernetes.io/component=unifabric-agent -o jsonpath='{.items[0].status.podIP}')
curl -s "http://${POD_IP}:8082/metrics" | grep '^unifabric_'
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

检查 RDMA 指标中的网卡分类:

```bash
curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="scaleOut"'
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

## 故障排查

先按[基础安装故障排查](./getting-started.zh.md#故障排查)排除 `FabricNode` 和
RDMA 指标的通用问题。

### Node label 没有写入

1. 确认 NVIDIA topograph 和 node-observer 正常运行,并且有权限更新 Node。
2. 确认 `nvidiaTopograph.provider.name=netq`。
3. 确认 `nvidiaTopograph.provider.params.apiUrl=<netq-url>` 配置正确。
4. 确认 `nvidiaTopograph.credentialsSecretName` 指向安装 namespace 中的
   Secret,且该 Secret 包含 `credentials.yaml` key。
5. 确认 NetQ 账号可以访问目标 premises,且 NetQ 中有对应的 fabric 拓扑数据。
6. 如果自定义了 `topoDiscovery.*.nodeLabel.keyTemplate`,同步更新调度器配置中
   的 label key。

### RDMA 指标不包含 RDMA 网卡

1. 确认 Agent Pod 正常运行,且节点上的 RDMA 设备在 `/sys/class/infiniband`
   下可见。
2. 确认 `fabricNode.scaleOutInterfaceSelector` 匹配 RDMA 网卡名或 CIDR。
3. 确认 `nodeMetrics.enabled=true`。
4. 如果使用 Prometheus Operator,确认 `nodeMetrics.serviceMonitor.enabled=true`,
   且 Prometheus selector 选中了该 `ServiceMonitor`。

### Topograph 无法访问 NetQ

1. 确认 NetQ API URL、Secret 中的用户名和密码、证书以及网络连通性都正确。
2. 确认 Topograph Pod 已把凭据挂载到
   `/etc/topograph/credentials/credentials.yaml`。
3. 查看 topograph 和 node-observer 日志。
4. 确认 topograph Service 可达。

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

## 下一步

- 在调度器中使用这些 label:参见 [Kueue TAS 工作负载示例](./usage/workload-tas.zh.md)。
- 渲染拓扑:参见[拓扑可视化指南](./usage/topology-visualization.zh.md)。
- 查阅字段:参见 [FabricNode 参考](./reference/fabricnode.zh.md)。
- 调整安装配置:参见 [Helm values 参考](../chart/README.md)。
