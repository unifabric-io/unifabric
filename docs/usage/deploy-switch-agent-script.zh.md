# 使用脚本批量部署 switch-agent

English version: [deploy-switch-agent-script.md](./deploy-switch-agent-script.md)

项目提供
[deploy-switch-agent.sh](../../tools/deploy-switch-agent/deploy-switch-agent.sh)，用于从一台
管理机向多台交换机复制 pinned mTLS 证书并部署 switch-agent 容器。脚本默认挂载
`/run/lldpd.socket` 采集 LLDP，也可以切换到 `hostProc` 模式。

脚本不会保存目标地址或密码，也不会从镜像仓库拉取镜像。它会：

1. 检查本地是否存在 `peer.crt`、`tls.crt`、`tls.key`。
2. 通过 SSH/SCP 把证书复制到每台目标交换机。
3. 使用 sudo 将证书安装到 `/opt/unifabric-switch-agent/mtls`。目录和文件默认归
   `root:root` 所有，证书权限为 `0644`，私钥权限为 `0600`。
4. 检查目标交换机本地是否已经存在指定镜像。
5. 如果 `unifabric-switch-agent` 容器存在，删除后重新创建；不存在时直接创建。
6. 继续处理其他交换机，并在最后汇总失败的目标。

脚本只部署交换机上的 agent，不创建 Kubernetes `Switch` CR。

## 前置条件

管理机需要：

- Bash、`ssh` 和 `scp`。
- 可以访问目标交换机的 SSH 端口。
- 使用密码认证时安装 `sshpass`；SSH key 认证不需要。
- 已配置 `kubectl`，用于导出 mTLS 证书。

每台交换机需要：

- Docker 已安装并可由远端用户通过 sudo 执行。
- switch-agent 镜像已经预先加载到本地 Docker。脚本不会执行 `docker pull`。
- 默认 socket 模式要求交换机宿主机存在 `/run/lldpd.socket`。
- `hostProc` 模式要求交换机本机的 `lldpd`、`lldpcli` 工作正常。
- SSH 非交互命令可以进入 Linux shell。

## 导出 mTLS 证书

在管理机执行：

```bash
mkdir -p ./tmp-switch-mtls

kubectl -n unifabric-system get secret switch-controller-mtls-agent \
  -o jsonpath='{.data.tls\.crt}' | base64 -d > ./tmp-switch-mtls/tls.crt
kubectl -n unifabric-system get secret switch-controller-mtls-agent \
  -o jsonpath='{.data.tls\.key}' | base64 -d > ./tmp-switch-mtls/tls.key
kubectl -n unifabric-system get secret switch-controller-mtls-agent \
  -o jsonpath='{.data.peer\.crt}' | base64 -d > ./tmp-switch-mtls/peer.crt

chmod 0644 ./tmp-switch-mtls/tls.crt ./tmp-switch-mtls/peer.crt
chmod 0600 ./tmp-switch-mtls/tls.key
```

## 设置目标交换机

通过 `HOSTS` 环境变量传入逗号分隔的管理 IP：

```bash
export HOSTS="192.0.2.11,192.0.2.12,192.0.2.21"
```

这些地址使用 RFC 5737 保留的文档示例网段，实际运行时替换为交换机管理 IP。

确认每台交换机已经存在准备部署的镜像：

```bash
docker image inspect ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>
```

## 选择 LLDP 采集方式

默认使用权限更小的 socket 挂载方式，不需要设置额外参数：

```bash
export LLDP_COLLECTION_MODE=socket
export LLDP_SOCKET_PATH=/run/lldpd.socket
```

该模式挂载指定 socket，并通过 `-p 8090:8090` 暴露 gRPC 端口。如果交换机不能挂载
`lldpd` socket，改用 `hostProc`：

```bash
export LLDP_COLLECTION_MODE=hostProc
```

`hostProc` 模式使用 host network、host UTS、`--privileged` 和宿主机 `/proc`。
`GRPC_PORT` 可以修改 switch-agent 的监听端口，socket 模式会同时修改 Docker 端口映射。

## 使用 SSH key 部署

SSH key 是默认认证方式：

```bash
export SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>"

SSH_USER=your-ssh-user \
HOSTS="192.0.2.11,192.0.2.12,192.0.2.21" \
CERT_SOURCE_DIR=./tmp-switch-mtls \
./tools/deploy-switch-agent/deploy-switch-agent.sh
```

