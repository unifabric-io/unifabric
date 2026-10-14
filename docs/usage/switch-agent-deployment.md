# Switch Agent Deployment

中文版: [switch-agent-deployment.zh.md](./switch-agent-deployment.zh.md)

## Introduction

The switch-agent runs on each physical switch in the SONiC RoCE scenario. It collects
LLDP snapshots on the switch and serves them to the Controller over mTLS gRPC on port
`8090`. This guide covers three deployment cases: the batch deployment script, a
manual container in `hostProc` mode, and a native binary under systemd. Every case
installs the agent only. After any case, you must create one Kubernetes `Switch`
resource per switch so that the Controller connects to the agent.

Select the case from this table:

| Case | Use it when |
| --- | --- |
| Case 1: deployment script | The switches run Docker, and you deploy several switches from one management host. |
| Case 2: manual container in `hostProc` mode | The switch runs Docker, but the switch OS does not expose `/run/lldpd.socket`, or the socket permissions block container access. |
| Case 3: systemd service | The switch cannot run Docker or another container runtime. |

> **Note:** For one switch that exposes a working `lldpd` socket, the plain
> `docker run` steps in the [SONiC RoCE guide](../getting-started-sonic-roce.md) are
> the main path. The default `socket` mode mounts only the `lldpd` socket and the mTLS
> directory, and it needs no privileged permissions. `hostProc` mode requires
> `--privileged` and mounts the host `/proc`. Use it only when the socket is not
> usable.

## Prerequisites

- Unifabric is installed through the [basic installation](../getting-started.md), and
  the [SONiC RoCE guide](../getting-started-sonic-roce.md) selected Scenario 1,
  switches that run switch-agent.
- The Kubernetes Node that runs `unifabric-controller` can reach TCP port `8090` on
  the management IP of every switch.
- `lldpd` runs on every switch. Make sure of it on each switch:

  ```bash
  lldpcli -f json0 show neighbors
  ```

  ```text
  <!-- TODO: capture output from a real environment -->
  ```

- A host with a configured `kubectl` context exports the mTLS certificates.

## Configuration and Usage

### Export the mTLS Certificates

All three cases use the same three files: `tls.crt`, `tls.key`, and `peer.crt` from
the Helm-generated `switch-controller-mtls-agent` Secret. On the host with `kubectl`
access, export them into a local directory:

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

Case 1 reads this directory through `CERT_SOURCE_DIR` and installs the files on every
switch. For Case 2 and Case 3, copy the files to each switch yourself. The case steps
name the target directory.

### Case 1: Deploy Multiple Switches with the Script

The project provides
[deploy-switch-agent.sh](../../tools/deploy-switch-agent/deploy-switch-agent.sh). It
copies the exported mTLS certificates from the management host and deploys the
switch-agent container on multiple switches. The script stores no target address or
password, and it does not pull from an image registry. After it starts, it
automatically:

1. Checks for `peer.crt`, `tls.crt`, and `tls.key` on the management host.
2. Copies the certificates to every target over SSH and SCP.
3. Uses sudo to install them under `/opt/unifabric-switch-agent/mtls`. Directories and
   files are owned by `root:root`. Certificates use mode `0644`, and the private key
   uses `0600`.
4. Checks that the requested image already exists in Docker on the target switch.
5. Removes and recreates an existing `unifabric-switch-agent` container, or creates it
   when absent.
6. Continues with the other switches and reports every failed target at the end.

The management host can be a Kubernetes node or a separate Linux host. It must reach
the management network of the target switches, and it needs Bash, `ssh`, and `scp`.
`sshpass` is needed only for password authentication. Every switch needs SONiC OS
2025+ or NVIDIA Cumulus Linux 4.x.x+, Docker that the remote user can operate through
sudo, the switch-agent image preloaded in local Docker, and non-interactive SSH
commands that enter a Linux shell.

Run all commands in this case on the management host, unless a step says otherwise.

1. Select the LLDP collection mode. The script passes the mode to the container on
   every target switch through environment variables.

   The default is `socket` mode. `LLDP_SOCKET_PATH` is the path of the `lldpd` Unix
   socket on every target switch host, not on the management host. Before deployment,
   log in to every switch and make sure that the host exposes the socket:

   ```bash
   ls -l /run/lldpd.socket
   ```

   ```text
   <!-- TODO: capture output from a real environment -->
   ```

   The first character of the permission string must be `s`, which marks a Unix
   socket. To set the mode and the socket path explicitly:

   ```bash
   export LLDP_COLLECTION_MODE=socket
   export LLDP_SOCKET_PATH=/run/lldpd.socket
   ```

   The script first checks that the path is a socket on every switch host, then
   mounts it at the same path in the container and publishes gRPC with
   `-p 8090:8090`. If a switch host cannot expose or mount the `lldpd` socket, select
   `hostProc`:

   ```bash
   export LLDP_COLLECTION_MODE=hostProc
   ```

   In `hostProc` mode, the script creates the container with host network, host UTS,
   `--privileged`, and the host `/proc`. `GRPC_PORT` changes the switch-agent listen
   port, and socket mode also updates the Docker port mapping.

