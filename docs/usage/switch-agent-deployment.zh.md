# Switch Agent 部署

English version: [switch-agent-deployment.md](./switch-agent-deployment.md)

## 简介

在 SONiC RoCE 场景中,switch-agent 运行在每台物理交换机上。它在交换机侧采集 LLDP
快照,并通过 `8090` 端口的 mTLS gRPC 提供给 Controller。本文覆盖三种部署 case:
批量部署脚本、`hostProc` 模式手动容器、systemd 原生二进制。每种 case 都只安装
agent 本身;完成任意 case 之后,必须为每台交换机创建一个 Kubernetes `Switch`
资源,Controller 才会连接该 agent。

按下表选择 case:

| Case | 适用条件 |
| --- | --- |
| Case 1:部署脚本 | 交换机运行 Docker,且需要从一台管理机批量部署多台交换机。 |
| Case 2:`hostProc` 模式手动容器 | 交换机运行 Docker,但交换机 OS 不暴露 `/run/lldpd.socket`,或 socket 权限阻止容器访问。 |
| Case 3:systemd 服务 | 交换机无法运行 Docker 或其他容器运行时。 |

> **注意:** 对于单台能正常暴露 `lldpd` socket 的交换机,
> [SONiC RoCE 指南](../getting-started-sonic-roce.zh.md)中的普通 `docker run`
> 步骤是主路径。默认的 `socket` 模式只挂载 `lldpd` socket 和 mTLS 目录,不需要
> 特权权限。`hostProc` 模式需要 `--privileged` 并挂载宿主机 `/proc`,仅在 socket
> 不可用时使用。

## 前提条件

- 已按[基础安装](../getting-started.zh.md)安装 Unifabric,且在
  [SONiC RoCE 指南](../getting-started-sonic-roce.zh.md)中选择了场景 1,
  即交换机运行 switch-agent。
- 运行 `unifabric-controller` 的 Kubernetes Node 能访问每台交换机管理 IP 的
  TCP `8090` 端口。
- 每台交换机上都运行 `lldpd`。在每台交换机上确认:

  ```bash
  lldpcli -f json0 show neighbors
  ```

  ```text
  <!-- TODO: 待真实环境采集输出 -->
  ```

- 有一台配置好 `kubectl` 上下文的主机用于导出 mTLS 证书。

## 配置与使用

### 导出 mTLS 证书

三种 case 使用相同的三个文件:来自 Helm 生成的 `switch-controller-mtls-agent`
Secret 的 `tls.crt`、`tls.key` 和 `peer.crt`。在有 `kubectl` 访问权限的主机上,
把它们导出到本地目录:

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

Case 1 通过 `CERT_SOURCE_DIR` 读取该目录,并自动安装到每台交换机。Case 2 和
Case 3 需要你自行把文件复制到每台交换机,目标目录在各 case 步骤中说明。

### Case 1:使用脚本批量部署多台交换机

项目提供
[deploy-switch-agent.sh](../../tools/deploy-switch-agent/deploy-switch-agent.sh)。
它从管理机复制已导出的 mTLS 证书,并在多台交换机上部署 switch-agent 容器。脚本
不存储目标地址和密码,也不从镜像仓库拉取镜像。启动后它自动:

1. 检查管理机上的 `peer.crt`、`tls.crt` 和 `tls.key`。
2. 通过 SSH 和 SCP 把证书复制到每个目标。
3. 用 sudo 把证书安装到 `/opt/unifabric-switch-agent/mtls`。目录和文件属主为
   `root:root`;证书权限 `0644`,私钥权限 `0600`。
4. 检查目标交换机的 Docker 中已存在指定镜像。
5. 删除并重建已存在的 `unifabric-switch-agent` 容器;不存在时直接创建。
6. 继续处理其余交换机,并在结束时汇报所有失败的目标。

管理机可以是 Kubernetes 节点,也可以是独立的 Linux 主机。它必须能访问目标交换机
的管理网络,并安装 Bash、`ssh` 和 `scp`;仅在密码认证时需要 `sshpass`。每台交换机
需要 SONiC OS 2025+ 或 NVIDIA Cumulus Linux 4.x.x+、远程用户可以通过 sudo 操作的
Docker、本地 Docker 中已预加载的 switch-agent 镜像,以及能进入 Linux shell 的
非交互式 SSH 命令。

除非步骤特别说明,本 case 的所有命令都在管理机上执行。

