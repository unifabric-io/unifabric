# Deploy switch-agent on Multiple Switches with the Script

中文版: [deploy-switch-agent-script.zh.md](./deploy-switch-agent-script.zh.md)

The project provides
[deploy-switch-agent.sh](../../tools/deploy-switch-agent/deploy-switch-agent.sh) to copy pinned
mTLS certificates from one management host and deploy the switch-agent container on multiple
switches. By default, the script mounts `/run/lldpd.socket` for LLDP collection. It can also use
`hostProc` mode.

The script stores no target address or password and does not pull from an image registry. After it
starts, it automatically:

1. Checks for `peer.crt`, `tls.crt`, and `tls.key` on the management host.
2. Copies the certificates to every target over SSH/SCP.
3. Uses sudo to install them under `/opt/unifabric-switch-agent/mtls`. Directories and files are
   owned by `root:root`; certificates use mode `0644`, and the private key uses `0600`.
4. Checks that the requested image already exists in Docker on the target switch.
5. Removes and recreates an existing `unifabric-switch-agent` container, or creates it when absent.
6. Continues with the other switches and reports every failed target at the end.

The script deploys only the agent on the switches. It does not create Kubernetes `Switch` CRs.

## Prerequisites

The management host may be any Kubernetes node or a separate Linux host. In
either case, it must be able to reach the target switches' management network
and the Kubernetes API used to export the switch-agent mTLS certificates.

The management host needs:

- Bash, `ssh`, and `scp`.
- Network access to every target switch management IP and SSH port.
- `sshpass` only when password authentication is used.
- A configured `kubectl` context to export the mTLS certificates.

Every switch needs:

- SONiC OS 2025+ or NVIDIA Cumulus Linux 4.x.x+.
- Docker installed and available to the remote user through sudo.
- The switch-agent image must be preloaded in local Docker before deployment. The script never runs
  `docker pull`.
- Every switch host must expose `/run/lldpd.socket` as a Unix socket for the default socket mode.
- Working switch-local `lldpd` and `lldpcli` for `hostProc` mode.
- Non-interactive SSH commands that enter a Linux shell.

## Deployment Workflow

Complete the deployment in this order:

1. Export the switch-agent mTLS certificates on the management host.
2. Select the LLDP collection mode for this deployment on the management host.
3. Specify the target switches, select one SSH authentication method, and run the deployment script
   on the management host.
4. After the script succeeds, create a Kubernetes `Switch` CR for every physical switch.
5. Verify the container on each switch and the subscription and LLDP state from the management host.

Unless a step explicitly says to run a command on a target switch, run all commands below on the
management host.

### Step 1: Export the mTLS Certificates on the Management Host

Create a local certificate directory on the management host, then export the switch-agent
`peer.crt`, `tls.crt`, and `tls.key` from the Kubernetes Secret:

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

The deployment command later uses `CERT_SOURCE_DIR=./tmp-switch-mtls` to reference this directory.

### Step 2: Select the LLDP Collection Mode on the Management Host

Set the following environment variables on the management host. The deployment script uses them
when it creates the container on every target switch.

The lower-privilege socket mount is the default and requires no environment variable.
`LLDP_SOCKET_PATH` is the path to the `lldpd` Unix socket on every target switch host; it is not a
path on the management host. Before deployment, log in to every switch and confirm that the host
can see the socket:

```bash
ls -l /run/lldpd.socket
```

The first character of the permission string should be `s`, indicating a Unix socket.

To set the collection mode and switch-host socket path explicitly on the management host:

```bash
export LLDP_COLLECTION_MODE=socket
export LLDP_SOCKET_PATH=/run/lldpd.socket
```

The script first checks that the path is a socket on every switch host, then mounts it at the same
path in the switch-agent container and publishes gRPC with `-p 8090:8090`. If a switch host cannot
see or mount the `lldpd` socket, select `hostProc`:

```bash
export LLDP_COLLECTION_MODE=hostProc
```

`hostProc` uses host network, host UTS, `--privileged`, and the host `/proc`. `GRPC_PORT` changes
the switch-agent listen port; socket mode also updates the Docker port mapping.

### Step 3: Specify the Targets and Run the Deployment Script on the Management Host

Choose either SSH key or SSH password authentication, depending on how the management host logs in
to the switches. The choice affects only SSH login and remote sudo authentication; the certificate,
target, image, and LLDP settings remain the same.