2. Specify the targets and run the script. Pass the comma-separated switch management
   IPs through `HOSTS`. The commands below use addresses from the RFC 5737
   documentation range. Replace them with the real management IPs. Choose one SSH
   authentication option. The choice affects only the SSH login and the remote sudo
   authentication.

   Option A, SSH key, is the default:

   ```bash
   SSH_USER=your-ssh-user \
   HOSTS="192.0.2.11,192.0.2.12,192.0.2.21" \
   CERT_SOURCE_DIR=./tmp-switch-mtls \
   SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>" \
   ./tools/deploy-switch-agent/deploy-switch-agent.sh
   ```

   ```text
   <!-- TODO: capture output from a real environment -->
   ```

   > **Note:** An SSH key authenticates the login only. The script still runs remote
   > `sudo` to install certificates and operate Docker. No extra option is needed when
   > the remote user has passwordless sudo. When sudo requires a password, add
   > `SUDO_AUTH_MODE=password`. The script prompts once, with the input hidden from
   > the terminal.

   Option B, SSH password. Set `SSH_AUTH_MODE=password`. When `SSH_PASSWORD` is not
   already set, the script prompts for it on the terminal instead of storing it in the
   script or the target list:

   ```bash
   SSH_AUTH_MODE=password \
   SSH_USER=your-ssh-user \
   HOSTS="192.0.2.11,192.0.2.12,192.0.2.21" \
   CERT_SOURCE_DIR=./tmp-switch-mtls \
   SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>" \
   ./tools/deploy-switch-agent/deploy-switch-agent.sh
   ```

   ```text
   <!-- TODO: capture output from a real environment -->
   ```

   > **Note:** Password mode assumes that the SSH password is also the sudo password.
   > Set `SUDO_PASSWORD` separately before the run when they differ.

   The script accepts these environment variables:

   | Environment variable | Default | Description |
   | --- | --- | --- |
   | `SSH_USER` | none | Required switch SSH user. |
   | `SSH_PORT` | `22` | SSH port. |
   | `SSH_AUTH_MODE` | `key` | `key` or `password`. |
   | `SUDO_AUTH_MODE` | derived from SSH mode | Defaults to `passwordless` for SSH keys and `password` for SSH password mode. |
   | `HOSTS` | none | Required comma-separated switch management IPs. |
   | `CERT_SOURCE_DIR` | `./tmp-switch-mtls` | Local certificate directory. |
   | `SWITCH_AGENT_IMAGE` | none | Required full image name already present on every target. |
   | `LLDP_COLLECTION_MODE` | `socket` | `socket` or `hostProc`. |
   | `LLDP_SOCKET_PATH` | `/run/lldpd.socket` | Path of the `lldpd` Unix socket on each target switch host. Socket mode mounts it at the same path in the container. |
   | `GRPC_PORT` | `8090` | switch-agent listen port. Socket mode also maps this port. |
   | `REMOTE_UPLOAD_DIR` | `/tmp/unifabric-switch-agent-<user>` | Temporary certificate upload directory. |
   | `REMOTE_CERT_DIR` | `/opt/unifabric-switch-agent/mtls` | Final certificate directory. |
   | `CONTAINER_NAME` | `unifabric-switch-agent` | Docker container name. |

3. Log in to each target switch and make sure that the container runs and that the
   logs contain no errors:

   ```bash
   docker ps --filter name=unifabric-switch-agent
   docker logs --tail 100 unifabric-switch-agent
   ```

   ```text
   <!-- TODO: capture output from a real environment -->
   ```

### Case 2: Run the Container Manually in hostProc Mode

`hostProc` mode mounts the host `/proc` and uses `nsenter` to run the host `lldpcli`
from the host mount and network namespaces. `nsenter` is a Linux tool that runs a
command inside the namespaces of another process. This mode uses the `lldpcli` and
`lldpd` pair that the switch OS ships, not the CLI binaries in the switch-agent
image. The mode does not read `UNIFABRIC_SWITCH_AGENT_LLDP_CLI_VERSION`.

Run all commands in this case on the target switch.

1. Install the exported mTLS files under `/opt/unifabric-switch-agent/mtls`:

   ```bash
   sudo install -d -m 0755 /opt/unifabric-switch-agent/mtls
   sudo install -m 0644 tls.crt peer.crt /opt/unifabric-switch-agent/mtls/
   sudo install -m 0600 tls.key /opt/unifabric-switch-agent/mtls/
   ```

