#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOPOLOGY_DIR="${TOPOLOGY_DIR:-${SCRIPT_DIR}}"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

usage() {
  cat <<'EOF'
Usage:
  bash e2e/topology/setup.sh [all|step1-install-topology|step2-setup-rdma-rxe|step3-install-monitoring-operators|step4-install-unifabric|step5-deploy-switch-agent|step6-deploy-hsflowd|step7-install-spiderpool-macvlan] [args...]

Examples:
  CONTROLLER_REGISTRY=ghcr.io CONTROLLER_REPOSITORY=unifabric-io/unifabric-controller CONTROLLER_TAG=YOU_TAG \
  AGENT_REGISTRY=ghcr.io AGENT_REPOSITORY=unifabric-io/unifabric-agent AGENT_TAG=YOU_TAG \
  SWITCH_AGENT_IMAGE=ghcr.io/unifabric-io/unifabric-switch-agent:YOU_TAG \
  HSFLOWD_IMAGE=ghcr.io/unifabric-io/unifabric-hsflowd:YOU_TAG \
    bash e2e/topology/setup.sh all --delete-if-exists
  bash e2e/topology/setup.sh step1-install-topology --delete-if-exists --bootstrap-node node-gpu-1
  bash e2e/topology/setup.sh step2-setup-rdma-rxe node-gpu-1,node-gpu-2,node-gpu-3,node-gpu-4
  bash e2e/topology/setup.sh step2-setup-rdma-rxe --simulation my-simulation
  SIMULATION_NAME=my-simulation bash e2e/topology/setup.sh step3-install-monitoring-operators
  CONTROLLER_REGISTRY=ghcr.io CONTROLLER_REPOSITORY=unifabric-io/unifabric-controller CONTROLLER_TAG=YOU_TAG \
    AGENT_REGISTRY=ghcr.io AGENT_REPOSITORY=unifabric-io/unifabric-agent AGENT_TAG=YOU_TAG \
    bash e2e/topology/setup.sh step4-install-unifabric
  SWITCH_AGENT_IMAGE=ghcr.io/unifabric-io/unifabric-switch-agent:YOU_TAG \
    bash e2e/topology/setup.sh step5-deploy-switch-agent
  HSFLOWD_IMAGE=ghcr.io/unifabric-io/unifabric-hsflowd:YOU_TAG \
    bash e2e/topology/setup.sh step6-deploy-hsflowd
  bash e2e/topology/setup.sh step7-install-spiderpool-macvlan
EOF
}

resolve_bootstrap_node() {
  local args=("$@")
  local index=0

  while (( index < ${#args[@]} )); do
    case "${args[index]}" in
      -b|--bootstrap-node)
        if (( index + 1 < ${#args[@]} )) && [[ -n "${args[index + 1]}" ]]; then
          printf '%s\n' "${args[index + 1]}"
          return
        fi
        ;;
    esac
    ((index += 1))
  done

  if [[ -n "${BOOTSTRAP_NODE:-}" ]]; then
    printf '%s\n' "${BOOTSTRAP_NODE}"
  fi
}

resolve_simulation_name() {
  if [[ -n "${SIMULATION_NAME:-}" ]]; then
    printf '%s\n' "${SIMULATION_NAME}"
    return
  fi

  if [[ ! -f "${TOPOLOGY_DIR}/topology.json" ]]; then
    echo "topology.json not found in ${TOPOLOGY_DIR}" >&2
    exit 2
  fi

  local simulation_name
  simulation_name="$(awk -F'"' '/"title"[[:space:]]*:[[:space:]]*"/ { print $4; exit }' "${TOPOLOGY_DIR}/topology.json")"
  if [[ -z "${simulation_name}" ]]; then
    echo "failed to parse simulation title from ${TOPOLOGY_DIR}/topology.json" >&2
    exit 2
  fi

  printf '%s\n' "${simulation_name}"
}

run_step() {
  local script_name="$1"
  shift

  export TOPOLOGY_DIR
  export REPO_ROOT
  bash "${SCRIPT_DIR}/script/${script_name}" "$@"
}

stage="${1:-all}"
if (($# > 0)); then
  shift
fi

case "${stage}" in
  -h|--help|help)
    usage
    exit 0
    ;;
  all)
    resolved_bootstrap_node="$(resolve_bootstrap_node "$@")"
    if [[ -n "${resolved_bootstrap_node:-}" ]]; then
      export BOOTSTRAP_NODE="${resolved_bootstrap_node}"
    fi
    run_step install-topology.sh "$@"
    SIMULATION_NAME="$(resolve_simulation_name)"
    export SIMULATION_NAME
    export KUBECONFIG="${REPO_ROOT}/.tmp/kubeconfig-${SIMULATION_NAME}-external.yaml"
    run_step setup-rdma-rxe.sh --simulation "${SIMULATION_NAME}"
    run_step install-monitoring-operators.sh
    run_step install-unifabric.sh
    run_step deploy-switch-agent.sh
    run_step deploy-hsflowd.sh
    ;;
  step1-install-topology)
    run_step install-topology.sh "$@"
    ;;
  step2-setup-rdma-rxe)
    run_step setup-rdma-rxe.sh "$@"
    ;;
  step3-install-monitoring-operators)
    run_step install-monitoring-operators.sh "$@"
    ;;
  step4-install-unifabric)
    run_step install-unifabric.sh "$@"
    ;;
  step5-deploy-switch-agent)
    run_step deploy-switch-agent.sh "$@"
    ;;
  step6-deploy-hsflowd)
    run_step deploy-hsflowd.sh "$@"
    ;;
  step7-install-spiderpool-macvlan)
    run_step install-spiderpool-macvlan.sh "$@"
    ;;
  *)
    echo "Unknown stage: ${stage}" >&2
    usage >&2
    exit 2
    ;;
esac
