#!/usr/bin/env bash

set -euo pipefail

# Supported custom variables:
#   NVAIR_BIN
#   TOPOLOGY_DIR
#   SWITCH_AGENT_IMAGE
#   SWITCH_TOPOLOGY_ARTIFACT_DIR
#   SWITCH_SUDO_PASSWORD
#
# Kubeconfig behavior:
#   This script does not define KUBECONFIG_PATH.
#   kubectl will use the native KUBECONFIG environment variable.
#   If KUBECONFIG is unset, kubectl will use the default kubeconfig, usually ~/.kube/config.

NVAIR_BIN="${NVAIR_BIN:-nvair}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOPOLOGY_DIR="${TOPOLOGY_DIR:-$(cd "${SCRIPT_DIR}/.." && pwd)}"

SWITCH_AGENT_IMAGE="${SWITCH_AGENT_IMAGE:-}"
IMAGE_REGISTRY="${IMAGE_REGISTRY:-ghcr.io}"
IMAGE_NAMESPACE="${IMAGE_NAMESPACE:-}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
SWITCH_TOPOLOGY_ARTIFACT_DIR="${SWITCH_TOPOLOGY_ARTIFACT_DIR:-}"
SWITCH_SUDO_PASSWORD="${SWITCH_SUDO_PASSWORD:-Dangerous1#}"

# Fixed values.
UNIFABRIC_NAMESPACE="unifabric-system"
SWITCH_AGENT_SECRET_NAME="switch-controller-mtls-agent"
SWITCH_AGENT_CONTAINER_NAME="unifabric-switch-agent"
SWITCH_AGENT_REMOTE_DIR="/opt/unifabric-switch-agent"

usage() {
  cat <<'EOF'
Usage:
  e2e/topology/script/deploy-switch-agent.sh

Required environment variables:
  SWITCH_AGENT_IMAGE          Full switch-agent image to run on scale-out switches.

Optional environment variables:
  NVAIR_BIN                   Path to nvair binary. Default: nvair
  TOPOLOGY_DIR                Topology directory containing topology.json. Default: e2e/topology
  IMAGE_REGISTRY              Image registry used to derive SWITCH_AGENT_IMAGE. Default: ghcr.io
  IMAGE_NAMESPACE             Image namespace used to derive SWITCH_AGENT_IMAGE
  IMAGE_TAG                   Image tag used to derive SWITCH_AGENT_IMAGE. Default: latest
  SWITCH_TOPOLOGY_ARTIFACT_DIR  Directory containing switch-resources.yaml
  SWITCH_SUDO_PASSWORD        Switch sudo password for Cumulus VX images.

Kubeconfig:
  This script does not accept KUBECONFIG_PATH.
  Use the standard KUBECONFIG environment variable if needed.

Fixed Secret:
  unifabric-system/switch-controller-mtls-agent
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
  echo "[switch-agent-activate] $*"
}

require_command "${NVAIR_BIN}"
require_command kubectl
require_command base64

if [[ -z "${SWITCH_AGENT_IMAGE}" && -n "${IMAGE_NAMESPACE}" ]]; then
  SWITCH_AGENT_IMAGE="${IMAGE_REGISTRY}/${IMAGE_NAMESPACE}/unifabric-switch-agent:${IMAGE_TAG}"
fi

require_non_empty SWITCH_AGENT_IMAGE

if [[ ! -f "${TOPOLOGY_DIR}/topology.json" ]]; then
  echo "topology.json not found in ${TOPOLOGY_DIR}" >&2
  exit 2
fi

if [[ -n "${KUBECONFIG:-}" ]]; then
  echo "Kubeconfig: ${KUBECONFIG}"
else
  echo "Kubeconfig: kubectl default"
fi

log "Topology dir: ${TOPOLOGY_DIR}"
log "Secret: ${UNIFABRIC_NAMESPACE}/${SWITCH_AGENT_SECRET_NAME}"
log "Switch-agent image: ${SWITCH_AGENT_IMAGE}"
log "Switch-agent container name: ${SWITCH_AGENT_CONTAINER_NAME}"
log "Switch-agent remote dir: ${SWITCH_AGENT_REMOTE_DIR}"

SIMULATION="$(awk -F'"' '/"title"[[:space:]]*:[[:space:]]*"/ { print $4; exit }' "${TOPOLOGY_DIR}/topology.json")"

if [[ -z "${SIMULATION}" ]]; then
  echo "failed to parse simulation title from ${TOPOLOGY_DIR}/topology.json" >&2
  exit 2
fi

SIM_ARGS=(-s "${SIMULATION}")

if [[ -z "${SWITCH_TOPOLOGY_ARTIFACT_DIR}" ]]; then
  SWITCH_TOPOLOGY_ARTIFACT_DIR="${TOPOLOGY_DIR}/.artifacts/${SIMULATION}"
fi
SWITCH_RESOURCES_FILE="${SWITCH_TOPOLOGY_ARTIFACT_DIR}/switch-resources.yaml"

