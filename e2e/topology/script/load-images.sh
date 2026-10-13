#!/usr/bin/env bash

set -euo pipefail

SIMULATION_NAME="${SIMULATION_NAME:-unifable-e2e-topology}"
IMAGE_ARTIFACT_DIR="${IMAGE_ARTIFACT_DIR:-}"
SWITCH_SUDO_PASSWORD="${SWITCH_SUDO_PASSWORD:-Dangerous1#}"

usage() {
  cat <<'EOF'
Usage:
  bash e2e/topology/script/load-images.sh

Required environment variables:
  IMAGE_ARTIFACT_DIR             Directory containing unifabric image tar artifacts.

Optional environment variables:
  SIMULATION_NAME               Simulation name. Default: unifable-e2e-topology
  SWITCH_NAMES                  Optional comma/space-separated switch names.
  SWITCH_SUDO_PASSWORD          Switch sudo password for Cumulus VX images.

Run after install-topology with KUBECONFIG pointing to the remote cluster.
Images are copied via nvair using the existing NVIDIA Air SSH configuration.
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if (( $# > 0 )) || [[ -z "${IMAGE_ARTIFACT_DIR}" ]]; then
  usage >&2
  exit 2
fi

for command in nvair kubectl; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    echo "Missing required command: ${command}" >&2
    exit 1
  fi
done

for image in unifabric-controller unifabric-agent unifabric-switch-agent; do
  if [[ ! -s "${IMAGE_ARTIFACT_DIR}/${image}.tar" ]]; then
    echo "Image archive not found or empty: ${IMAGE_ARTIFACT_DIR}/${image}.tar" >&2
    exit 1
  fi
done

SIM_ARGS=(-s "${SIMULATION_NAME}")
nodes="$(kubectl get nodes -o 'jsonpath={range .items[*]}{.metadata.name}{"\n"}{end}')"
mapfile -t KUBE_NODES < <(printf '%s\n' "${nodes}" | awk 'NF')
if [[ -n "${SWITCH_NAMES:-}" ]]; then
  read -r -a SWITCHES <<< "${SWITCH_NAMES//,/ }"
else
  nodes="$(nvair get nodes "${SIM_ARGS[@]}")"
  mapfile -t SWITCHES < <(printf '%s\n' "${nodes}" | awk 'NR>1 && $1 ~ /^switch-/ {print $1}')
fi

if (( ${#KUBE_NODES[@]} == 0 || ${#SWITCHES[@]} == 0 )); then
  echo "No Kubernetes nodes or switches found for image loading." >&2
  exit 1
fi

load_image() {
  local node="$1"
  local image="$2"
  local runtime="$3"
  local remote_path="/tmp/${image}.$$.tar"
  local path_quoted password_quoted
  printf -v path_quoted '%q' "${remote_path}"
  printf -v password_quoted '%q' "${SWITCH_SUDO_PASSWORD}"

  echo "[load-images] Loading ${image} into ${runtime} on ${node}..."
  nvair cp "${SIM_ARGS[@]}" "${IMAGE_ARTIFACT_DIR}/${image}.tar" "${node}:${remote_path}"
  if [[ "${runtime}" == "containerd" ]]; then
    nvair exec "${node}" "${SIM_ARGS[@]}" -- bash -c "set -euo pipefail
trap 'rm -f ${path_quoted}' EXIT
sudo ctr -n k8s.io images import --platform linux/amd64 ${path_quoted}"
  else
    nvair exec "${node}" "${SIM_ARGS[@]}" -- bash -c "set -euo pipefail
trap 'rm -f ${path_quoted}' EXIT
printf '%s\n' ${password_quoted} | sudo -S -p '' -- docker load -i ${path_quoted}"
  fi
}

for node in "${KUBE_NODES[@]}"; do
  load_image "${node}" unifabric-controller containerd
  load_image "${node}" unifabric-agent containerd
done

for node in "${SWITCHES[@]}"; do
  load_image "${node}" unifabric-switch-agent docker
done
