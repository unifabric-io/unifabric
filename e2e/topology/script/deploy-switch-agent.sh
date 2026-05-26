#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SWITCH_AGENT_IMAGE="${SWITCH_AGENT_IMAGE:-}"
SIMULATION_NAME="${SIMULATION_NAME:-unifable-e2e-topology}"
SWITCH_NAMES_INPUT="${SWITCH_NAMES:-}"
SWITCH_SUDO_PASSWORD="${SWITCH_SUDO_PASSWORD:-Dangerous1#}"
SWITCH_RESOURCES_FILE="${SWITCH_RESOURCES_FILE:-${SCRIPT_DIR}/switch-resources.yaml}"
DEFAULT_SWITCHES=(
  switch-gpu-leaf1
  switch-gpu-leaf2
  switch-gpu-leaf3
  switch-gpu-leaf4
  switch-gpu-spine1
  switch-scaleup-leaf1
  switch-scaleup-leaf2
  switch-storage-leaf1
)
TARGET_SWITCHES=()

usage() {
  cat <<'EOF'
Usage:
  e2e/topology/script/deploy-switch-agent.sh

Required environment variables:
  SWITCH_AGENT_IMAGE            Full switch-agent image to run on switches.

Optional environment variables:
  SIMULATION_NAME               Simulation name. Default: unifable-e2e-topology
  SWITCH_NAMES                  Optional comma/space-separated switch names.
  SWITCH_SUDO_PASSWORD          Switch sudo password for Cumulus VX images.
  SWITCH_RESOURCES_FILE         Switch CR manifest to apply after deployment.
                                Default: e2e/topology/script/switch-resources.yaml

If SWITCH_NAMES is empty, the script uses its built-in default switch list for
the e2e topology.
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if (( $# > 0 )); then
  echo "Unknown argument: ${1}" >&2
  usage >&2
  exit 2
fi

require_command() {
  if ! command -v "${1}" >/dev/null 2>&1; then
    echo "Missing required command: ${1}" >&2
    exit 1
  fi
}

require_non_empty() {
  local name="$1"

  if [[ -z "${!name}" ]]; then
    echo "${name} must not be empty." >&2
    usage >&2
    exit 2
  fi
}

log() {
  echo "[switch-agent-deploy] $*"
}

resolve_target_switches() {
  if [[ -n "${SWITCH_NAMES_INPUT}" ]]; then
    local normalized
    normalized="${SWITCH_NAMES_INPUT//,/ }"
    read -r -a TARGET_SWITCHES <<< "${normalized}"
    return
  fi

  TARGET_SWITCHES=("${DEFAULT_SWITCHES[@]}")
}

require_command nvair
require_command kubectl
require_command base64

require_non_empty SWITCH_AGENT_IMAGE
resolve_target_switches

if [[ ! -f "${SWITCH_RESOURCES_FILE}" ]]; then
  echo "Switch resources manifest not found: ${SWITCH_RESOURCES_FILE}" >&2
  exit 2
fi

if (( ${#TARGET_SWITCHES[@]} == 0 )); then
  echo "No target switches specified." >&2
  usage >&2
  exit 2
fi

SIM_ARGS=(-s "${SIMULATION_NAME}")

log "Target switches: ${TARGET_SWITCHES[*]}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

log "Exporting switch-agent mTLS materials from Secret unifabric-system/switch-controller-mtls-agent..."

for filename in tls.crt tls.key peer.crt; do
  kubectl -n unifabric-system get secret switch-controller-mtls-agent \
    -o "jsonpath={.data.${filename//./\\.}}" \
    | base64 -d > "${TMP_DIR}/${filename}"

  if [[ ! -s "${TMP_DIR}/${filename}" ]]; then
    echo "Secret unifabric-system/switch-controller-mtls-agent is missing non-empty key ${filename}." >&2
    exit 1
  fi
done

chmod 0644 "${TMP_DIR}/tls.crt" "${TMP_DIR}/peer.crt"
chmod 0600 "${TMP_DIR}/tls.key"

switch_sudo_function() {
  local password_quoted
  printf -v password_quoted "%q" "${SWITCH_SUDO_PASSWORD}"

  cat <<EOF
switch_sudo() {
  printf '%s\n' ${password_quoted} | sudo -S -p '' -- "\$@"
}
EOF
}

copy_file_to_remote_tmp() {
  local node="$1"
  local local_path="$2"
  local remote_tmp="$3"

  nvair cp "${SIM_ARGS[@]}" "${local_path}" "${node}:${remote_tmp}"
}

install_remote_file_on_switch() {
  local node="$1"
  local local_path="$2"
  local remote_path="$3"
  local mode="$4"
  local remote_tmp="/tmp/$(basename "${remote_path}").$$"

  copy_file_to_remote_tmp "${node}" "${local_path}" "${remote_tmp}"

  nvair exec "${node}" "${SIM_ARGS[@]}" -- bash -c "set -euo pipefail
$(switch_sudo_function)
switch_sudo mkdir -p '$(dirname "${remote_path}")'
switch_sudo install -m '${mode}' '${remote_tmp}' '${remote_path}'
rm -f '${remote_tmp}'"
}

for switch_name in "${TARGET_SWITCHES[@]}"; do
  log "Deploying switch-agent on ${switch_name}..."

  nvair exec "${switch_name}" "${SIM_ARGS[@]}" -- bash -c "set -euo pipefail
$(switch_sudo_function)

if ! command -v docker >/dev/null 2>&1; then
  echo 'docker not found on switch' >&2
  exit 1
fi

switch_sudo mkdir -p /opt/unifabric-switch-agent/mtls"

  install_remote_file_on_switch "${switch_name}" "${TMP_DIR}/tls.crt" /opt/unifabric-switch-agent/mtls/tls.crt 0644
  install_remote_file_on_switch "${switch_name}" "${TMP_DIR}/tls.key" /opt/unifabric-switch-agent/mtls/tls.key 0600
  install_remote_file_on_switch "${switch_name}" "${TMP_DIR}/peer.crt" /opt/unifabric-switch-agent/mtls/peer.crt 0644

  nvair exec "${switch_name}" "${SIM_ARGS[@]}" -- bash -c "set -euo pipefail
$(switch_sudo_function)

switch_sudo docker pull '$(printf '%q' "${SWITCH_AGENT_IMAGE}")'

switch_sudo docker rm -f unifabric-switch-agent >/dev/null 2>&1 || true

if [[ ! -S /run/lldpd.socket ]]; then
  echo '/run/lldpd.socket was not found on the switch host' >&2
  exit 1
fi

set +e
container_output=\$(switch_sudo docker run -d \
  --name unifabric-switch-agent \
  --restart unless-stopped \
  --network host \
  -e UNIFABRIC_SWITCH_AGENT_SWITCH_NAME='$(printf '%q' "${switch_name}")' \
  -e UNIFABRIC_SWITCH_AGENT_LLDP_COLLECTION_MODE=socket \
  -e UNIFABRIC_SWITCH_AGENT_LLDP_SOCKET_PATH=/run/lldpd.socket \
  -e UNIFABRIC_SWITCH_AGENT_LLDP_CLI_VERSION=1.0.16 \
  -v /run/lldpd.socket:/run/lldpd.socket \
  -v /opt/unifabric-switch-agent/mtls:/etc/unifabric/switch-mtls:ro \
  '$(printf '%q' "${SWITCH_AGENT_IMAGE}")' \
  /usr/bin/unifabric/switch-agent 2>&1)
container_status=\$?
set -e
if [[ "\${container_status}" != "0" ]]; then
  printf 'docker run failed with status %s:\n%s\n' "\${container_status}" "\${container_output}" >&2
  switch_sudo docker ps -a --filter name=unifabric-switch-agent || true
  exit "\${container_status}"
fi
printf 'Started switch-agent container: %s\n' "\${container_output}"

container_status_line=\$(switch_sudo docker inspect \
  --format 'container={{.Name}}|state={{.State.Status}}|running={{.State.Running}}|image={{.Config.Image}}' \
  unifabric-switch-agent)
if [[ -z "\${container_status_line}" ]]; then
  echo 'switch-agent container was not found after docker run' >&2
  exit 1
fi
printf '%s\n' "\${container_status_line}"

switch_sudo docker ps \
  --filter name=unifabric-switch-agent \
  --format '{{.Names}}' \
  | grep -qx unifabric-switch-agent"
done

log "switch-agent deployed successfully for ${#TARGET_SWITCHES[@]} switches."
log "Applying Switch resources from ${SWITCH_RESOURCES_FILE}..."
kubectl apply -f "${SWITCH_RESOURCES_FILE}"
