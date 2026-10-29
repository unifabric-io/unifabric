# LLDP Discovery in RoCE Networks

中文版: [roce-lldp-discovery.zh.md](./roce-lldp-discovery.zh.md)

## Introduction

LLDP is a protocol. A device uses it to advertise its identity and its ports to the
device at the other end of a link. LLDP discovery in RoCE networks has two parts:

- The Unifabric Agent on each node reads the LLDP neighbors of each RDMA NIC. This
  gives the links from the node to the leaf switches.
- The Unifabric switch-agent on each switch reads the LLDP neighbors of the switch.
  This gives the links between the switches. switch-agent serves them to the
  Controller over mTLS gRPC on port `8090`.

The Controller merges the two parts and calculates the leaf, spine, and core tiers.
This document tells how to run switch-agent on the switches and how to register the
switches in Kubernetes.

| Item | Description |
| --- | --- |
| Applicable networks | Scale-out network, Storage network |
| Topology source | The LLDP neighbors on the node side and on the switch side |
| Discovered tiers | Node, leaf, spine, core. All tiers. |
| Switch side requirements | LLDP is enabled. Docker or a Linux shell is available to run switch-agent. SONiC meets these requirements. |
| Cluster side requirements | The node that runs `unifabric-controller` can reach TCP port `8090` on the switch management IP. |
| Helm configuration | `topoDiscovery.scaleOut.mode=unifabric-roce` or `topoDiscovery.storage.mode=unifabric-roce`. Both are the default values. The Storage network also needs a NIC selector. See [basic installation](../../getting-started.md#installation). |

If your security policy does not let you run software on the switches, use
[Custom network topology](./custom-network-topology.md) instead. Also use it if the switch OS has
no Docker and no Linux shell.

> Note: NVIDIA Spectrum switches run Cumulus Linux, which supports LLDP and Docker.
> Thus switch-agent can run on them in theory, but this is not verified.

To select a discovery method for each network on different network devices, see
[Network topology discovery](./README.md#choose-a-discovery-method-by-network).

## Prerequisites

- Unifabric is deployed according to the [basic installation](../../getting-started.md),
  and `topoDiscovery.scaleOut.mode` or `topoDiscovery.storage.mode` is
  `unifabric-roce`.
- LLDP is enabled on every switch. Run the command below on the switch. A leaf must
  see the nodes and the spine. A spine must see all leaf switches. The example
  below is the output on `leaf1`:

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

- The Kubernetes Node that runs `unifabric-controller` can reach TCP port `8090`
  on the management IP of every switch. The management IP is `spec.mgmtIP` of the
  `Switch` resource. Set it to the address of the switch management port on the
  out-of-band management network.

  The Controller Pod does not use the host network. Its traffic leaves through
  the default route of its node. In an AI cluster, the default route of a node is
  usually on the server management network, and the switch management ports are
  on a different out-of-band management network. So these conditions must be
  true:
  - A route exists between the server management network and the out-of-band
    management network of the switches. NVIDIA NetQ, UFM, and other network
    management software have the same requirement: their servers must also reach
    the out-of-band management network of the switches.
    Each architecture provides this route in a different way. For example, in
    NVIDIA DGX SuperPOD GB200, the in-band network has a dedicated segment that
    lets the management nodes reach the out-of-band devices. In GB300 and Vera
    Rubin, the out-of-band network can be a physically separate network that
    connects to the other networks through its own uplink or through route
    leaking. Before the deployment, ask the network administrator to confirm this
    route. See the NVIDIA documents in [References](#references).
  - If only some nodes can reach the out-of-band management network of the
    switches, for example the Kubernetes control plane nodes, use the Helm value
    `controller.nodeSelector`, `controller.tolerations`, or `controller.affinity`
    to schedule the Controller on these nodes.
  - The firewall or ACL of the switch allows inbound connections to port `8090`
    from the Controller node. The firewall of the Controller node allows outbound
    connections to the switches.

  On the node that runs the Controller, use the command below to check the
  connection:

  ```bash
  nc -zv <switch-management-ip> 8090
  ```

  The Controller checks switch-agent against a pinned certificate. It does not
  check the IP address in the certificate, so a different management IP does not
  need new certificates.
- A host with a configured `kubectl` context is available to export the mTLS
  certificates.

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
7df6e0a82fb9   ghcr.io/unifabric-io/unifabric-switch-agent:v0.4.2   "/usr/bin/unifabric/…"   2 minutes ago   Up 2 minutes   0.0.0.0:8090->8090/tcp, [::]:8090->8090/tcp   unifabric-switch-agent
{"time":"2026-10-20T08:15:02.114702361Z","level":"INFO","source":{"function":"main.main","file":"/unifabric/cmd/switch-agent/main.go","line":20},"msg":"load switch-agent config","source":"env"}
{"time":"2026-10-20T08:15:02.115380925Z","level":"INFO","source":{"function":"main.main","file":"/unifabric/cmd/switch-agent/main.go","line":44},"msg":"start switch agent"}
{"time":"2026-10-20T08:15:02.117264118Z","level":"INFO","source":{"function":"github.com/unifabric-io/unifabric/pkg/switchagent.(*Agent).Start","file":"/unifabric/pkg/switchagent/agent.go","line":88},"msg":"start switch agent","switchName":"leaf1","listenAddress":":8090","mtlsEnabled":true,"lldpRefreshInterval":"10s","lldpCollectionMode":"socket","lldpSocketPath":"/run/lldpd.socket","lldpCLIVersion":"1.0.16"}
```

The default deployment may not be usable, or you may need to deploy many
switches. Then choose the deployment from three independent choices:

- Run type: if the switch can run Docker, run switch-agent as a container. If it
  cannot, run it as a systemd service.
- LLDP collection mode, for containers only: `socket` mode mounts the host
  `/run/lldpd.socket` and needs no privileges. It is the default. `hostProc` mode
  does not need the socket, but it needs `--privileged`, host network, host UTS,
  and the host `/proc`.
- Delivery: run the commands on each switch by hand, or deploy many switches from
  one management host with the deployment script. The script supports both LLDP
  collection modes. `LLDP_COLLECTION_MODE` selects the mode.

| Run type | LLDP collection mode | Switch requirements | Manual, one switch at a time | Script, many switches |
| --- | --- | --- | --- | --- |
| Container | `socket` (default) | Docker. The host exposes `/run/lldpd.socket` and allows a container to mount it. | The default deployment above | [Option 1](#deployment-option-1-deploy-several-switches-with-the-script), `LLDP_COLLECTION_MODE=socket` |
| Container | `hostProc` | Docker. A container can run with `--privileged`, host network, and host UTS. | [Option 2](#deployment-option-2-run-the-container-in-hostproc-mode) | [Option 1](#deployment-option-1-deploy-several-switches-with-the-script), `LLDP_COLLECTION_MODE=hostProc` |
| systemd service | Calls the local `lldpcli` directly | No Docker. `lldpcli` is installed. | [Option 3](#deployment-option-3-install-as-a-systemd-service) | Not supported |

Prefer `socket` mode, because it needs the fewest permissions. Select `hostProc`
mode only if the switch OS does not expose `/run/lldpd.socket`, or if the socket
permissions block container access.

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
   srwxrwx--- 1 _lldpd _lldpd 0 Oct 20 08:10 /run/lldpd.socket
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

   > Note: The SSH key authenticates only the login. The script still runs
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

   > Note: Password mode assumes that the SSH password is also the sudo
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
   CONTAINER ID   IMAGE                                                COMMAND                  CREATED         STATUS         PORTS                                         NAMES
   3f1c9d2e8b7a   ghcr.io/unifabric-io/unifabric-switch-agent:v0.4.2   "/usr/bin/unifabric/…"   5 minutes ago   Up 5 minutes   0.0.0.0:8090->8090/tcp, [::]:8090->8090/tcp   unifabric-switch-agent
   {"time":"2026-10-20T08:15:02.114702361Z","level":"INFO","source":{"function":"main.main","file":"/unifabric/cmd/switch-agent/main.go","line":20},"msg":"load switch-agent config","source":"env"}
   {"time":"2026-10-20T08:15:02.115380925Z","level":"INFO","source":{"function":"main.main","file":"/unifabric/cmd/switch-agent/main.go","line":44},"msg":"start switch agent"}
   {"time":"2026-10-20T08:15:02.117264118Z","level":"INFO","source":{"function":"github.com/unifabric-io/unifabric/pkg/switchagent.(*Agent).Start","file":"/unifabric/pkg/switchagent/agent.go","line":88},"msg":"start switch agent","switchName":"leaf1","listenAddress":":8090","mtlsEnabled":true,"lldpRefreshInterval":"10s","lldpCollectionMode":"socket","lldpSocketPath":"/run/lldpd.socket","lldpCLIVersion":"1.0.16"}
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

   > Warning: The container runs with `--privileged`, host network, host UTS,
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
   9a4e7b10c3d5   ghcr.io/unifabric-io/unifabric-switch-agent:v0.4.2   "/usr/bin/unifabric/…"   2 minutes ago   Up 2 minutes             unifabric-switch-agent
   {"time":"2026-10-20T08:15:02.114702361Z","level":"INFO","source":{"function":"main.main","file":"/unifabric/cmd/switch-agent/main.go","line":20},"msg":"load switch-agent config","source":"env"}
   {"time":"2026-10-20T08:15:02.115380925Z","level":"INFO","source":{"function":"main.main","file":"/unifabric/cmd/switch-agent/main.go","line":44},"msg":"start switch agent"}
   {"time":"2026-10-20T08:15:02.117264118Z","level":"INFO","source":{"function":"github.com/unifabric-io/unifabric/pkg/switchagent.(*Agent).Start","file":"/unifabric/pkg/switchagent/agent.go","line":88},"msg":"start switch agent","switchName":"leaf1","listenAddress":":8090","mtlsEnabled":true,"lldpRefreshInterval":"10s","lldpCollectionMode":"hostProc","lldpSocketPath":"/run/lldpd.socket","lldpCLIVersion":"1.0.16"}
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
   unifabric-switch-agent_v0.4.2_linux_amd64.tar.gz: OK
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
   systemctl is-active unifabric-switch-agent
   journalctl -u unifabric-switch-agent -n 100 --no-pager
   ```

   ```text
   active
   Oct 20 08:20:11 leaf1 unifabric-switch-agent[2817]: {"time":"2026-10-20T08:20:11.114702361Z","level":"INFO","source":{"function":"main.main","file":"github.com/unifabric-io/unifabric/cmd/switch-agent/main.go","line":20},"msg":"load switch-agent config","source":"env"}
   Oct 20 08:20:11 leaf1 unifabric-switch-agent[2817]: {"time":"2026-10-20T08:20:11.115380925Z","level":"INFO","source":{"function":"main.main","file":"github.com/unifabric-io/unifabric/cmd/switch-agent/main.go","line":44},"msg":"start switch agent"}
   Oct 20 08:20:11 leaf1 unifabric-switch-agent[2817]: {"time":"2026-10-20T08:20:11.117264118Z","level":"INFO","source":{"function":"github.com/unifabric-io/unifabric/pkg/switchagent.(*Agent).Start","file":"github.com/unifabric-io/unifabric/pkg/switchagent/agent.go","line":88},"msg":"start switch agent","switchName":"leaf1","listenAddress":":8090","mtlsEnabled":true,"lldpRefreshInterval":"10s","lldpCollectionMode":"socket","lldpSocketPath":"/run/lldpd.socket","lldpCLIVersion":"1.0.16"}
   ```

### Step 3: Create the Switch Resources

After the agent runs, go to the host with `kubectl` access. Create one Kubernetes
`Switch` resource for each physical switch. The Controller connects to
`spec.mgmtIP` and `spec.grpcPort` to read the LLDP snapshot. The `192.0.2.0/24`
addresses below are only examples. Replace them with the real management IP of
each switch:

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

Repeat for the other switches. For the switches of the Storage network, set `role`
to `Storage`.

> Note: `metadata.name` does not have to match the switch hostname.

> The Controller uses `Switch.status.hostname` to match the switch-side data with
> the hostname that the node LLDP discovered.

> Do not add the `unifabric.io/switch-neighbors` annotation to any `Switch` of the
> same role. That annotation selects [Custom network topology](./custom-network-topology.md).

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

The switch side is complete when two conditions are true for every `Switch`.
`HEALTHY` is `true`, and `NEIGHBORS` matches the real neighbor count of the switch.

The final result is the Node labels and the `Topology` resource. To view them, see
[Network topology discovery](./README.md#view-topology-and-node-labels).

## Troubleshooting

### The Log Shows lldpcli command not found

First make sure that the prerequisites are true. Log in to the switch and run:

```bash
lldpcli -f json0 show neighbors
```

If the command fails, install `lldpd` and `lldpcli` on the switch. The command can
work in an interactive shell while the systemd service still fails. In this case,
make sure that the `PATH` line in the service file includes the directory of
`lldpcli`.

### Switch.status.healthy Stays false

1. Make sure that the Controller can reach port `8090`, or the configured
   `grpcPort`, on the management IP of the switch.
2. Make sure that the mTLS files on the switch match the Helm-generated Secret
   `switch-controller-mtls-agent`. After a Helm reinstall, export the files again.

### The Script Reports a Missing Local Image

The script never runs `docker pull`. Import the correct image version into Docker
on that switch. Then run the script again.

### The Script Fails at Remote sudo

Make sure that the remote user has root permission for Docker and for the
certificate directory. If the sudo password is different from the SSH password,
set `SUDO_PASSWORD` before the run.

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

- [Network topology discovery](./README.md): the discovery methods and how to view
  the result.
- [Custom network topology](./custom-network-topology.md): declare the switch neighbors instead of
  running switch-agent.
- [Switch reference](../../reference/switch.md): the `spec` and `status` fields of
  the `Switch` resource.
- [Scale-out topology design](../../design/scaleout-topology.md): how the
  Controller uses the switch-side LLDP snapshot.
- [deploy-switch-agent.sh](../../../tools/deploy-switch-agent/deploy-switch-agent.sh):
  the batch deployment script.

The NVIDIA documents below describe the management networks in an AI cluster,
and how network management software reaches the out-of-band management network
of the switches:

- [DGX SuperPOD GB200 reference architecture: Network Fabrics](https://docs.nvidia.com/dgx-superpod/reference-architecture-scalable-infrastructure-gb200/latest/network-fabrics.html):
  the in-band segment dedicated to access to the out-of-band devices.
- [DGX SuperPOD GB300 reference architecture (PDF)](https://docs.nvidia.com/pdf/dgx-spod-gb300-ra.pdf):
  the separate out-of-band network, its uplink, and route leaking.
- [DGX SuperPOD Spectrum-X reference architecture: Vera Rubin NVL72 (PDF)](https://docs.nvidia.com/dgx-superpod-spectrum-x-reference-architecture-with-dgx-vera-rubin-nvl72.pdf):
  the switch management network inside the out-of-band network.
- [DGX SuperPOD H100 reference architecture: Network Fabrics](https://docs.nvidia.com/dgx-superpod/reference-architecture-scalable-infrastructure-h100/latest/network-fabrics.html):
  the connection between the out-of-band network and the in-band network.
- [NetQ: Install NetQ Agents](https://docs.nvidia.com/networking-ethernet-software/cumulus-netq-49/Installation-Management/Install-NetQ/Install-NetQ-Agents/):
  the NetQ Agent on a switch connects to the NetQ server through the management VRF.
- [UFM Enterprise: UFM Communication Requirements](https://networking-docs.nvidia.com/ufmenterpriseum/6.26.1/ufm-communication-requirements):
  the UFM server manages InfiniBand switches over the out-of-band management network.
