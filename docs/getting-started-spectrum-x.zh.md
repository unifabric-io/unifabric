# Spectrum-X Fabric

本文说明如何在 Spectrum-X 交换机集群中部署 Unifabric。该场景通过 NetQ API 获取 fabric 拓扑。

## 部署目标

完成部署后，集群中应达成两个目标：

- Node 会获得供调度系统使用的 `scale-up.unifabric.io/tier-N` 和
  `scale-out.unifabric.io/tier-N` 拓扑 label。
- 节点 RDMA 状态可观测，能够通过 Unifabric Agent metrics 查看 RDMA device、port、
  priority 和 Pod 归属等指标。

## 前置条件

- Kubernetes 集群，包含目标 GPU 节点。
- 已安装 `kubectl` 和 Helm 3。
- 节点上存在 RDMA 设备，并能在 `/sys/class/infiniband` 下看到。
- Spectrum-X fabric 已经由 NetQ 管理，并且 NetQ 中已经有对应 fabric 拓扑数据。
  - 如果您没有部署 NetQ，但是网络是承载的 IB 网络，那么可以参考 [InfiniBand fabric](./getting-started-infiniband.zh.md)。
  - 如果您承载的是 RoCE 网络，那么可以参考 [General SONiC RoCE](./getting-started-sonic-roce.zh.md)。
- 集群可以访问 NetQ API。
- 已准备 NetQ API URL、用户名和密码。
- 集群需要安装 Prometheus Operator 和 Grafana Operator，如果未安装，请在安装 Unifabric 时取消下发 ServiceMonitor
  和 GrafanaDashboard，避免下发 CRD 失败。

确认当前集群连接：

```bash
kubectl cluster-info
kubectl get nodes -o wide
```

## 安装 Unifabric

以下命令使用最新的 release 版本。示例将 RDMA interface selector 留空，因此所有 RDMA
网卡都会被 metrics 观测。Spectrum-X 拓扑 label 由 NVIDIA topograph 通过 NetQ
provider 写回。

先创建 namespace 和 NetQ 凭据 Secret。下面的命令不会把凭据写入本地文件、Helm values
或 ConfigMap；Secret 必须包含名为 `credentials.yaml` 的 key：

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

生产环境还应为 Kubernetes Secret 启用静态加密，并限制读取该 Secret 的 RBAC 权限。

```bash
LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unifabric-io/unifabric/releases/latest | grep '"tag_name":' | cut -d '"' -f4)
CHART_VERSION="${LATEST_TAG#v}"

helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
  --version "${CHART_VERSION}" \
  --namespace unifabric-system \
  --create-namespace \
  --set topoDiscovery.scaleUp.mode=nv-topograph \
  --set topoDiscovery.scaleOut.mode=nv-topograph \
  --set topoDiscovery.storage.mode=unifabric-roce \
  --set nvidiaTopograph.provider.name=netq \
  --set-string nvidiaTopograph.provider.params.apiUrl=https://netq.example.com \
  --set-string nvidiaTopograph.credentialsSecretName=netq-credentials \
  --set-string fabricNode.scaleUpInterfaceSelector="" \
  --set-string fabricNode.scaleOutInterfaceSelector="" \
  --set-string fabricNode.storageInterfaceSelector="" \
  --set nodeMetrics.enabled=true \
  --set nodeMetrics.serviceMonitor.enabled=true \
  --set grafanaDashboard.enabled=true \
  --wait --debug
```

参数说明：

