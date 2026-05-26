# switch-agent systemd 安装方式

English version: [switch-agent-systemd.md](./switch-agent-systemd.md)

当交换机不能运行 Docker 或其他容器运行时时，可以使用这种方式。该模式下，
`unifabric-switch-agent` 作为 Linux 原生二进制由 systemd 管理。

systemd 原生安装会直接使用交换机本机的 `lldpcli` 采集 LLDP，不需要像容器模式一样挂载
`/run/lldpd.socket`。

## 前置条件

- 交换机上已经运行 `lldpd`。
- 交换机上已经安装 `lldpcli`，并且 systemd 的 `PATH` 可以找到它。

安装服务前，先在交换机本机验证 LLDP：

```bash
lldpcli -f json0 show neighbors
```

## 安装二进制

从 GitHub Release 页面下载与交换机 CPU 架构匹配的压缩包。例如：

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

## 安装 mTLS 文件

在交换机上创建默认 mTLS 目录：

```bash
sudo install -d -m 0755 /etc/unifabric/switch-mtls
```

将从 `switch-controller-mtls-agent` Secret 导出的 `tls.crt`、`tls.key` 和
`peer.crt` 复制到 `/etc/unifabric/switch-mtls/`，然后设置权限：

```bash
sudo chmod 0644 /etc/unifabric/switch-mtls/tls.crt /etc/unifabric/switch-mtls/peer.crt
sudo chmod 0600 /etc/unifabric/switch-mtls/tls.key
```

## 配置 systemd

创建 `/etc/systemd/system/unifabric-switch-agent.service`：

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

`%H` 会由 systemd 渲染为当前交换机的 hostname。

启动服务：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now unifabric-switch-agent
```

## 验证

```bash
systemctl status unifabric-switch-agent
journalctl -u unifabric-switch-agent -n 100 --no-pager
```

然后在 Kubernetes 集群中创建或更新对应的 `Switch` 资源：

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

检查 controller 是否已经收到 LLDP 数据：

```bash
kubectl get switches -o wide
kubectl get switch <switch-name> -o yaml
```

## 排障

- 如果服务日志出现 `lldpcli command not found in PATH`，需要在交换机上安装
  `lldpd` / `lldpcli`，或调整 systemd 的 `PATH`。
- 如果交互式执行 `lldpcli -f json0 show neighbors` 正常，但 systemd 下失败，确认 service
  的 `PATH` 包含 `lldpcli` 所在目录。
- 如果 `Switch.status.healthy` 长时间为 `false`，确认 controller 可以访问交换机管理 IP 和 `8090` 端口，并确认 mTLS 文件与 Helm 生成的 `switch-controller-mtls-agent` Secret 匹配。
