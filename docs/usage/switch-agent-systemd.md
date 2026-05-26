# switch-agent systemd installation

中文版: [switch-agent-systemd.zh.md](./switch-agent-systemd.zh.md)

Use this guide when a switch cannot run Docker or another container runtime. In
this mode, `unifabric-switch-agent` runs as a native Linux binary managed by
systemd.

The systemd native installation uses the switch-local `lldpcli` to collect LLDP
data. It does not need to mount `/run/lldpd.socket` like the container mode.

## Prerequisites

- `lldpd` is running on the switch.
- `lldpcli` is installed on the switch and available from systemd's `PATH`.

Verify LLDP locally before installing the service:

```bash
lldpcli -f json0 show neighbors
```

## Install the Binary

Download the archive that matches the switch CPU architecture from the GitHub
Release page. For example:

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

## Install mTLS Files

On the switch, create the default mTLS directory:

```bash
sudo install -d -m 0755 /etc/unifabric/switch-mtls
```

Copy `tls.crt`, `tls.key`, and `peer.crt` exported from the
`switch-controller-mtls-agent` Secret to `/etc/unifabric/switch-mtls/`, then set
permissions:

```bash
sudo chmod 0644 /etc/unifabric/switch-mtls/tls.crt /etc/unifabric/switch-mtls/peer.crt
sudo chmod 0600 /etc/unifabric/switch-mtls/tls.key
```

## Configure systemd

Create `/etc/systemd/system/unifabric-switch-agent.service`:

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

`%H` is rendered by systemd as the current switch hostname.

Start the service:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now unifabric-switch-agent
```

## Verify

```bash
systemctl status unifabric-switch-agent
journalctl -u unifabric-switch-agent -n 100 --no-pager
```

Then create or update the matching `Switch` resource in the Kubernetes cluster:

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

Check the controller has received LLDP data:

```bash
kubectl get switches -o wide
kubectl get switch <switch-name> -o yaml
```

## Troubleshooting

- If the service logs show `lldpcli command not found in PATH`, install
  `lldpd` / `lldpcli` on the switch or adjust the systemd `PATH`.
- If `lldpcli -f json0 show neighbors` works interactively but not under
  systemd, confirm the service `PATH` includes the directory that contains
  `lldpcli`.
- If `Switch.status.healthy` stays `false`, confirm the controller can connect
  to the switch management IP and port `8090`, and confirm the mTLS files match
  the Helm-generated `switch-controller-mtls-agent` Secret.
