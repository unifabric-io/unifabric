#!/usr/bin/env bash

set -euo pipefail

NVAIR_BIN="${NVAIR_BIN:-nvair}"
NODE_LIST="${NODE_LIST:-node-gpu-1,node-gpu-2,node-gpu-3,node-gpu-4}"
SIMULATION_NAME="${SIMULATION_NAME:-unifable-e2e-topology}"

usage() {
  cat <<'EOF'
Usage:
  setup-rdma-rxe.sh

Examples:
  SIMULATION_NAME=my-simulation NODE_LIST=node-1,node-2 bash setup-rdma-rxe.sh
  SIMULATION_NAME=my-simulation bash setup-rdma-rxe.sh

Notes:
  - NODE_LIST defaults to node-gpu-1,node-gpu-2,node-gpu-3,node-gpu-4.
  - SIMULATION_NAME defaults to unifable-e2e-topology.
  - This script runs commands non-interactively with:
      nvair exec <node> --simulation <simulation> -- bash -lc '<script>'
EOF
}

require_command() {
  if ! command -v "${1}" >/dev/null 2>&1; then
    echo "Missing required command: ${1}" >&2
    exit 1
  fi
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if (($# > 0)); then
  echo "Unexpected arguments: $*" >&2
  usage >&2
  exit 2
fi

require_command "${NVAIR_BIN}"

NODES=()
IFS=',' read -r -a parsed_nodes <<<"${NODE_LIST}"
for parsed_node in "${parsed_nodes[@]}"; do
  if [[ -n "${parsed_node}" ]]; then
    if [[ "${parsed_node}" =~ [[:space:]] ]]; then
      echo "Node names must not contain whitespace. Use comma-separated nodes, e.g. node-1,node-2." >&2
      exit 2
    fi

    NODES+=("${parsed_node}")
  fi
done

if (( ${#NODES[@]} == 0 )); then
  echo "No nodes found. Set NODE_LIST to comma-separated node names." >&2
  usage >&2
  exit 2
fi

NVAIR_SIMULATION_ARGS=()
if [[ -n "${SIMULATION_NAME}" ]]; then
  NVAIR_SIMULATION_ARGS=(--simulation "${SIMULATION_NAME}")
fi

REMOTE_SCRIPT=$(cat <<'REMOTE_SCRIPT'
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

running_kernel="$(uname -r)"
if [[ "${running_kernel}" == "6.8.0-90-generic" ]]; then
  download_dir="$(mktemp -d)"
  cleanup() {
    rm -rf "${download_dir}"
  }
  trap cleanup EXIT

  curl -fsSL --retry 5 --retry-delay 2 -o "${download_dir}/wireless-regdb.deb" "http://archive.ubuntu.com/ubuntu/pool/main/w/wireless-regdb/wireless-regdb_2025.10.07-0ubuntu1~24.04.1_all.deb"
  curl -fsSL --retry 5 --retry-delay 2 -o "${download_dir}/linux-modules-extra.deb" "https://launchpadlibrarian.net/832146409/linux-modules-extra-6.8.0-90-generic_6.8.0-90.91_amd64.deb"
  sudo -n dpkg -i "${download_dir}/wireless-regdb.deb" "${download_dir}/linux-modules-extra.deb"
  sudo -n modprobe rdma_rxe

  if ! command -v rdma >/dev/null 2>&1; then
    sudo -n apt-get update
    sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y rdma-core
  fi
else
  sudo -n apt-get update
  sudo -n apt-get install -y "linux-modules-extra-$(uname -r)" rdma-core
  sudo -n modprobe rdma_rxe
fi

for i in $(seq 1 13); do
  dev="eth${i}"

  if ! ip link show "${dev}" >/dev/null 2>&1; then
    continue
  fi

  sudo -n ip link set "${dev}" up

  if rdma link show | grep -Fq "rxe_${dev}/"; then
    echo "rxe_${dev} exists, skip"
    continue
  fi

  sudo -n rdma link add "rxe_${dev}" type rxe netdev "${dev}"
done
REMOTE_SCRIPT
)

for node in "${NODES[@]}"; do
  echo "Setup RDMA RXE on node: ${node}"
  "${NVAIR_BIN}" exec "${node}" "${NVAIR_SIMULATION_ARGS[@]}" -- bash -lc "${REMOTE_SCRIPT}"
done

printf -v COMPLETED_NODES '%s,' "${NODES[@]}"
COMPLETED_NODES="${COMPLETED_NODES%,}"
echo "Success: RDMA RXE setup completed on ${#NODES[@]} node(s): ${COMPLETED_NODES}"