When running the script, pass the comma-separated switch management IPs through `HOSTS`. The
following commands use addresses from the RFC 5737 documentation range; replace them with the
actual management IPs.

#### Option A: Use an SSH Key

SSH key authentication is the default:

```bash
export SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>"

SSH_USER=your-ssh-user \
HOSTS="192.0.2.11,192.0.2.12,192.0.2.21" \
CERT_SOURCE_DIR=./tmp-switch-mtls \
./tools/deploy-switch-agent/deploy-switch-agent.sh
```

An SSH key authenticates the login only. The script still runs remote `sudo` to install
certificates and operate Docker:

- No extra option is needed when the remote user has passwordless sudo.
- When sudo requires a password, add `SUDO_AUTH_MODE=password`. The script prompts once, with the
  input hidden from the terminal.

```bash
SSH_USER=your-ssh-user \
SUDO_AUTH_MODE=password \
HOSTS="192.0.2.11,192.0.2.12,192.0.2.21" \
CERT_SOURCE_DIR=./tmp-switch-mtls \
SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>" \
./tools/deploy-switch-agent/deploy-switch-agent.sh
```

#### Option B: Use an SSH Password

Set `SSH_AUTH_MODE=password`. When `SSH_PASSWORD` is not already set, the script securely prompts
for it on the terminal instead of storing it in the script or target list:

```bash
SSH_AUTH_MODE=password \
SSH_USER=your-ssh-user \
HOSTS="192.0.2.11,192.0.2.12,192.0.2.21" \
CERT_SOURCE_DIR=./tmp-switch-mtls \
SWITCH_AGENT_IMAGE="ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>" \
./tools/deploy-switch-agent/deploy-switch-agent.sh
```

Password mode assumes that the SSH password is also the sudo password. Set `SUDO_PASSWORD`
separately before running when they differ.

#### Deployment Option Reference

Pass all of these options to the deployment script as environment variables on the management host:

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
| `LLDP_SOCKET_PATH` | `/run/lldpd.socket` | Path to the `lldpd` Unix socket on each target switch host; socket mode mounts it at the same path in the container. |
| `GRPC_PORT` | `8090` | switch-agent listen port; socket mode also maps this port. |
| `REMOTE_UPLOAD_DIR` | `/tmp/unifabric-switch-agent-<user>` | Temporary certificate upload directory. |
| `REMOTE_CERT_DIR` | `/opt/unifabric-switch-agent/mtls` | Final certificate directory. |
| `CONTAINER_NAME` | `unifabric-switch-agent` | Docker container name. |

### Step 4: Create the Switch CRs from the Management Host

The script creates containers on the switches only. After it succeeds, use `kubectl` on the
management host to create a corresponding Kubernetes `Switch` CR for every physical switch. Save
this example as `switch-leaf1.yaml`:

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

Apply it from the management host:

```bash
kubectl apply -f switch-leaf1.yaml
```

Repeat this process for the remaining physical switches, keeping each `spec.mgmtIP` consistent with
the management IP passed through `HOSTS` when running the deployment script.

Do not add the `unifabric.io/switch-neighbors` annotation in fully automatic mode.
`metadata.name` may be a business-facing name, but FabricNode LLDP hostnames must match either the
Switch CR name or `status.hostname` reported by switch-agent.

### Step 5: Verify the Deployment

Log in to each target switch and check the switch-agent container and its logs:

```bash
docker ps --filter name=unifabric-switch-agent
docker logs --tail 100 unifabric-switch-agent
```

Return to the management host and use `kubectl` to check subscription and LLDP status:

```bash
kubectl get switches -o wide
kubectl get switch <switch-name> -o yaml
```

If the script reports a missing local image, import the correct image version on that switch and
run the script again. If sudo fails, confirm that the remote user has root access for Docker and
the certificate directory, and set `SUDO_PASSWORD` correctly.

## LLDP Collection Modes and Privileges

The default socket mode mounts only the `lldpd` socket and mTLS directory. It does not use
privileged, host network, or host UTS. The script enables those permissions and mounts the host
`/proc` read-only only when `LLDP_COLLECTION_MODE=hostProc` is selected. See
[switch-agent hostProc LLDP collection](./switch-agent-host-proc.md) for the tradeoffs.