1. 选择 LLDP 采集模式。脚本通过环境变量把模式传给每台目标交换机上的容器。

   默认是 `socket` 模式。`LLDP_SOCKET_PATH` 是每台目标交换机宿主机上 `lldpd`
   Unix socket 的路径,不是管理机上的路径。部署前登录每台交换机,确认宿主机
   暴露了该 socket:

   ```bash
   ls -l /run/lldpd.socket
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

   权限串的第一个字符必须是 `s`,表示 Unix socket。显式设置模式和 socket 路径:

   ```bash
   export LLDP_COLLECTION_MODE=socket
   export LLDP_SOCKET_PATH=/run/lldpd.socket
   ```

   脚本先在每台交换机宿主机上检查该路径是 socket,然后把它以相同路径挂载进容器,
   并用 `-p 8090:8090` 发布 gRPC。如果某台交换机宿主机不能暴露或挂载 `lldpd`
   socket,选择 `hostProc`:

   ```bash
   export LLDP_COLLECTION_MODE=hostProc
   ```

   `hostProc` 模式下,脚本以 host network、host UTS、`--privileged` 和宿主机
   `/proc` 创建容器。`GRPC_PORT` 修改 switch-agent 监听端口;socket 模式下同时
   更新 Docker 端口映射。

2. 指定目标并运行脚本。通过 `HOSTS` 传入逗号分隔的交换机管理 IP。以下命令使用
   RFC 5737 文档保留地址段,替换为真实管理 IP。选择一种 SSH 认证方式,该选择只
   影响 SSH 登录和远程 sudo 认证。

   方式 A,SSH 密钥,为默认方式:

   ```bash
   SSH_USER=your-ssh-user \
   HOSTS="192.0.2.11,192.0.2.12,192.0.2.21" \
   CERT_SOURCE_DIR=./tmp-switch-mtls \
   SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>" \
   ./tools/deploy-switch-agent/deploy-switch-agent.sh
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

   > **注意:** SSH 密钥只认证登录。脚本仍会执行远程 `sudo` 来安装证书和操作
   > Docker。远程用户有免密 sudo 时不需要额外选项;sudo 需要密码时,加上
   > `SUDO_AUTH_MODE=password`,脚本会提示输入一次,输入在终端中隐藏。

   方式 B,SSH 密码。设置 `SSH_AUTH_MODE=password`。未预先设置 `SSH_PASSWORD`
   时,脚本在终端安全提示输入,而不是把密码存放在脚本或目标列表中:

   ```bash
   SSH_AUTH_MODE=password \
   SSH_USER=your-ssh-user \
   HOSTS="192.0.2.11,192.0.2.12,192.0.2.21" \
   CERT_SOURCE_DIR=./tmp-switch-mtls \
   SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>" \
   ./tools/deploy-switch-agent/deploy-switch-agent.sh
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

   > **注意:** 密码模式假设 SSH 密码同时是 sudo 密码。两者不同时,运行前单独
   > 设置 `SUDO_PASSWORD`。

   脚本接受以下环境变量:

   | 环境变量 | 默认值 | 说明 |
   | --- | --- | --- |
   | `SSH_USER` | 无 | 必填,交换机 SSH 用户。 |
   | `SSH_PORT` | `22` | SSH 端口。 |
   | `SSH_AUTH_MODE` | `key` | `key` 或 `password`。 |
   | `SUDO_AUTH_MODE` | 随 SSH 模式推导 | SSH 密钥时默认 `passwordless`,SSH 密码时默认 `password`。 |
   | `HOSTS` | 无 | 必填,逗号分隔的交换机管理 IP。 |
   | `CERT_SOURCE_DIR` | `./tmp-switch-mtls` | 本地证书目录。 |
   | `SWITCH_AGENT_IMAGE` | 无 | 必填,每个目标上已存在的完整镜像名。 |
   | `LLDP_COLLECTION_MODE` | `socket` | `socket` 或 `hostProc`。 |
   | `LLDP_SOCKET_PATH` | `/run/lldpd.socket` | 每台目标交换机宿主机上 `lldpd` Unix socket 的路径;socket 模式按相同路径挂载进容器。 |
   | `GRPC_PORT` | `8090` | switch-agent 监听端口;socket 模式同时映射该端口。 |
   | `REMOTE_UPLOAD_DIR` | `/tmp/unifabric-switch-agent-<user>` | 证书临时上传目录。 |
   | `REMOTE_CERT_DIR` | `/opt/unifabric-switch-agent/mtls` | 证书最终目录。 |
   | `CONTAINER_NAME` | `unifabric-switch-agent` | Docker 容器名。 |

3. 登录每台目标交换机,确认容器在运行且日志没有错误:

   ```bash
   docker ps --filter name=unifabric-switch-agent
   docker logs --tail 100 unifabric-switch-agent
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