2. Start the container:

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
   <!-- TODO: capture output from a real environment -->
   ```

   > **Warning:** The container runs with `--privileged`, host network, host UTS, and
   > the host `/proc`. If the switch OS exposes `/run/lldpd.socket` reliably, use the
   > default `socket` mode from the [SONiC RoCE guide](../getting-started-sonic-roce.md)
   > instead. It has a smaller permission surface.

3. Make sure that the container runs and that the logs contain no errors:

   ```bash
   docker ps | grep unifabric-switch-agent
   docker logs --tail 100 unifabric-switch-agent
   ```

   ```text
   <!-- TODO: capture output from a real environment -->
   ```

### Case 3: Install as a systemd Service

Use this case when the switch cannot run Docker or another container runtime. The
switch-agent runs as a native Linux binary that systemd manages. It calls the
switch-local `lldpcli` directly, so it does not mount `/run/lldpd.socket`. The switch
needs `lldpcli` installed and reachable from the `PATH` of systemd.

Run all commands in this case on the target switch.

1. Download and install the binary that matches the CPU architecture of the switch
   from the GitHub Release page:

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
   <!-- TODO: capture output from a real environment -->
   ```

2. Install the exported mTLS files under `/etc/unifabric/switch-mtls`:

   ```bash
   sudo install -d -m 0755 /etc/unifabric/switch-mtls
   sudo install -m 0644 tls.crt peer.crt /etc/unifabric/switch-mtls/
   sudo install -m 0600 tls.key /etc/unifabric/switch-mtls/
   ```

3. Create `/etc/systemd/system/unifabric-switch-agent.service`. systemd renders `%H`
   as the current switch hostname:

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

4. Start the service:

   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable --now unifabric-switch-agent
   ```

5. Make sure that the service runs and that the logs contain no errors:

   ```bash
   systemctl status unifabric-switch-agent
   journalctl -u unifabric-switch-agent -n 100 --no-pager
   ```

   ```text
   <!-- TODO: capture output from a real environment -->
   ```

### Create the Switch Resources

All three cases deploy the agent only. After the agent runs, create one Kubernetes
`Switch` resource per physical switch on the host with `kubectl` access. The
Controller connects to `spec.mgmtIP` and `spec.grpcPort` to read LLDP snapshots. Save
this example as `switch-leaf1.yaml` and keep `spec.mgmtIP` consistent with the real
management IP of the switch:

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

Apply it:

```bash
kubectl apply -f switch-leaf1.yaml
```

```text
<!-- TODO: capture output from a real environment -->
```

Repeat this for the remaining switches.

> **Note:** Do not add the `unifabric.io/switch-neighbors` annotation in fully
> automatic mode. `metadata.name` can be a business-facing name, but the FabricNode
> LLDP hostnames must match either the `Switch` resource name or the
> `status.hostname` that the switch-agent reports.

## Verification

On the host with `kubectl` access, make sure that the Controller receives LLDP data
from every switch:

```bash
kubectl get switches -o wide
kubectl get switch <switch-name> -o yaml
```

```text
<!-- TODO: capture output from a real environment -->
```

The verification passes when every `Switch` resource has `status.healthy: true` and
`status.lldpNeighbors` contains the expected node and switch peers.

## Troubleshooting

### The Logs Show lldpcli command not found

First make sure that the precondition holds. Log in to the switch and run:

```bash
lldpcli -f json0 show neighbors
```

If the command fails, install `lldpd` and `lldpcli` on the switch. If the command
works interactively but the systemd service still fails, make sure that the `PATH`
line of the service file includes the directory that contains `lldpcli`.

### Switch.status.healthy Stays false

1. Make sure that the Controller can connect to the switch management IP on port
   `8090`, or on the configured `grpcPort`.
2. Make sure that the mTLS files on the switch match the Helm-generated
   `switch-controller-mtls-agent` Secret. Export the files again after a Helm
   reinstall.

### The Script Reports a Missing Local Image

The script never runs `docker pull`. Import the correct image version into Docker on
that switch, then run the script again.

### The Script Fails on Remote sudo

Make sure that the remote user has root access for Docker and for the certificate
directory. When the sudo password differs from the SSH password, set `SUDO_PASSWORD`
before the run.

## References

- [Switch reference](../reference/switch.md): the `spec` and `status` fields of the
  `Switch` resource.
- [Scale-out topology design](../design/scaleout-topology.md): how the Controller
  uses the switch-side LLDP snapshots.
- [SONiC RoCE guide](../getting-started-sonic-roce.md): the single-switch `docker run`
  deployment in `socket` mode.
