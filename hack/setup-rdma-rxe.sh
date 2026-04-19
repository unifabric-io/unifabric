#!/usr/bin/env bash

set -euo pipefail

# Comma-separated node list. You can edit it, or pass one as an argument:
#   NODE_LIST="node-1,node-2"
#   hack/setup-rdma-rxe.sh --simulation my-simulation node-1,node-2
NODE_LIST=""
SIMULATION_NAME="${SIMULATION_NAME:-}"

usage() {
  cat <<'EOF'
Usage:
  setup-rdma-rxe.sh [-s <simulation>] [node,node,...]

Examples:
  setup-rdma-rxe.sh --simulation my-simulation node-1,node-2

Notes:
  - If you pass a comma-separated node list, this script uses it instead of NODE_LIST.
  - If you pass a simulation name, this script adds it to each nvair exec call.
  - This script runs commands non-interactively with:
      nvair exec <node> --simulation <simulation> -- bash -lc '<script>'
EOF
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
  echo "No nodes found. Edit NODE_LIST, or pass comma-separated node names." >&2
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

sudo -n apt-get update
sudo -n apt-get install -y "linux-modules-extra-$(uname -r)" rdma-core
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
  nvair exec "${node}" "${NVAIR_SIMULATION_ARGS[@]}" -- bash -lc "${REMOTE_SCRIPT}"
done

printf -v COMPLETED_NODES '%s,' "${NODES[@]}"
COMPLETED_NODES="${COMPLETED_NODES%,}"
echo "Success: RDMA RXE setup completed on ${#NODES[@]} node(s): ${COMPLETED_NODES}"