| Helm value | 用途 |
| --- | --- |
| `topoDiscovery.scaleUp.mode` | 设置为 `nv-topograph`，由 NVIDIA Topograph 写入 scale-up label；该设置与 scale-out mode 相互独立。 |
| `topoDiscovery.scaleOut.mode` | 设置为 `nv-topograph`，由 NVIDIA Topograph 写入 scale-out label。 |
| `topoDiscovery.storage.mode` | 设置为 `unifabric-roce`，使用内置 RoCE storage 发现。 |
| `nvidiaTopograph.provider.name` | 设置为 `netq`，表示通过 NetQ API 发现拓扑。 |
| `nvidiaTopograph.provider.params.apiUrl` | NetQ API URL。 |
| `nvidiaTopograph.credentialsSecretName` | 已有 NetQ 凭据 Secret 的名称；Secret 必须包含 `credentials.yaml` key。凭据仅以只读文件挂载到 Topograph Pod。 |
| `nodeMetrics.enabled` | 开启 Agent Metrics 用于节点 RDMA 可观测。 |
| `fabricNode.scaleUpInterfaceSelector` | 选择特定 RDMA 网卡用于观测，并在 RDMA 指标中打上 `kind=scaleUp` 标签，支持 `interface=eth*,mlx*` 或 `cidr=172.17.0.0/16`，默认为全部 RDMA 网卡。 |
| `fabricNode.storageInterfaceSelector` | 选择一组 RDMA 存储网卡观测，并在 RDMA 指标中打上 `kind=storage` 标签。支持 `interface=eth*,mlx*` 或 `cidr=172.17.0.0/16`。默认为空。|
| `nodeMetrics.serviceMonitor.enabled` | 创建 Prometheus Operator 使用的 `ServiceMonitor`。 |
| `grafanaDashboard.enabled` | 渲染内置 RDMA Dashboard。 |

更多 Helm 参数见 [chart/README.md](../chart/README.md)。

如果您位于中国地区，可以额外增加下面的参数，加速下载：

```bash
--set global.registry=m.daocloud.io \
--set controller.image.repository=ghcr.io/unifabric-io/unifabric-controller \
--set agent.image.repository=ghcr.io/unifabric-io/unifabric-agent \
--set nvidiaTopograph.image.repository=ghcr.io/nvidia/topograph
```

## 验证部署

重点检查 topograph 组件、NetQ provider 配置和 Node label：

```bash
kubectl -n unifabric-system get pods
kubectl get pods -n unifabric-system -o wide
kubectl -n unifabric-system describe secret netq-credentials
kubectl get fabricnodes.unifabric.io
kubectl get nodes -L scale-up.unifabric.io/tier-1,scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2,scale-out.unifabric.io/tier-3,kubernetes.io/hostname
kubectl get topo
```

配置 Kueue、Volcano 或 KAI Scheduler 时，应只使用上述命令中已经真实写到 Node 上的 label。

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
curl -s "http://${POD_IP}:8082/metrics" | grep 'kind="scaleUp"'
```

## 常见问题

### Node label 没有写入

- 确认 NVIDIA topograph 和 node-observer 组件正常运行，并且有权限更新 Node。
- 确认 `nvidiaTopograph.provider.name=netq`。
- 确认 `nvidiaTopograph.provider.params.apiUrl=<netq-url>` 配置正确。
- 确认 `nvidiaTopograph.credentialsSecretName` 指向安装 namespace 中的 Secret，且该
  Secret 包含 `credentials.yaml` key。
- 确认 NetQ 账号可访问目标 premises，并且 NetQ 中已经有对应 fabric 拓扑数据。
- 如果自定义了 Helm values 中的 `topoDiscovery.*.label.keyTemplate`，调度器配置中的 label key 也必须同步更新。

### RDMA metrics 没有采集到 RDMA 网卡

- 确认 Agent Pod 正常运行，并且节点上能在 `/sys/class/infiniband` 下看到 RDMA 设备。
- 确认 `fabricNode.scaleUpInterfaceSelector` 匹配节点上的 RDMA 网卡名称或 CIDR。
- 确认 `nodeMetrics.enabled=true`。
- 如果使用 Prometheus Operator，确认 `nodeMetrics.serviceMonitor.enabled=true` 且
  `ServiceMonitor` 能被 Prometheus selector 选中。

### topograph 无法访问 NetQ

- 检查 NetQ API URL、Secret 中的用户名和密码、证书及网络连通性。
- 确认 Topograph Pod 已将凭据挂载到 `/etc/topograph/credentials/credentials.yaml`。
- 查看 topograph / node-observer 日志。
- 确认 topograph Service 可达。

## 卸载

```bash
helm uninstall unifabric --namespace unifabric-system --wait
```

## 下一步

- 返回 [文档索引](./README.zh.md)。
- 阅读 [Kueue TAS 工作负载示例](./usage/workload-tas.zh.md)。
- 查看 [Helm values 参考](../chart/README.md)。
