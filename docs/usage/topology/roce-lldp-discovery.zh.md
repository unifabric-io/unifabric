# RoCE 网络中 LLDP 发现

English version: [roce-lldp-discovery.md](./roce-lldp-discovery.md)

## 简介

LLDP 是一种协议。设备用它向链路对端的设备通告自己的身份和端口。RoCE 网络中 LLDP 发现由两部分
组成:

- 每个节点上的 Unifabric Agent 读取每张 RDMA 网卡的 LLDP 邻居,得到节点到 leaf
  交换机的连接。
- 每台交换机上的 Unifabric switch-agent 读取交换机的 LLDP 邻居,得到交换机之间的
  连接,并通过 `8090` 端口的 mTLS gRPC 提供给 Controller。

Controller 合并两者,计算出 leaf、spine、core 层级。本文说明如何在交换机上运行
switch-agent,以及如何在 Kubernetes 中登记交换机。

| 项目 | 说明 |
| --- | --- |
| 适用网络 | Scale-out 网络、Storage 网络 |
| 拓扑来源 | 节点侧和交换机侧的 LLDP 邻居 |
| 能发现的层级 | 节点、leaf、spine、core。全部层级。 |
| 交换机侧要求 | 启用 LLDP。有 Docker 或 Linux shell 供 switch-agent 运行。SONiC 满足要求。 |
| 集群侧要求 | 运行 `unifabric-controller` 的节点能访问交换机管理 IP 的 TCP `8090` 端口。 |
| Helm 配置 | `topoDiscovery.scaleOut.mode=unifabric-roce` 或 `topoDiscovery.storage.mode=unifabric-roce`,两者都是默认值。Storage 网络还需要网卡 selector。参见[基础安装](../../getting-started.zh.md#安装)。 |

如果安全策略不允许在交换机上运行软件,改用[自定义网络拓扑](./custom-network-topology.zh.md)。
如果交换机 OS 没有 Docker 也没有 Linux shell,也改用它。

> 注意: NVIDIA Spectrum 交换机运行的 Cumulus Linux 支持 LLDP 和 Docker。因此理论上
> 可以在它们上运行 switch-agent,但尚未经过验证。

各网络在不同网络设备下应选择哪种发现方式,参见
[网络拓扑发现](./README.zh.md#按网络选择发现方式)。

## 前提条件

- 已按[基础安装](../../getting-started.zh.md)完成部署,并且 `topoDiscovery.scaleOut.mode`
  或 `topoDiscovery.storage.mode` 为 `unifabric-roce`。
- 每台交换机已启用 LLDP。在交换机上执行下面的命令。leaf 必须看到节点和 spine。spine
  必须看到所有 leaf 交换机。下面的示例是 `leaf1` 上的输出:

  ```bash
  lldpcli show neighbors summary
  ```

  ```text
  -------------------------------------------------------------------------------
  LLDP neighbors:
  -------------------------------------------------------------------------------
  Interface:    Ethernet0, via: LLDP
    Chassis:
      ChassisID:    mac ee:e0:c1:36:71:78
      SysName:      node1
    Port:
      PortID:       mac ee:e0:c1:36:71:78
      PortDescr:    eth1
      TTL:          120
  -------------------------------------------------------------------------------
  Interface:    Ethernet48, via: LLDP
    Chassis:
      ChassisID:    mac 4a:c8:80:d5:14:63
      SysName:      spine1
    Port:
      PortID:       mac 4a:c8:80:d5:14:63
      PortDescr:    Ethernet0
      TTL:          120
  -------------------------------------------------------------------------------
  ```

- 运行 `unifabric-controller` 的 Kubernetes Node 能访问每台交换机管理 IP 的 TCP `8090`
  端口。管理 IP 是 `Switch` 资源的 `spec.mgmtIP`,填写交换机管理口所在的带外管理网络
  地址。

  Controller Pod 不使用 host network,它的流量从所在节点的默认路由发出。在 AI 集群中,
  节点的默认路由通常在服务器的管理网络上,而交换机管理口在另一张带外管理网络上。所以
  需要满足以下条件:
  - 服务器的管理网络与交换机的带外管理网络之间有路由。这与 NVIDIA NetQ、UFM 等网络
    管理软件的要求相同:它们的服务器也必须能访问交换机的带外管理网络。各代架构打通
    这条路由的方式不同。例如在 NVIDIA DGX SuperPOD GB200 中,带内网络有一个专用
    网段让管理节点访问带外设备。在 GB300 和 Vera Rubin 中,带外网络可以是物理独立的
    网络,通过自己的上联或路由泄露与其他网络互通。部署前请向网络管理员确认该路由。
    参见[参考](#参考)中的 NVIDIA 文档。
  - 如果只有部分节点能访问交换机的带外管理网络,例如 Kubernetes 管理节点,用 Helm
    value `controller.nodeSelector`、`controller.tolerations` 或 `controller.affinity`
    把 Controller 调度到这些节点上。
  - 交换机的防火墙或 ACL 允许来自 Controller 节点的 `8090` 端口入站连接。Controller
    节点的防火墙允许到交换机的出站连接。

  在运行 Controller 的节点上用下面的命令确认连通性:

  ```bash
  nc -zv <交换机管理 IP> 8090
  ```

  Controller 用固定的证书校验 switch-agent,不校验证书中的 IP 地址,所以更换管理 IP
  不需要重新生成证书。
- 有一台配置好 `kubectl` 上下文的主机用于导出 mTLS 证书。

## 操作步骤

拓扑中的每台交换机(leaf、spine、core)都要完成步骤 1 到步骤 3。

### 步骤 1:导出 mTLS 证书

所有部署方式使用 Helm 生成的 Secret `switch-controller-mtls-agent` 中相同的三个文件:
`tls.crt`、`tls.key` 和 `peer.crt`。在有 `kubectl` 访问权限的主机上,把它们导出到
本地目录:

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

部署方式一通过 `CERT_SOURCE_DIR` 读取该目录,并把文件安装到每台交换机。默认部署、
方式二和方式三需要你自行把文件复制到每台交换机。各步骤会说明目标目录。

### 步骤 2:在交换机上运行 switch-agent

默认部署是在每台交换机上以 `socket` 模式执行一次 `docker run`。它只挂载 `lldpd`
socket 和 mTLS 目录。它不需要 host network、host UTS 或 privileged 权限。先把步骤 1
的三个文件复制到交换机的 `/opt/unifabric-switch-agent/mtls/`:

```bash
sudo mkdir -p /opt/unifabric-switch-agent/mtls
sudo chmod 700 /opt/unifabric-switch-agent/mtls
```

然后在交换机上启动容器。把 `<release-tag>` 替换为与 Controller 相同的发布 tag。
`SWITCH_NAME` 必须与该交换机的 LLDP hostname 一致。`-p 8090:8090` 发布 Controller
连接的 gRPC 端口:

```bash
export SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>"
export SWITCH_NAME="leaf1"

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

启动后,确认容器在运行并且日志没有错误:

```bash
docker ps | grep unifabric-switch-agent
docker logs --tail 100 unifabric-switch-agent
```

```text
7df6e0a82fb9   ghcr.io/unifabric-io/unifabric-switch-agent:v0.4.2   "/usr/bin/unifabric/…"   2 minutes ago   Up 2 minutes   0.0.0.0:8090->8090/tcp, [::]:8090->8090/tcp   unifabric-switch-agent
{"time":"2026-10-20T08:15:02.114702361Z","level":"INFO","source":{"function":"main.main","file":"/unifabric/cmd/switch-agent/main.go","line":20},"msg":"load switch-agent config","source":"env"}
{"time":"2026-10-20T08:15:02.115380925Z","level":"INFO","source":{"function":"main.main","file":"/unifabric/cmd/switch-agent/main.go","line":44},"msg":"start switch agent"}
{"time":"2026-10-20T08:15:02.117264118Z","level":"INFO","source":{"function":"github.com/unifabric-io/unifabric/pkg/switchagent.(*Agent).Start","file":"/unifabric/pkg/switchagent/agent.go","line":88},"msg":"start switch agent","switchName":"leaf1","listenAddress":":8090","mtlsEnabled":true,"lldpRefreshInterval":"10s","lldpCollectionMode":"socket","lldpSocketPath":"/run/lldpd.socket","lldpCLIVersion":"1.0.16"}
```

默认部署不可用,或需要批量部署时,按下面三个相互独立的选择确定部署方式:

- 运行方式:交换机能运行 Docker 时,以容器运行。不能运行 Docker 时,以 systemd
  服务运行。
- LLDP 采集模式,仅容器方式需要选择:`socket` 模式挂载宿主机的 `/run/lldpd.socket`,
  不需要特权,是默认模式。`hostProc` 模式不需要该 socket,但需要 `--privileged`、
  host network、host UTS 和宿主机 `/proc`。
- 下发方式:在每台交换机上手工执行命令,或用部署脚本从一台管理机批量部署。部署
  脚本支持上面两种 LLDP 采集模式,由 `LLDP_COLLECTION_MODE` 选择。

| 运行方式 | LLDP 采集模式 | 交换机要求 | 手工逐台部署 | 脚本批量部署 |
| --- | --- | --- | --- | --- |
| 容器 | `socket` (默认) | 有 Docker。宿主机暴露 `/run/lldpd.socket`,并允许挂载进容器。 | 上面的默认部署 | [方式一](#部署方式一-使用脚本批量部署多台交换机),`LLDP_COLLECTION_MODE=socket` |
| 容器 | `hostProc` | 有 Docker。允许以 `--privileged`、host network 和 host UTS 运行容器。 | [方式二](#部署方式二-以-hostproc-模式运行容器) | [方式一](#部署方式一-使用脚本批量部署多台交换机),`LLDP_COLLECTION_MODE=hostProc` |
| systemd 服务 | 直接调用本地 `lldpcli` | 没有 Docker。已安装 `lldpcli`。 | [方式三](#部署方式三-安装为-systemd-服务) | 不支持 |

优先选择 `socket` 模式,它的权限面最小。只有交换机 OS 不暴露 `/run/lldpd.socket`,
或 socket 权限阻止容器访问时,才选择 `hostProc` 模式。

#### 部署方式一: 使用脚本批量部署多台交换机

项目提供
[deploy-switch-agent.sh](../../../tools/deploy-switch-agent/deploy-switch-agent.sh)。
它从管理机复制已导出的 mTLS 证书,并在多台交换机上部署 switch-agent 容器。脚本不
存储目标地址和密码,也不从镜像仓库拉取镜像。启动后它做以下事情:

1. 确认管理机上存在 `peer.crt`、`tls.crt` 和 `tls.key`。
2. 通过 SSH 和 SCP 把证书复制到每个目标。
3. 用 sudo 把证书安装到 `/opt/unifabric-switch-agent/mtls`。目录和文件的属主为
   `root:root`。证书权限为 `0644`,私钥权限为 `0600`。
4. 确认目标交换机的 Docker 中存在指定镜像。
5. 删除并重建已存在的 `unifabric-switch-agent` 容器。如果容器不存在,脚本直接创建。
6. 继续处理其余交换机,并在结束时汇报所有失败的目标。

管理机可以是 Kubernetes 节点,也可以是独立的 Linux 主机。它必须能访问目标交换机的
管理网络,并且安装了 Bash、`ssh` 和 `scp`。仅在密码认证时需要 `sshpass`。每台交换机
需要:

- SONiC OS 2025+ 或 NVIDIA Cumulus Linux 4.x.x+。
- 远程用户可以通过 sudo 操作的 Docker。
- 本地 Docker 中已预加载的 switch-agent 镜像。
- 能进入 Linux shell 的非交互式 SSH 命令。

除非步骤特别说明,本方式的所有命令都在管理机上执行。

1. 选择 LLDP 采集模式。脚本通过环境变量把模式传给每台目标交换机上的容器。

   默认是 `socket` 模式。`LLDP_SOCKET_PATH` 是每台目标交换机上 `lldpd` Unix socket
   的路径,不是管理机上的路径。部署前,登录每台交换机,确认宿主机暴露了该 socket:

   ```bash
   ls -l /run/lldpd.socket
   ```

   ```text
   srwxrwx--- 1 _lldpd _lldpd 0 Oct 20 08:10 /run/lldpd.socket
   ```

   权限串的第一个字符必须是 `s`,表示 Unix socket。显式设置模式和 socket 路径:

   ```bash
   export LLDP_COLLECTION_MODE=socket
   export LLDP_SOCKET_PATH=/run/lldpd.socket
   ```

   脚本先确认每台交换机上该路径是 socket。然后它把 socket 以相同路径挂载进容器,并用
   `-p 8090:8090` 发布 gRPC。如果某台交换机不能暴露或挂载 `lldpd` socket,选择
   `hostProc`:

   ```bash
   export LLDP_COLLECTION_MODE=hostProc
   ```

   `hostProc` 模式下,脚本以 host network、host UTS、`--privileged` 和宿主机 `/proc`
   创建容器。`GRPC_PORT` 修改 switch-agent 的监听端口。socket 模式下它同时更新
   Docker 端口映射。

2. 指定目标并运行脚本。通过 `HOSTS` 传入逗号分隔的交换机管理 IP。下面的命令使用
   RFC 5737 文档保留地址段。替换为真实的管理 IP。选择一种 SSH 认证方式。该选择只
   影响 SSH 登录和远程 sudo 认证。

   认证方式 A,SSH 密钥,为默认方式:

   ```bash
   SSH_USER=your-ssh-user \
   HOSTS="192.0.2.11,192.0.2.12,192.0.2.21" \
   CERT_SOURCE_DIR=./tmp-switch-mtls \
   SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>" \
   ./tools/deploy-switch-agent/deploy-switch-agent.sh
   ```

   ```text
   Deploying switch-agent to your-ssh-user@192.0.2.11
   3f1c9d2e8b7a6f04d2c18e95b7a3f6c0d1e2b4a5968c7d3e1f0a2b4c6d8e9f01
   Completed: 192.0.2.11
   Deploying switch-agent to your-ssh-user@192.0.2.12
   b82d4e6f1a3c5097e8d2b4f6a1c3e5d7092b4d6f8a1c3e5b7d9f1a3c5e7b9d02
   Completed: 192.0.2.12
   Deploying switch-agent to your-ssh-user@192.0.2.21
   c5e7a9b1d3f5072e4c6a8b0d2f4e6a8c0b2d4f6e8a0c2b4d6f8e0a2c4b6d8f03
   Completed: 192.0.2.21
   switch-agent deployed on 3 switch(es) with socket LLDP collection.
   ```

   > 注意: SSH 密钥只认证登录。脚本仍会执行远程 `sudo` 来安装证书和操作 Docker。
   > 如果远程用户有免密 sudo,不需要额外选项。如果 sudo 需要密码,加上
   > `SUDO_AUTH_MODE=password`。脚本会提示输入一次密码,终端隐藏输入。

   认证方式 B,SSH 密码。设置 `SSH_AUTH_MODE=password`。如果没有预先设置
   `SSH_PASSWORD`,脚本在终端提示输入。它不把密码存放在脚本或目标列表中:

   ```bash
   SSH_AUTH_MODE=password \
   SSH_USER=your-ssh-user \
   HOSTS="192.0.2.11,192.0.2.12,192.0.2.21" \
   CERT_SOURCE_DIR=./tmp-switch-mtls \
   SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>" \
   ./tools/deploy-switch-agent/deploy-switch-agent.sh
   ```

   ```text
   SSH password for your-ssh-user:
   Deploying switch-agent to your-ssh-user@192.0.2.11
   3f1c9d2e8b7a6f04d2c18e95b7a3f6c0d1e2b4a5968c7d3e1f0a2b4c6d8e9f01
   Completed: 192.0.2.11
   Deploying switch-agent to your-ssh-user@192.0.2.12
   b82d4e6f1a3c5097e8d2b4f6a1c3e5d7092b4d6f8a1c3e5b7d9f1a3c5e7b9d02
   Completed: 192.0.2.12
   Deploying switch-agent to your-ssh-user@192.0.2.21
   c5e7a9b1d3f5072e4c6a8b0d2f4e6a8c0b2d4f6e8a0c2b4d6f8e0a2c4b6d8f03
   Completed: 192.0.2.21
   switch-agent deployed on 3 switch(es) with socket LLDP collection.
   ```

   > 注意: 密码模式假设 SSH 密码同时是 sudo 密码。如果两者不同,运行前单独设置
   > `SUDO_PASSWORD`。

   脚本接受以下环境变量:

   | 环境变量 | 默认值 | 说明 |
   | --- | --- | --- |
   | `SSH_USER` | 无 | 必填。交换机的 SSH 用户。 |
   | `SSH_PORT` | `22` | SSH 端口。 |
   | `SSH_AUTH_MODE` | `key` | `key` 或 `password`。 |
   | `SUDO_AUTH_MODE` | 随 SSH 模式推导 | SSH 密钥时默认 `passwordless`,SSH 密码时默认 `password`。 |
   | `HOSTS` | 无 | 必填。逗号分隔的交换机管理 IP。 |
   | `CERT_SOURCE_DIR` | `./tmp-switch-mtls` | 本地证书目录。 |
   | `SWITCH_AGENT_IMAGE` | 无 | 必填。每个目标上已存在的完整镜像名。 |
   | `LLDP_COLLECTION_MODE` | `socket` | `socket` 或 `hostProc`。 |
   | `LLDP_SOCKET_PATH` | `/run/lldpd.socket` | 每台目标交换机上 `lldpd` Unix socket 的路径。socket 模式下按相同路径挂载进容器。 |
   | `GRPC_PORT` | `8090` | switch-agent 的监听端口。socket 模式下脚本同时映射该端口。 |
   | `REMOTE_UPLOAD_DIR` | `/tmp/unifabric-switch-agent-<user>` | 证书的临时上传目录。 |
   | `REMOTE_CERT_DIR` | `/opt/unifabric-switch-agent/mtls` | 证书的最终目录。 |
   | `CONTAINER_NAME` | `unifabric-switch-agent` | Docker 容器名。 |

3. 登录每台目标交换机。确认容器在运行并且日志没有错误:

   ```bash
   docker ps --filter name=unifabric-switch-agent
   docker logs --tail 100 unifabric-switch-agent
   ```

   ```text
   CONTAINER ID   IMAGE                                                COMMAND                  CREATED         STATUS         PORTS                                         NAMES
   3f1c9d2e8b7a   ghcr.io/unifabric-io/unifabric-switch-agent:v0.4.2   "/usr/bin/unifabric/…"   5 minutes ago   Up 5 minutes   0.0.0.0:8090->8090/tcp, [::]:8090->8090/tcp   unifabric-switch-agent
   {"time":"2026-10-20T08:15:02.114702361Z","level":"INFO","source":{"function":"main.main","file":"/unifabric/cmd/switch-agent/main.go","line":20},"msg":"load switch-agent config","source":"env"}
   {"time":"2026-10-20T08:15:02.115380925Z","level":"INFO","source":{"function":"main.main","file":"/unifabric/cmd/switch-agent/main.go","line":44},"msg":"start switch agent"}
   {"time":"2026-10-20T08:15:02.117264118Z","level":"INFO","source":{"function":"github.com/unifabric-io/unifabric/pkg/switchagent.(*Agent).Start","file":"/unifabric/pkg/switchagent/agent.go","line":88},"msg":"start switch agent","switchName":"leaf1","listenAddress":":8090","mtlsEnabled":true,"lldpRefreshInterval":"10s","lldpCollectionMode":"socket","lldpSocketPath":"/run/lldpd.socket","lldpCLIVersion":"1.0.16"}
   ```

#### 部署方式二: 以 hostProc 模式运行容器

`hostProc` 模式挂载宿主机 `/proc`,并用 `nsenter` 在宿主机的 mount 和 network
命名空间中运行宿主机的 `lldpcli`。`nsenter` 是一个 Linux 工具,在另一个进程的命名
空间内运行命令。该模式使用交换机 OS 提供的 `lldpcli` 和 `lldpd` 版本,而不是
switch-agent 镜像内的 CLI 二进制。该模式不读取
`UNIFABRIC_SWITCH_AGENT_LLDP_CLI_VERSION`。

本方式的所有命令都在目标交换机上执行。

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

   > 警告: 容器以 `--privileged`、host network、host UTS 和宿主机 `/proc` 运行。
   > 如果交换机 OS 能稳定暴露 `/run/lldpd.socket`,改用
   > [步骤 2](#步骤-2在交换机上运行-switch-agent) 中的默认部署。它的权限面更小。

3. 确认容器在运行并且日志没有错误:

   ```bash
   docker ps | grep unifabric-switch-agent
   docker logs --tail 100 unifabric-switch-agent
   ```

   ```text
   9a4e7b10c3d5   ghcr.io/unifabric-io/unifabric-switch-agent:v0.4.2   "/usr/bin/unifabric/…"   2 minutes ago   Up 2 minutes             unifabric-switch-agent
   {"time":"2026-10-20T08:15:02.114702361Z","level":"INFO","source":{"function":"main.main","file":"/unifabric/cmd/switch-agent/main.go","line":20},"msg":"load switch-agent config","source":"env"}
   {"time":"2026-10-20T08:15:02.115380925Z","level":"INFO","source":{"function":"main.main","file":"/unifabric/cmd/switch-agent/main.go","line":44},"msg":"start switch agent"}
   {"time":"2026-10-20T08:15:02.117264118Z","level":"INFO","source":{"function":"github.com/unifabric-io/unifabric/pkg/switchagent.(*Agent).Start","file":"/unifabric/pkg/switchagent/agent.go","line":88},"msg":"start switch agent","switchName":"leaf1","listenAddress":":8090","mtlsEnabled":true,"lldpRefreshInterval":"10s","lldpCollectionMode":"hostProc","lldpSocketPath":"/run/lldpd.socket","lldpCLIVersion":"1.0.16"}
   ```

#### 部署方式三: 安装为 systemd 服务

交换机无法运行 Docker 或其他容器运行时的场景使用本方式。switch-agent 以原生 Linux
二进制运行,由 systemd 管理。它直接调用交换机本地的 `lldpcli`,因此不挂载
`/run/lldpd.socket`。交换机需要安装 `lldpcli`,并且 systemd 的 `PATH` 必须能找到它。

本方式的所有命令都在目标交换机上执行。

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
   unifabric-switch-agent_v0.4.2_linux_amd64.tar.gz: OK
   ```

2. 把导出的 mTLS 文件安装到 `/etc/unifabric/switch-mtls`:

   ```bash
   sudo install -d -m 0755 /etc/unifabric/switch-mtls
   sudo install -m 0644 tls.crt peer.crt /etc/unifabric/switch-mtls/
   sudo install -m 0600 tls.key /etc/unifabric/switch-mtls/
   ```

3. 创建 `/etc/systemd/system/unifabric-switch-agent.service`。systemd 会把 `%H`
   渲染为交换机的主机名:

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

5. 确认服务在运行并且日志没有错误:

   ```bash
   systemctl is-active unifabric-switch-agent
   journalctl -u unifabric-switch-agent -n 100 --no-pager
   ```

   ```text
   active
   Oct 20 08:20:11 leaf1 unifabric-switch-agent[2817]: {"time":"2026-10-20T08:20:11.114702361Z","level":"INFO","source":{"function":"main.main","file":"github.com/unifabric-io/unifabric/cmd/switch-agent/main.go","line":20},"msg":"load switch-agent config","source":"env"}
   Oct 20 08:20:11 leaf1 unifabric-switch-agent[2817]: {"time":"2026-10-20T08:20:11.115380925Z","level":"INFO","source":{"function":"main.main","file":"github.com/unifabric-io/unifabric/cmd/switch-agent/main.go","line":44},"msg":"start switch agent"}
   Oct 20 08:20:11 leaf1 unifabric-switch-agent[2817]: {"time":"2026-10-20T08:20:11.117264118Z","level":"INFO","source":{"function":"github.com/unifabric-io/unifabric/pkg/switchagent.(*Agent).Start","file":"github.com/unifabric-io/unifabric/pkg/switchagent/agent.go","line":88},"msg":"start switch agent","switchName":"leaf1","listenAddress":":8090","mtlsEnabled":true,"lldpRefreshInterval":"10s","lldpCollectionMode":"socket","lldpSocketPath":"/run/lldpd.socket","lldpCLIVersion":"1.0.16"}
   ```

### 步骤 3:创建 Switch 资源

agent 运行后,进入有 `kubectl` 访问权限的主机。为每台物理交换机创建一个 Kubernetes
`Switch` 资源。Controller 连接 `spec.mgmtIP` 和 `spec.grpcPort` 读取 LLDP 快照。下面
的 `192.0.2.0/24` 地址仅为示例。替换为各交换机真实的管理 IP:

```bash
kubectl apply -f - <<'EOF'
apiVersion: unifabric.io/v1beta1
kind: Switch
metadata:
  name: leaf1
spec:
  mgmtIP: 192.0.2.11
  role: ScaleOut
  grpcPort: 8090
---
apiVersion: unifabric.io/v1beta1
kind: Switch
metadata:
  name: spine1
spec:
  mgmtIP: 192.0.2.21
  role: ScaleOut
  grpcPort: 8090
EOF
```

其余交换机重复此操作。Storage 网络的交换机把 `role` 设为 `Storage`。

> 注意: `metadata.name` 不要求与交换机 hostname 相同。

> Controller 用 `Switch.status.hostname` 把交换机侧数据与节点 LLDP 发现的 hostname 关联。

> 不要给同 role 的任何 `Switch` 添加 `unifabric.io/switch-neighbors` annotation。该
> annotation 会选择[自定义网络拓扑](./custom-network-topology.zh.md)。

## 验证

确认 Controller 已连上每台交换机的 switch-agent:

```bash
kubectl get switches -o wide
```

```text
NAME     MGMTIP       ROLE       HEALTHY   NEIGHBORS
leaf1    192.0.2.11   ScaleOut   true      3
leaf2    192.0.2.12   ScaleOut   true      3
leaf3    192.0.2.13   ScaleOut   true      3
leaf4    192.0.2.14   ScaleOut   true      3
spine1   192.0.2.21   ScaleOut   true      4
```

每个 `Switch` 都满足两个条件时,交换机侧完成。`HEALTHY` 为 `true`,并且 `NEIGHBORS`
与该交换机真实的邻居数一致。

最终结果是 Node label 和 `Topology` 资源。查看方法参见
[网络拓扑发现](./README.zh.md#查看-topology-与-node-label)。

## 故障排查

### 日志出现 lldpcli command not found

先确认前提条件成立。登录交换机执行:

```bash
lldpcli -f json0 show neighbors
```

如果命令失败,在交换机上安装 `lldpd` 和 `lldpcli`。命令可能在交互式 shell 中正常,而
systemd 服务仍然失败。这时,确认 service 文件的 `PATH` 行包含 `lldpcli` 所在目录。

### Switch.status.healthy 一直为 false

1. 确认 Controller 能访问交换机管理 IP 的 `8090` 端口,或配置的 `grpcPort`。
2. 确认交换机上的 mTLS 文件与 Helm 生成的 Secret `switch-controller-mtls-agent`
   一致。Helm 重装后,重新导出文件。

### 脚本提示本地镜像缺失

脚本从不执行 `docker pull`。在该交换机的 Docker 中导入正确版本的镜像,然后重新运行
脚本。

### 脚本在远程 sudo 处失败

确认远程用户对 Docker 和证书目录有 root 权限。如果 sudo 密码与 SSH 密码不同,运行前
设置 `SUDO_PASSWORD`。

### 没有发现 spine 层级

- 确认拓扑中每台物理交换机都有 `Switch` 资源和运行中的 switch-agent。
- 确认没有残留 `unifabric.io/switch-neighbors` annotation。
- 确认 `Switch.status.hostname` 或资源名与节点 LLDP 发现的交换机 hostname 一致。
- 查看 Controller 日志和交换机日志:

  ```bash
  kubectl -n unifabric-system logs deployment/unifabric-controller
  docker logs --tail 100 unifabric-switch-agent
  ```

## 参考

- [网络拓扑发现](./README.zh.md):发现方式与结果查看。
- [自定义网络拓扑](./custom-network-topology.zh.md):不运行 switch-agent,改为声明交换机邻居。
- [Switch 参考](../../reference/switch.zh.md):`Switch` 资源的 `spec` 与 `status`
  字段。
- [Scale-out 拓扑设计](../../design/scaleout-topology.zh.md):Controller 如何使用
  交换机侧 LLDP 快照。
- [deploy-switch-agent.sh](../../../tools/deploy-switch-agent/deploy-switch-agent.sh):
  批量部署脚本。

以下 NVIDIA 文档说明了 AI 集群中的管理网络规划,以及网络管理软件对交换机带外管理
网络的访问要求:

- [DGX SuperPOD GB200 参考架构:Network Fabrics](https://docs.nvidia.com/dgx-superpod/reference-architecture-scalable-infrastructure-gb200/latest/network-fabrics.html):
  带内网络中专用于访问带外设备的网段。
- [DGX SuperPOD GB300 参考架构 (PDF)](https://docs.nvidia.com/pdf/dgx-spod-gb300-ra.pdf):
  独立的带外网络,以及带外网络的上联与路由泄露。
- [DGX SuperPOD Spectrum-X 参考架构:Vera Rubin NVL72 (PDF)](https://docs.nvidia.com/dgx-superpod-spectrum-x-reference-architecture-with-dgx-vera-rubin-nvl72.pdf):
  带外网络中的交换机管理网络。
- [DGX SuperPOD H100 参考架构:Network Fabrics](https://docs.nvidia.com/dgx-superpod/reference-architecture-scalable-infrastructure-h100/latest/network-fabrics.html):
  带外网络与带内网络互通。
- [NetQ:Install NetQ Agents](https://docs.nvidia.com/networking-ethernet-software/cumulus-netq-49/Installation-Management/Install-NetQ/Install-NetQ-Agents/):
  交换机上的 NetQ Agent 通过管理 VRF 连接 NetQ 服务器。
- [UFM Enterprise:UFM Communication Requirements](https://networking-docs.nvidia.com/ufmenterpriseum/6.26.1/ufm-communication-requirements):
  UFM 服务器通过带外管理网络管理 InfiniBand 交换机。
