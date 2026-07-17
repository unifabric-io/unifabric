#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

CONTROLLER_REGISTRY="${CONTROLLER_REGISTRY:-}"
CONTROLLER_REPOSITORY="${CONTROLLER_REPOSITORY:-}"
CONTROLLER_TAG="${CONTROLLER_TAG:-}"
AGENT_REGISTRY="${AGENT_REGISTRY:-}"
AGENT_REPOSITORY="${AGENT_REPOSITORY:-}"
AGENT_TAG="${AGENT_TAG:-}"
CHART_PATH_INPUT="${CHART_PATH:-chart}"

HELM_TIMEOUT="15m"

usage() {
  cat <<'EOF'
Usage:
  e2e/topology/script/install-unifabric.sh

Required image inputs:
  CONTROLLER_REGISTRY   Controller image registry.
  CONTROLLER_REPOSITORY Controller image repository.
  CONTROLLER_TAG        Controller image tag.
  AGENT_REGISTRY        Agent and LLDP image registry.
  AGENT_REPOSITORY      Agent and LLDP image repository.
  AGENT_TAG             Agent and LLDP image tag.

Optional environment variables:
  CHART_PATH                    Helm chart path. Default: chart

Kubeconfig:
  This script does not accept KUBECONFIG_PATH.
  Use the standard KUBECONFIG environment variable if needed.
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

resolve_chart_path() {
  if [[ "${CHART_PATH_INPUT}" = /* ]]; then
    CHART_PATH="${CHART_PATH_INPUT}"
  else
    CHART_PATH="${REPO_ROOT}/${CHART_PATH_INPUT}"
  fi
}

require_command helm
require_command kubectl

resolve_chart_path

require_non_empty CONTROLLER_REGISTRY
require_non_empty CONTROLLER_REPOSITORY
require_non_empty CONTROLLER_TAG
require_non_empty AGENT_REGISTRY
require_non_empty AGENT_REPOSITORY
require_non_empty AGENT_TAG

if [[ ! -d "${CHART_PATH}" ]]; then
  echo "Helm chart path not found: ${CHART_PATH}" >&2
  exit 2
fi

HELM_ARGS=(
  upgrade --install unifabric "${CHART_PATH}"
  --namespace unifabric-system
  --create-namespace
  --wait
  --timeout "${HELM_TIMEOUT}"
  --debug

  --set-string fabricNode.scaleOutInterfaceSelector="interface=eth1\\,eth2\\,eth3\\,eth4\\,eth5\\,eth6\\,eth7\\,eth8"
  --set-string fabricNode.storageInterfaceSelector=interface=eth9

  --set-string controller.image.registry="${CONTROLLER_REGISTRY}"
  --set-string controller.image.repository="${CONTROLLER_REPOSITORY}"
  --set-string controller.image.tag="${CONTROLLER_TAG}"

  --set-string agent.image.registry="${AGENT_REGISTRY}"
  --set-string agent.image.repository="${AGENT_REPOSITORY}"
  --set-string agent.image.tag="${AGENT_TAG}"

  --set-string topoDiscovery.scaleUp.mode=manual
  --set-string topoDiscovery.scaleOut.mode=unifabric-roce
  --set-string topoDiscovery.storage.mode=unifabric-roce
  --set grafanaDashboard.enabled=true
  --set-string grafanaDashboard.kind=GrafanaDashboard

  --set switchSubscription.mtls.mode=auto

)

printf '%q' helm
printf ' %q' "${HELM_ARGS[@]}"
printf '\n'
helm "${HELM_ARGS[@]}"