### Case 2:以 hostProc 模式手动运行容器

`hostProc` 模式挂载宿主机 `/proc`,并用 `nsenter` 在宿主机的 mount 和 network
命名空间中运行宿主机的 `lldpcli`。`nsenter` 是在另一个进程的命名空间内执行命令
的 Linux 工具。该模式使用交换机 OS 自带的 `lldpcli` 与 `lldpd` 版本组合,而不是
switch-agent 镜像内打包的 CLI 二进制。该模式不读取
`UNIFABRIC_SWITCH_AGENT_LLDP_CLI_VERSION`。

本 case 的所有命令都在目标交换机上执行。

1. 把导出的 mTLS 文件安装到 `/opt/unifabric-switch-agent/mtls`:

   ```bash
   sudo install -d -m 0755 /opt/unifabric-switch-agent/mtls
   sudo install -m 0644 tls.crt peer.crt /opt/unifabric-switch-agent/mtls/
   sudo install -m 0600 tls.key /opt/unifabric-switch-agent/mtls/
   ```

2. 启动容器:

   ```bash
   export SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>"

   docker pull "${SWITCH_AGENT_IMAGE}"

   docker rm -f unifabric-switch-agent 2>/dev/null || true

   docker run -d \
     --name unifabric-switch-agent \
     --restart unless-stopped \
     --network host \
     --uts host \
     --privileged \
     -e UNIFABRIC_SWITCH_AGENT_LLDP_COLLECTION_MODE=hostProc \
     -v /proc:/host/proc:ro \
     -v /opt/unifabric-switch-agent/mtls:/etc/unifabric/switch-mtls:ro \
     "${SWITCH_AGENT_IMAGE}" \
     /usr/bin/unifabric/switch-agent
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

   > **警告:** 容器以 `--privileged`、host network、host UTS 和宿主机 `/proc`
   > 运行。如果交换机 OS 能稳定暴露 `/run/lldpd.socket`,改用
   > [SONiC RoCE 指南](../getting-started-sonic-roce.zh.md)中的默认 `socket`
   > 模式,其权限面更小。

3. 确认容器在运行且日志没有错误:

   ```bash
   docker ps | grep unifabric-switch-agent
   docker logs --tail 100 unifabric-switch-agent
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

### Case 3:安装为 systemd 服务

交换机无法运行 Docker 或其他容器运行时的场景使用本 case。switch-agent 以原生
Linux 二进制运行,由 systemd 管理。它直接调用交换机本地的 `lldpcli`,因此不挂载
`/run/lldpd.socket`。交换机需要安装 `lldpcli`,且 systemd 的 `PATH` 能找到它。

本 case 的所有命令都在目标交换机上执行。

