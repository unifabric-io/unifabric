#!/usr/bin/env bash

set -euo pipefail

NVAIR_BIN="${NVAIR_BIN:-nvair}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOPOLOGY_DIR="${TOPOLOGY_DIR:-$(cd "${SCRIPT_DIR}/.." && pwd)}"
NODE_LIST=""
SIMULATION_NAME="${SIMULATION_NAME:-}"

usage() {
  cat <<'EOF'
Usage:
  setup-rdma-rxe.sh [-s <simulation>] [node,node,...]

Examples:
  setup-rdma-rxe.sh --simulation my-simulation node-1,node-2
  setup-rdma-rxe.sh --simulation my-simulation

Notes:
  - If you pass a comma-separated node list, this script uses it directly.
  - If you omit the node list, GPU nodes are discovered from the simulation.
  - Simulation defaults to SIMULATION_NAME or unifable-e2e-topology.
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

resolve_simulation_name() {
  printf '%s\n' "${SIMULATION_NAME:-unifable-e2e-topology}"
}

discover_gpu_nodes() {
  local simulation_name="$1"
  local discovered=()

  mapfile -t discovered < <(
    "${NVAIR_BIN}" get nodes --simulation "${simulation_name}" \
      | awk 'NR>1 && ($1 ~ /^node-gpu-/ || $1 ~ /^gpu-node-/) { print $1 }' \
      | sort -V
  )

  if (( ${#discovered[@]} == 0 )); then
    echo "No GPU nodes found for RDMA RXE setup in simulation ${simulation_name}." >&2
    exit 1
  fi

  printf '%s\n' "${discovered[@]}"
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

while (($# > 0)); do
  case "$1" in
    -s|--simulation)
      if (($# < 2)) || [[ -z "${2:-}" ]]; then
        echo "Missing value for $1" >&2
        usage >&2
        exit 2
      fi
      SIMULATION_NAME="${2}"
      shift 2
      ;;
    -*)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "${NODE_LIST}" ]]; then
        echo "Node list must be comma-separated in a single argument, e.g. node-1,node-2." >&2
        usage >&2
        exit 2
      fi
      NODE_LIST="${1}"
      shift
      ;;
  esac
done

require_command "${NVAIR_BIN}"

NODES=()
if [[ -n "${NODE_LIST}" ]]; then
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
else
  SIMULATION_NAME="$(resolve_simulation_name)"
  mapfile -t NODES < <(discover_gpu_nodes "${SIMULATION_NAME}")
fi

if (( ${#NODES[@]} == 0 )); then
  echo "No nodes found. Pass comma-separated node names, or use --simulation for dynamic discovery." >&2
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
sudo -n apt-get update
if [[ "${running_kernel}" == "6.8.0-90-generic" ]]; then
  # linux-modules-extra for this kernel is absent from the archive; pull the
  # last published build directly from Launchpad and let apt resolve the rest.
  download_dir="$(mktemp -d)"
  cleanup() {
    rm -rf "${download_dir}"
  }
  trap cleanup EXIT

  curl -fsSL --retry 5 --retry-delay 2 -o "${download_dir}/linux-modules-extra.deb" "https://launchpadlibrarian.net/832146409/linux-modules-extra-6.8.0-90-generic_6.8.0-90.91_amd64.deb"
  sudo -n apt-get install -y wireless-regdb rdma-core
  sudo -n dpkg -i "${download_dir}/linux-modules-extra.deb"
else
  sudo -n apt-get install -y "linux-modules-extra-${running_kernel}" rdma-core
fi
sudo -n modprobe rdma_rxe

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