SSH key 只负责登录交换机。脚本安装证书和操作 Docker 时还需要执行远端 `sudo`：

- 如果远端用户可以免密执行 sudo，不需要增加任何参数。
- 如果执行 sudo 时需要密码，增加 `SUDO_AUTH_MODE=password`。脚本启动后会提示输入一次
  sudo 密码，输入内容不会显示在终端上。

```bash
SSH_USER=your-ssh-user \
SUDO_AUTH_MODE=password \
HOSTS="192.0.2.11,192.0.2.12,192.0.2.21" \
CERT_SOURCE_DIR=./tmp-switch-mtls \
SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>" \
./tools/deploy-switch-agent/deploy-switch-agent.sh
```

## 使用 SSH 密码部署

设置 `SSH_AUTH_MODE=password`。没有提前设置 `SSH_PASSWORD` 时，脚本会在终端中安全提示
输入，不会把密码写入脚本或目标列表：

```bash
SSH_AUTH_MODE=password \
SSH_USER=your-ssh-user \
HOSTS="192.0.2.11,192.0.2.12,192.0.2.21" \
CERT_SOURCE_DIR=./tmp-switch-mtls \
SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>" \
./tools/deploy-switch-agent/deploy-switch-agent.sh
```

密码模式默认认为 SSH 密码也是 sudo 密码。两者不同时，运行前单独设置
`SUDO_PASSWORD`。

## 常用参数

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `SSH_USER` | 无 | 必填；交换机 SSH 用户。 |
| `SSH_PORT` | `22` | SSH 端口。 |
| `SSH_AUTH_MODE` | `key` | `key` 或 `password`。 |
| `SUDO_AUTH_MODE` | 根据 SSH 方式决定 | SSH key 默认 `passwordless`；SSH 密码默认 `password`。 |
| `HOSTS` | 无 | 逗号分隔的交换机管理 IP，必填。 |
| `CERT_SOURCE_DIR` | `./tmp-switch-mtls` | 本地证书目录。 |
| `SWITCH_AGENT_IMAGE` | 无 | 目标交换机中已经存在的 unifabric agent 完整镜像名 |
| `LLDP_COLLECTION_MODE` | `socket` | `socket` 或 `hostProc`。 |
| `LLDP_SOCKET_PATH` | `/run/lldpd.socket` | socket 模式挂载的宿主机 socket。 |
| `GRPC_PORT` | `8090` | switch-agent 监听端口，socket 模式同时映射该端口。 |
| `REMOTE_UPLOAD_DIR` | `/tmp/unifabric-switch-agent-<user>` | 证书临时上传目录。 |
| `REMOTE_CERT_DIR` | `/opt/unifabric-switch-agent/mtls` | 证书最终目录。 |
| `CONTAINER_NAME` | `unifabric-switch-agent` | Docker 容器名称。 |

## 部署 Switch CR

脚本成功后，仍需在 Kubernetes 中为每台物理交换机创建对应的 `Switch` CR：

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

全自动模式不要添加 `unifabric.io/neighbors` annotation。`metadata.name` 可以使用业务名称，
但 FabricNode LLDP hostname 必须能匹配 Switch CR 名称或 switch-agent 上报的
`status.hostname`。

## 验证

在交换机上检查容器：

```bash
docker ps --filter name=unifabric-switch-agent
docker logs --tail 100 unifabric-switch-agent
```

在 Kubernetes 集群中检查订阅和 LLDP 状态：

```bash
kubectl get switches -o wide
kubectl get switch <switch-name> -o yaml
```

如果日志提示找不到本地镜像，先在对应交换机导入正确版本的镜像再重新运行脚本。如果提示
sudo 失败，确认远端用户具有 Docker、证书目录相关的 root 权限，并正确设置
`SUDO_PASSWORD`。

## 权限说明

默认 socket 模式只挂载 `lldpd` socket 和 mTLS 目录，不启用 privileged、host network
或 host UTS。只有显式设置 `LLDP_COLLECTION_MODE=hostProc` 时，脚本才使用这些权限并
只读挂载宿主机 `/proc`。两种采集方式的原理和取舍见
[switch-agent hostProc LLDP 采集方式](./switch-agent-host-proc.zh.md)。