if [[ ! -f "${SWITCH_RESOURCES_FILE}" ]]; then
  echo "switch resources manifest not found: ${SWITCH_RESOURCES_FILE}" >&2
  echo "Run step1-install-topology first so switch-resources.yaml is generated." >&2
  exit 2
fi

log "Switch topology artifact dir: ${SWITCH_TOPOLOGY_ARTIFACT_DIR}"
log "Applying Switch CRs from ${SWITCH_RESOURCES_FILE}"
kubectl apply -f "${SWITCH_RESOURCES_FILE}"

mapfile -t SCALE_OUT_SWITCHES < <(
  "${NVAIR_BIN}" get nodes "${SIM_ARGS[@]}" \
    | awk 'NR>1 && $1 ~ /^switch-gpu-(leaf|spine)/ { print $1 }' \
    | sort -V
)

if (( ${#SCALE_OUT_SWITCHES[@]} == 0 )); then
  echo "No scale-out switches found. Expected names like switch-gpu-leaf* or switch-gpu-spine*." >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

log "Exporting switch-agent mTLS materials from Secret ${UNIFABRIC_NAMESPACE}/${SWITCH_AGENT_SECRET_NAME}..."

for filename in tls.crt tls.key peer.crt; do
  kubectl -n "${UNIFABRIC_NAMESPACE}" get secret "${SWITCH_AGENT_SECRET_NAME}" \
    -o "jsonpath={.data.${filename//./\\.}}" \
    | base64 -d > "${TMP_DIR}/${filename}"

  if [[ ! -s "${TMP_DIR}/${filename}" ]]; then
    echo "Secret ${UNIFABRIC_NAMESPACE}/${SWITCH_AGENT_SECRET_NAME} is missing non-empty key ${filename}." >&2
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

  "${NVAIR_BIN}" cp "${SIM_ARGS[@]}" "${local_path}" "${node}:${remote_tmp}"
}

install_remote_file_on_switch() {
  local node="$1"
  local local_path="$2"
  local remote_path="$3"
  local mode="$4"
  local remote_tmp="/tmp/$(basename "${remote_path}").$$"

  copy_file_to_remote_tmp "${node}" "${local_path}" "${remote_tmp}"

  "${NVAIR_BIN}" exec "${node}" "${SIM_ARGS[@]}" -- bash -c "set -euo pipefail
$(switch_sudo_function)
switch_sudo mkdir -p '$(dirname "${remote_path}")'
switch_sudo install -m '${mode}' '${remote_tmp}' '${remote_path}'
rm -f '${remote_tmp}'"
}

for switch_name in "${SCALE_OUT_SWITCHES[@]}"; do
  log "Activating switch-agent on ${switch_name}..."

  "${NVAIR_BIN}" exec "${switch_name}" "${SIM_ARGS[@]}" -- bash -c "set -euo pipefail
$(switch_sudo_function)

if ! command -v docker >/dev/null 2>&1; then
  echo 'docker not found on switch' >&2
  exit 1
fi

switch_sudo mkdir -p '$(printf '%q' "${SWITCH_AGENT_REMOTE_DIR}/mtls")'"

  install_remote_file_on_switch "${switch_name}" "${TMP_DIR}/tls.crt" "${SWITCH_AGENT_REMOTE_DIR}/mtls/tls.crt" 0644
  install_remote_file_on_switch "${switch_name}" "${TMP_DIR}/tls.key" "${SWITCH_AGENT_REMOTE_DIR}/mtls/tls.key" 0600
  install_remote_file_on_switch "${switch_name}" "${TMP_DIR}/peer.crt" "${SWITCH_AGENT_REMOTE_DIR}/mtls/peer.crt" 0644

  "${NVAIR_BIN}" exec "${switch_name}" "${SIM_ARGS[@]}" -- bash -c "set -euo pipefail
$(switch_sudo_function)

switch_sudo docker pull '$(printf '%q' "${SWITCH_AGENT_IMAGE}")'

switch_sudo docker rm -f '$(printf '%q' "${SWITCH_AGENT_CONTAINER_NAME}")' >/dev/null 2>&1 || true

switch_sudo docker run -d \
  --name '$(printf '%q' "${SWITCH_AGENT_CONTAINER_NAME}")' \
  --restart unless-stopped \
  --network host \
  --uts host \
  --privileged \
  -v /proc:/host/proc:ro \
  -v '$(printf '%q' "${SWITCH_AGENT_REMOTE_DIR}")/mtls:/etc/unifabric/switch-mtls:ro' \
  '$(printf '%q' "${SWITCH_AGENT_IMAGE}")' \
  /usr/bin/unifabric/switch-agent

switch_sudo docker ps \
  --filter name='$(printf '%q' "${SWITCH_AGENT_CONTAINER_NAME}")' \
  --format '{{.Names}}' \
  | grep -qx '$(printf '%q' "${SWITCH_AGENT_CONTAINER_NAME}")'"
done

log "Switch-agent activation complete for ${#SCALE_OUT_SWITCHES[@]} switches."
