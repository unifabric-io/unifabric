# RoCE 网络中 NVIDIA NetQ 发现

English version: [roce-netq-discovery.md](./roce-netq-discovery.md)

## 简介

Spectrum-X 是 NVIDIA 面向 AI 集群的以太网 RoCE 网络平台。NVIDIA NetQ 管理它的
fabric,并且已经掌握完整拓扑。NetQ 还包含 NVLink 管理功能(原 NMX-M)。该功能可以
提供多节点 NVLink 系统的 NVLink 域。

Unifabric 通过 NVIDIA Topograph 从 NetQ 导入拓扑。Topograph 是 NVIDIA 开源的服务。
它从厂商数据源读取网络拓扑,并写入 Node label。本文说明如何配置 RoCE 网络中 NVIDIA NetQ 发现。

| 项目 | 说明 |
| --- | --- |
| 适用网络 | Scale-out、Storage、Scale-up |
| 拓扑来源 | Topograph 通过 NetQ API 读取 fabric 拓扑和 NVLink 域。 |
| 能发现的层级 | Scale-out:由 NetQ 中的 fabric 数据决定层级,通常是 leaf、spine 及以上全部层级。Scale-up:tier 1,即 NVLink 域。 |
| 交换机侧要求 | NetQ 管理 fabric,并且 NetQ 中有它的拓扑数据。 |
| 集群侧要求 | 运行 Topograph Pod 的节点能连接 NetQ API。已准备 NetQ 凭据。 |
| Helm 配置 | `topoDiscovery.scaleOut.mode=nv-topograph`、`nvidiaTopograph.provider.name=netq` 和 NetQ 凭据。Scale-up 网络还需加上 `topoDiscovery.scaleUp.mode=nv-topograph`。 |

限制:

- 本方式不创建 `Switch` 资源。`Topology` 资源和 Node label 是完整的。但在拓扑可视化
  中,每个性能域的交换机成员卡片为空。参见
  [可选:补充 Switch 资源](#可选补充-switch-资源)。
- Unifabric 暂不支持在从 NetQ 导入的拓扑中区分 Scale-out 网络和 Storage 网络。导入
  结果只作为 Scale-out 网络。如果 Storage 网络运行在 Spectrum 交换机上,为它使用
  [RoCE 网络中 LLDP 发现](./roce-lldp-discovery.zh.md)或[自定义网络拓扑](./custom-network-topology.zh.md)。

各网络在不同网络设备下应选择哪种发现方式,参见
[网络拓扑发现](./README.zh.md#按网络选择发现方式)。

## 前提条件

- 已按[部署 Unifabric](../../getting-started.zh.md)完成部署。
- NetQ 管理 Spectrum-X fabric,并且 NetQ 中有它的 fabric 拓扑数据。
- 集群可以连接 NetQ API。确认集群和 NetQ 之间的防火墙允许这条路径。
- 已准备 NetQ API URL、用户名和密码。

## 配置

1. 创建 namespace 和 NetQ 凭据 Secret。下面的命令不会把凭据写入本地文件、Helm
   values 或 ConfigMap。Secret 必须包含名为 `credentials.yaml` 的 key:

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
   namespace/unifabric-system created
   NetQ username: admin
   NetQ password:
   secret/netq-credentials created
   ```

   在生产集群上,还要为 Kubernetes Secret 启用静态加密。同时限制能读取该 Secret 的
   RBAC 权限。

2. 设置拓扑发现参数:

   ```bash
   helm upgrade --install unifabric oci://ghcr.io/unifabric-io/charts/unifabric \
     --namespace unifabric-system \
     --reuse-values \
     --set topoDiscovery.scaleOut.mode=nv-topograph \
     --set nvidiaTopograph.provider.name=netq \
     --set-string nvidiaTopograph.provider.params.apiUrl=https://netq.example.com \
     --set-string nvidiaTopograph.credentialsSecretName=netq-credentials \
     --wait
   ```

   如果集群是多节点 NVLink 系统,并且 NetQ 管理 NVLink 域,再加上
   `--set topoDiscovery.scaleUp.mode=nv-topograph`。这样 Unifabric 还会发现 Scale-up
   网络。

   参数说明:

   | Helm value | 用途 |
   | --- | --- |
   | `topoDiscovery.scaleOut.mode` | 设置为 `nv-topograph`。之后由 NVIDIA Topograph 写入 scale-out label。 |
   | `topoDiscovery.scaleUp.mode` | 可选。设置为 `nv-topograph`。之后由 NVIDIA Topograph 写入 scale-up label。 |
   | `nvidiaTopograph.provider.name` | 设置为 `netq`。之后 Topograph 通过 NetQ API 发现拓扑。 |
   | `nvidiaTopograph.provider.params.apiUrl` | NetQ API URL。 |
   | `nvidiaTopograph.credentialsSecretName` | 已有 NetQ 凭据 Secret 的名称。Secret 必须包含 `credentials.yaml` key。Topograph 以只读方式把凭据挂载到 Topograph Pod 中。 |

## 验证

确认 Node 上出现 scale-out label:

```bash
kubectl get nodes -L scale-out.unifabric.io/tier-1,scale-out.unifabric.io/tier-2
```

如果配置了 Scale-up 网络,确认 Node 上出现 scale-up label:

```bash
kubectl get nodes -L scale-up.unifabric.io/tier-1
```

label 的含义,以及 `Topology` 资源的查看方法,参见
[网络拓扑发现](./README.zh.md#查看-topology-与-node-label)。

## 可选:补充 Switch 资源

本方式不创建 `Switch` 资源。如果需要拓扑可视化显示交换机,参见
[创建 Switch 实例](./topograph-switch-resources.zh.md)。

## 故障排查

### Node 上没有拓扑 label

1. 确认 Topograph Deployment 在运行,并查看日志:

   ```bash
   kubectl -n unifabric-system get deployment unifabric-nvidia-topograph
   kubectl -n unifabric-system logs deployment/unifabric-nvidia-topograph --tail 100
   ```

2. 确认 NetQ 凭据和 API 地址正确。确认 Topograph Pod 能连接 NetQ API。

## 参考

- [网络拓扑发现](./README.zh.md):如何为各网络选择发现方式,以及如何查看结果。
- [InfiniBand 网络发现](./infiniband-discovery.zh.md):另一种同样使用 NVIDIA Topograph
  的发现方式。
- [创建 Switch 实例](./topograph-switch-resources.zh.md):可选步骤。
- [Topograph NetQ provider](https://github.com/dsx-ai-factory/topograph/blob/main/docs/providers/netq.md):
  上游文档。
