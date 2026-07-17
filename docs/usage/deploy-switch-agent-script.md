# Deploy switch-agent on Multiple Switches with the Script

中文版: [deploy-switch-agent-script.zh.md](./deploy-switch-agent-script.zh.md)

The project provides
[deploy-switch-agent.sh](../../tools/deploy-switch-agent/deploy-switch-agent.sh) to copy pinned
mTLS certificates from one management host and deploy the switch-agent container on multiple
switches. By default, the script mounts `/run/lldpd.socket` for LLDP collection. It can also use
`hostProc` mode.

The script stores no target address or password and does not pull from an image registry. It:

1. Checks for `peer.crt`, `tls.crt`, and `tls.key` on the management host.
2. Copies the certificates to every target over SSH/SCP.
3. Uses sudo to install them under `/opt/unifabric-switch-agent/mtls`. Directories and files are
   owned by `root:root`; certificates use mode `0644`, and the private key uses `0600`.
4. Checks that the requested image already exists in Docker on the target switch.
5. Removes and recreates an existing `unifabric-switch-agent` container, or creates it when absent.
6. Continues with the other switches and reports every failed target at the end.

The script deploys only the agent on the switches. It does not create Kubernetes `Switch` CRs.

## Prerequisites

The management host needs:

- Bash, `ssh`, and `scp`.
- SSH connectivity to every target switch.
- `sshpass` only when password authentication is used.
- A configured `kubectl` context to export the mTLS certificates.

Every switch needs:

- Docker installed and available to the remote user through sudo.
- The switch-agent image preloaded in local Docker. The script never runs `docker pull`.
- `/run/lldpd.socket` on the switch host for the default socket mode.
- Working switch-local `lldpd` and `lldpcli` for `hostProc` mode.
- Non-interactive SSH commands that enter a Linux shell.

## Export the mTLS Certificates

Run on the management host:

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

## Set the Target Switches

Set `HOSTS` to a comma-separated list of management IPs:

```bash
export HOSTS="192.0.2.11,192.0.2.12,192.0.2.21"
```

These addresses use the RFC 5737 documentation range. Replace them with the actual switch
management IPs.

Confirm that the requested image already exists on each switch:

```bash
docker image inspect ghcr.io/unifabric-io/unifabric-switch-agent:<release-tag>
```

## Choose the LLDP Collection Mode

The lower-privilege socket mount is the default and needs no extra setting:

```bash
export LLDP_COLLECTION_MODE=socket
export LLDP_SOCKET_PATH=/run/lldpd.socket
```

This mode mounts the selected socket and publishes gRPC with `-p 8090:8090`. If the switch cannot
mount the `lldpd` socket, select `hostProc`:

```bash
export LLDP_COLLECTION_MODE=hostProc
```

`hostProc` uses host network, host UTS, `--privileged`, and the host `/proc`. `GRPC_PORT` changes
the switch-agent listen port; socket mode also updates the Docker port mapping.

## Deploy with an SSH Key

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

## Deploy with an SSH Password

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

## Common Options

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
| `LLDP_SOCKET_PATH` | `/run/lldpd.socket` | Host socket mounted in socket mode. |
| `GRPC_PORT` | `8090` | switch-agent listen port; socket mode also maps this port. |
| `REMOTE_UPLOAD_DIR` | `/tmp/unifabric-switch-agent-<user>` | Temporary certificate upload directory. |
| `REMOTE_CERT_DIR` | `/opt/unifabric-switch-agent/mtls` | Final certificate directory. |
| `CONTAINER_NAME` | `unifabric-switch-agent` | Docker container name. |

## Create the Switch CRs

After the script succeeds, create a corresponding Kubernetes `Switch` CR for every physical
switch:

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

Do not add the `unifabric.io/neighbors` annotation in fully automatic mode. `metadata.name` may be
a business-facing name, but FabricNode LLDP hostnames must match either the Switch CR name or
`status.hostname` reported by switch-agent.

## Verify

Check the container on a switch:

```bash
docker ps --filter name=unifabric-switch-agent
docker logs --tail 100 unifabric-switch-agent
```

Check subscription and LLDP status in Kubernetes:

```bash
kubectl get switches -o wide
kubectl get switch <switch-name> -o yaml
```

If the script reports a missing local image, import the correct image version on that switch and
run the script again. If sudo fails, confirm that the remote user has root access for Docker and
the certificate directory, and set `SUDO_PASSWORD` correctly.

## Privilege Note

The default socket mode mounts only the `lldpd` socket and mTLS directory. It does not use
privileged, host network, or host UTS. The script enables those permissions and mounts the host
`/proc` read-only only when `LLDP_COLLECTION_MODE=hostProc` is selected. See
[switch-agent hostProc LLDP collection](./switch-agent-host-proc.md) for the tradeoffs.
