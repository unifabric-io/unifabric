# switch-agent Onboarding

中文版: [switch-agent-onboarding.zh.md](./switch-agent-onboarding.zh.md)

## Introduction

In LLDP discovery, the Unifabric Agent on each node discovers the links from the
nodes to the leaf switches. The links between the switches come from the Unifabric
switch-agent. switch-agent runs on each switch, collects the LLDP neighbors of the
switch, and serves them to the Controller over mTLS gRPC on port `8090`. This
document describes how to run switch-agent on the switches and how to register the
switches in Kubernetes.

switch-agent runs on SONiC switches and on NVIDIA Spectrum switches that run
Cumulus Linux. It also runs on other Ethernet switches that provide Docker or a
Linux shell. If you cannot run software on the switches, use
[Manual topology](./manual-topology.md) instead. For the result and the
limitations of this method, see
[Network Topology Discovery](./README.md#method-1-lldp-discovery-with-switch-agent).

## Prerequisites

- Unifabric is deployed according to the [basic installation](../../getting-started.md),
  and `topoDiscovery.scaleOut.mode` or `topoDiscovery.storage.mode` is
  `unifabric-roce`.
- LLDP is enabled on every switch. Run the command on the switch. A leaf must see
  the nodes and the spine. A spine must see all leaf switches:

  ```bash
  lldpcli show neighbors -f json0
  ```

  ```text
  <!-- TODO: collect the output from a real environment -->
  ```

- The Kubernetes Node that runs `unifabric-controller` can reach TCP port `8090`
  on the management IP of every switch. The management IP is usually on the
  out-of-band management network. Make sure that the firewall or ACL of the
  switch allows inbound connections to port `8090` from the Controller node, and
  that the firewall of the Controller node allows outbound connections to the
  switches.
- A host with a `kubectl` context is available to export the mTLS certificates.

## Procedure

Complete Step 1 to Step 3 for every switch in the topology (leaf, spine, and core).

### Step 1: Export the mTLS Certificates

All deployment variants use the same three files from the Helm-generated Secret
`switch-controller-mtls-agent`: `tls.crt`, `tls.key`, and `peer.crt`. On the host
with `kubectl` access, export them to a local directory:

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

Deployment option 1 reads this directory through `CERT_SOURCE_DIR` and installs
the files on each switch. For the default deployment, option 2, and option 3, you
copy the files to each switch yourself. Each procedure names the target
directory.

### Step 2: Run switch-agent on the Switch

The default deployment is one `docker run` per switch in `socket` mode. It mounts
only the `lldpd` socket and the mTLS directory. It does not need host network,
host UTS, or privileged permissions. First copy the three files from Step 1 to
`/opt/unifabric-switch-agent/mtls/` on the switch:

```bash
sudo mkdir -p /opt/unifabric-switch-agent/mtls
sudo chmod 700 /opt/unifabric-switch-agent/mtls
```

Then start the container on the switch. Replace `<release-tag>` with the same
release tag as the Controller. `SWITCH_NAME` must match the LLDP hostname of the
switch. `-p 8090:8090` publishes the gRPC port that the Controller connects to:

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

After the start, make sure that the container runs and that the log has no errors:

```bash
docker ps | grep unifabric-switch-agent
docker logs --tail 100 unifabric-switch-agent
```

```text
<!-- TODO: collect the output from a real environment -->
```

The three deployment options below apply when the default deployment is not
usable:

| Deployment option | Use it when |
| --- | --- |
| Option 1: deployment script | The switches run Docker, and you deploy several switches from one management host. |
| Option 2: container in `hostProc` mode | The switch runs Docker, but the switch OS does not expose `/run/lldpd.socket`, or the socket permissions block container access. |
| Option 3: systemd service | The switch cannot run Docker or another container runtime. |

#### Deployment Option 1: Deploy Several Switches with the Script

The project provides
[deploy-switch-agent.sh](../../../tools/deploy-switch-agent/deploy-switch-agent.sh).
It copies the exported mTLS certificates from the management host and deploys the
switch-agent container on several switches. The script does not store target
addresses or passwords, and it does not pull images from a registry. When it
starts, it does the following:

1. Makes sure that `peer.crt`, `tls.crt`, and `tls.key` exist on the management host.
2. Copies the certificates to each target over SSH and SCP.
3. Installs the certificates to `/opt/unifabric-switch-agent/mtls` with sudo. The
   owner of the directory and the files is `root:root`. The certificates get mode
   `0644` and the private key gets mode `0600`.
4. Makes sure that the given image exists in Docker on the target switch.
5. Removes and recreates an existing `unifabric-switch-agent` container. If the
   container does not exist, the script creates it.
6. Continues with the other switches and reports all failed targets at the end.

The management host can be a Kubernetes node or a separate Linux host. It must
reach the management network of the target switches, and it must have Bash, `ssh`,
and `scp`. It needs `sshpass` only for password authentication. Each switch needs:

- SONiC OS 2025+ or NVIDIA Cumulus Linux 4.x.x+.
- Docker that the remote user can operate with sudo.
- The switch-agent image preloaded in local Docker.
- Non-interactive SSH commands that reach a Linux shell.

Run all commands in this option on the management host, unless a step says
otherwise.

1. Select the LLDP collection mode. The script passes the mode to the container on
   each target switch through environment variables.

   The default is `socket` mode. `LLDP_SOCKET_PATH` is the path of the `lldpd`
   Unix socket on each target switch, not on the management host. Before the
   deployment, log in to each switch and make sure that the host exposes the
   socket:

   ```bash
   ls -l /run/lldpd.socket
   ```

   ```text
   <!-- TODO: collect the output from a real environment -->
   ```

   The first character of the permission string must be `s`, which marks a Unix
   socket. Set the mode and the socket path explicitly:

   ```bash
   export LLDP_COLLECTION_MODE=socket
   export LLDP_SOCKET_PATH=/run/lldpd.socket
   ```

   The script first makes sure that the path on each switch is a socket. Then it
   mounts the socket into the container at the same path and publishes gRPC with
   `-p 8090:8090`. If a switch cannot expose or mount the `lldpd` socket, select
   `hostProc`:

   ```bash
   export LLDP_COLLECTION_MODE=hostProc
   ```

   In `hostProc` mode, the script creates the container with host network, host
   UTS, `--privileged`, and the host `/proc`. `GRPC_PORT` changes the listen port
   of switch-agent. In socket mode it also updates the Docker port mapping.

2. Give the targets and run the script. Pass a comma-separated list of switch
   management IPs in `HOSTS`. The commands below use the RFC 5737 documentation
   address range. Replace them with the real management IPs. Select one SSH
   authentication method. The selection affects only the SSH login and the remote
   sudo authentication.

   Method A, SSH key, is the default:

   ```bash
   SSH_USER=your-ssh-user \
   HOSTS="192.0.2.11,192.0.2.12,192.0.2.21" \
   CERT_SOURCE_DIR=./tmp-switch-mtls \
   SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>" \
   ./tools/deploy-switch-agent/deploy-switch-agent.sh
   ```

   ```text
   <!-- TODO: collect the output from a real environment -->
   ```

   > **Note:** The SSH key authenticates only the login. The script still runs
   > remote `sudo` to install the certificates and to operate Docker. If the remote
   > user has passwordless sudo, no more options are needed. If sudo needs a
   > password, add `SUDO_AUTH_MODE=password`. The script asks for the password
   > once, and the terminal hides the input.

   Method B, SSH password. Set `SSH_AUTH_MODE=password`. If `SSH_PASSWORD` is not
   set in advance, the script asks for it in the terminal. It does not store the
   password in the script or in the target list:

   ```bash
   SSH_AUTH_MODE=password \
   SSH_USER=your-ssh-user \
   HOSTS="192.0.2.11,192.0.2.12,192.0.2.21" \
   CERT_SOURCE_DIR=./tmp-switch-mtls \
   SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>" \
   ./tools/deploy-switch-agent/deploy-switch-agent.sh
   ```

   ```text
   <!-- TODO: collect the output from a real environment -->
   ```

   > **Note:** Password mode assumes that the SSH password is also the sudo
   > password. If they differ, set `SUDO_PASSWORD` separately before the run.

   The script accepts these environment variables:

   | Environment variable | Default | Description |
   | --- | --- | --- |
   | `SSH_USER` | none | Required. The SSH user on the switches. |
   | `SSH_PORT` | `22` | The SSH port. |
   | `SSH_AUTH_MODE` | `key` | `key` or `password`. |
   | `SUDO_AUTH_MODE` | derived from the SSH mode | `passwordless` by default with an SSH key, `password` by default with an SSH password. |
   | `HOSTS` | none | Required. A comma-separated list of switch management IPs. |
   | `CERT_SOURCE_DIR` | `./tmp-switch-mtls` | The local certificate directory. |
   | `SWITCH_AGENT_IMAGE` | none | Required. The full image name that already exists on each target. |
   | `LLDP_COLLECTION_MODE` | `socket` | `socket` or `hostProc`. |
   | `LLDP_SOCKET_PATH` | `/run/lldpd.socket` | The path of the `lldpd` Unix socket on each target switch. In socket mode it is mounted into the container at the same path. |
   | `GRPC_PORT` | `8090` | The listen port of switch-agent. In socket mode the script also maps this port. |
   | `REMOTE_UPLOAD_DIR` | `/tmp/unifabric-switch-agent-<user>` | The temporary upload directory for the certificates. |
   | `REMOTE_CERT_DIR` | `/opt/unifabric-switch-agent/mtls` | The final certificate directory. |
   | `CONTAINER_NAME` | `unifabric-switch-agent` | The Docker container name. |

3. Log in to each target switch. Make sure that the container runs and that the
   log has no errors:

   ```bash
   docker ps --filter name=unifabric-switch-agent
   docker logs --tail 100 unifabric-switch-agent
   ```

   ```text
   <!-- TODO: collect the output from a real environment -->
   ```

#### Deployment Option 2: Run the Container in hostProc Mode

`hostProc` mode mounts the host `/proc` and runs the host `lldpcli` with `nsenter`
in the mount and network namespaces of the host. `nsenter` is a Linux tool that
runs a command inside the namespaces of another process. This mode uses the
`lldpcli` and `lldpd` versions that the switch OS provides, not the CLI binary in
the switch-agent image. This mode does not read
`UNIFABRIC_SWITCH_AGENT_LLDP_CLI_VERSION`.

Run all commands in this option on the target switch.

1. Install the exported mTLS files to `/opt/unifabric-switch-agent/mtls`:

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
   <!-- TODO: collect the output from a real environment -->
   ```

   > **Warning:** The container runs with `--privileged`, host network, host UTS,
   > and the host `/proc`. If the switch OS exposes `/run/lldpd.socket` reliably,
   > use the default deployment in
   > [Step 2](#step-2-run-switch-agent-on-the-switch) instead. It has a smaller
   > permission surface.

3. Make sure that the container runs and that the log has no errors:

   ```bash
   docker ps | grep unifabric-switch-agent
   docker logs --tail 100 unifabric-switch-agent
   ```

   ```text
   <!-- TODO: collect the output from a real environment -->
   ```

#### Deployment Option 3: Install as a systemd Service

Use this option when the switch cannot run Docker or another container runtime.
switch-agent runs as a native Linux binary, and systemd manages it. It calls the
local `lldpcli` on the switch directly, so it does not mount `/run/lldpd.socket`.
The switch needs `lldpcli`, and the systemd `PATH` must find it.

Run all commands in this option on the target switch.

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
   <!-- TODO: collect the output from a real environment -->
   ```

2. Install the exported mTLS files to `/etc/unifabric/switch-mtls`:

   ```bash
   sudo install -d -m 0755 /etc/unifabric/switch-mtls
   sudo install -m 0644 tls.crt peer.crt /etc/unifabric/switch-mtls/
   sudo install -m 0600 tls.key /etc/unifabric/switch-mtls/
   ```

3. Create `/etc/systemd/system/unifabric-switch-agent.service`. systemd renders
   `%H` as the hostname of the switch:

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

5. Make sure that the service runs and that the log has no errors:

   ```bash
   systemctl status unifabric-switch-agent
   journalctl -u unifabric-switch-agent -n 100 --no-pager
   ```

   ```text
   <!-- TODO: collect the output from a real environment -->
   ```

### Step 3: Create the Switch Resources

After the agent runs, create one Kubernetes `Switch` resource for each physical
switch on the host with `kubectl` access. The Controller connects to
`spec.mgmtIP` and `spec.grpcPort` to read the LLDP snapshot. The `192.0.2.0/24`
addresses below are examples. Replace them with the real management IP of each
switch:

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

Repeat for the other switches. For the switches of the storage network, set `role`
to `Storage`.

> **Note:** `metadata.name` does not have to match the switch hostname. The
> Controller uses `Switch.status.hostname` to match the switch-side data with the
> hostname that the node LLDP discovered. Do not add the
> `unifabric.io/switch-neighbors` annotation to any `Switch` of the same role.
> That annotation selects [Manual topology](./manual-topology.md).

## Verification

Make sure that the Controller connected to the switch-agent on every switch:

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

The switch side is complete when every `Switch` shows `HEALTHY` as `true` and
`NEIGHBORS` matches the real neighbor count of the switch.

The final result is the Node labels and the `Topology` resource. To view them, see
[Network Topology Discovery](./README.md#view-topology-and-node-labels).

## Troubleshooting

### The Log Shows lldpcli command not found

First make sure that the prerequisites are met. Log in to the switch and run:

```bash
lldpcli -f json0 show neighbors
```

If the command fails, install `lldpd` and `lldpcli` on the switch. If the command
works in an interactive shell but the systemd service still fails, make sure that
the `PATH` line in the service file includes the directory of `lldpcli`.

### Switch.status.healthy Stays false

1. Make sure that the Controller can reach port `8090`, or the configured
   `grpcPort`, on the management IP of the switch.
2. Make sure that the mTLS files on the switch match the Helm-generated Secret
   `switch-controller-mtls-agent`. After a Helm reinstall, export the files again.

### The Script Reports a Missing Local Image

The script never runs `docker pull`. Import the correct image version into Docker
on that switch, then run the script again.

### The Script Fails at Remote sudo

Make sure that the remote user has root permission for Docker and for the
certificate directory. If the sudo password differs from the SSH password, set
`SUDO_PASSWORD` before the run.

### No Spine Tier Is Discovered

- Make sure that every physical switch in the topology has a `Switch` resource and
  a running switch-agent.
- Make sure that no `unifabric.io/switch-neighbors` annotation remains.
- Make sure that `Switch.status.hostname` or the resource name matches the switch
  hostname that the node LLDP discovered.
- Read the Controller log and the switch log:

  ```bash
  kubectl -n unifabric-system logs deployment/unifabric-controller
  docker logs --tail 100 unifabric-switch-agent
  ```

## References

- [Network Topology Discovery](./README.md): the discovery methods and how to view
  the result.
- [Manual topology](./manual-topology.md): declare switch neighbors instead of
  running switch-agent.
- [Switch reference](../../reference/switch.md): the `spec` and `status` fields of
  the `Switch` resource.
- [Scale-out topology design](../../design/scaleout-topology.md): how the
  Controller uses the switch-side LLDP snapshot and the neighbor declaration.
- [deploy-switch-agent.sh](../../../tools/deploy-switch-agent/deploy-switch-agent.sh):
  the batch deployment script.