1. 从 GitHub Release 页面下载并安装与交换机 CPU 架构匹配的二进制:

   ```bash
   LATEST_TAG=$(curl -fsSL https://api.github.com/repos/unifabric-io/unifabric/releases/latest | grep '"tag_name":' | cut -d '"' -f4)
   ARCH="$(uname -m)"
   case "${ARCH}" in
     x86_64) RELEASE_ARCH=amd64 ;;
     aarch64|arm64) RELEASE_ARCH=arm64 ;;
     *) echo "unsupported architecture: ${ARCH}" >&2; exit 1 ;;
   esac

   curl -fLO "https://github.com/unifabric-io/unifabric/releases/download/${LATEST_TAG}/unifabric-switch-agent_${LATEST_TAG}_linux_${RELEASE_ARCH}.tar.gz"
   curl -fLO "https://github.com/unifabric-io/unifabric/releases/download/${LATEST_TAG}/unifabric-switch-agent_${LATEST_TAG}_linux_${RELEASE_ARCH}.tar.gz.sha256"
   sha256sum -c "unifabric-switch-agent_${LATEST_TAG}_linux_${RELEASE_ARCH}.tar.gz.sha256"

   tar -xzf "unifabric-switch-agent_${LATEST_TAG}_linux_${RELEASE_ARCH}.tar.gz"
   sudo install -m 0755 "unifabric-switch-agent_${LATEST_TAG}_linux_${RELEASE_ARCH}/unifabric-switch-agent" /usr/local/bin/unifabric-switch-agent
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

2. 把导出的 mTLS 文件安装到 `/etc/unifabric/switch-mtls`:

   ```bash
   sudo install -d -m 0755 /etc/unifabric/switch-mtls
   sudo install -m 0644 tls.crt peer.crt /etc/unifabric/switch-mtls/
   sudo install -m 0600 tls.key /etc/unifabric/switch-mtls/
   ```

3. 创建 `/etc/systemd/system/unifabric-switch-agent.service`。systemd 会把 `%H`
   渲染为当前交换机主机名:

   ```ini
   [Unit]
   Description=Unifabric switch-agent
   After=network-online.target lldpd.service
   Wants=network-online.target

   [Service]
   Type=simple
   Environment=UNIFABRIC_SWITCH_AGENT_SWITCH_NAME=%H
   Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
   ExecStart=/usr/local/bin/unifabric-switch-agent
   Restart=always
   RestartSec=5s

   [Install]
   WantedBy=multi-user.target
   ```

4. 启动服务:

   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable --now unifabric-switch-agent
   ```

5. 确认服务在运行且日志没有错误:

   ```bash
   systemctl status unifabric-switch-agent
   journalctl -u unifabric-switch-agent -n 100 --no-pager
   ```

   ```text
   <!-- TODO: 待真实环境采集输出 -->
   ```

### 创建 Switch 资源

三种 case 都只部署 agent。agent 运行后,在有 `kubectl` 访问权限的主机上为每台
物理交换机创建一个 Kubernetes `Switch` 资源。Controller 连接 `spec.mgmtIP` 和
`spec.grpcPort` 读取 LLDP 快照。把以下示例保存为 `switch-leaf1.yaml`,并保持
`spec.mgmtIP` 与交换机的真实管理 IP 一致:

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

应用:

```bash
kubectl apply -f switch-leaf1.yaml
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

对其余交换机重复上述步骤。

> **注意:** 全自动模式下不要添加 `unifabric.io/switch-neighbors` 注解。
> `metadata.name` 可以是业务名称,但 FabricNode 的 LLDP 主机名必须匹配 `Switch`
> 资源名或 switch-agent 上报的 `status.hostname`。

## 验证

在有 `kubectl` 访问权限的主机上,确认 Controller 已收到每台交换机的 LLDP 数据:

```bash
kubectl get switches -o wide
kubectl get switch <switch-name> -o yaml
```

```text
<!-- TODO: 待真实环境采集输出 -->
```

当每个 `Switch` 资源的 `status.healthy` 为 `true`,且 `status.lldpNeighbors`
包含预期的节点与交换机邻居时,验证通过。

## 故障排查

### 日志出现 lldpcli command not found

先确认前提条件成立。登录交换机执行:

```bash
lldpcli -f json0 show neighbors
```

如果命令失败,在交换机上安装 `lldpd` 和 `lldpcli`。如果命令交互执行正常但
systemd 服务仍然失败,确认 service 文件的 `PATH` 行包含 `lldpcli` 所在目录。

### Switch.status.healthy 一直为 false

1. 确认 Controller 能连接交换机管理 IP 的 `8090` 端口,或配置的 `grpcPort`。
2. 确认交换机上的 mTLS 文件与 Helm 生成的 `switch-controller-mtls-agent` Secret
   一致。Helm 重装后需要重新导出文件。

### 脚本提示本地镜像缺失

脚本从不执行 `docker pull`。在该交换机的 Docker 中导入正确版本的镜像,然后重新
运行脚本。

### 脚本在远程 sudo 处失败

确认远程用户对 Docker 和证书目录有 root 权限。sudo 密码与 SSH 密码不同时,运行
前设置 `SUDO_PASSWORD`。

## 参考

- [Switch 参考](../reference/switch.zh.md):`Switch` 资源的 `spec` 与 `status`
  字段。
- [Scale-out 拓扑设计](../design/scaleout-topology.zh.md):Controller 如何使用
  交换机侧 LLDP 快照。
- [SONiC RoCE 指南](../getting-started-sonic-roce.zh.md):`socket` 模式的单台
  交换机 `docker run` 部署。
