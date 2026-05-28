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
SFLOW_REGISTRY="${SFLOW_REGISTRY:-}"
SFLOW_REPOSITORY="${SFLOW_REPOSITORY:-}"
SFLOW_TAG="${SFLOW_TAG:-}"
SFLOW_NODE_PORT="${SFLOW_NODE_PORT:-30936}"
CHART_PATH_INPUT="${CHART_PATH:-chart}"

HELM_TIMEOUT="15m"
ROLLOUT_ID="${ROLLOUT_ID:-$(date -u +%Y%m%d%H%M%S)}"

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
  SFLOW_REGISTRY        sFlow collector image registry.
  SFLOW_REPOSITORY      sFlow collector image repository.
  SFLOW_TAG             sFlow collector image tag.

Optional environment variables:
  CHART_PATH                    Helm chart path. Default: chart
  SFLOW_NODE_PORT               Fixed UDP NodePort for the sFlow Service.
                                Default: 30936
  ROLLOUT_ID                    Pod template rollout marker. Defaults to the
                                current UTC timestamp so each run restarts Pods.

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

require_node_port() {
  local name="$1"
  local value="${!name}"

  if [[ ! "${value}" =~ ^[0-9]+$ ]]; then
    echo "${name} must be an integer NodePort (got: ${value})" >&2
    exit 2
  fi
  if (( value < 30000 || value > 32767 )); then
    echo "${name} must be within the default Kubernetes NodePort range 30000-32767 (got: ${value})" >&2
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
require_non_empty SFLOW_REGISTRY
require_non_empty SFLOW_REPOSITORY
require_non_empty SFLOW_TAG
require_node_port SFLOW_NODE_PORT

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

  --set-string nodeTopologyDiscovery.scaleOutInterfaceSelector="interface=eth1\\,eth2\\,eth3\\,eth4\\,eth5\\,eth6\\,eth7\\,eth8"
  --set-string nodeTopologyDiscovery.storageInterfaceSelector=interface=eth9

  --set-string controller.image.registry="${CONTROLLER_REGISTRY}"
  --set-string controller.image.repository="${CONTROLLER_REPOSITORY}"
  --set-string controller.image.tag="${CONTROLLER_TAG}"
  --set-string controller.image.pullPolicy=Always
  --set-string "controller.podAnnotations.e2e\\.unifabric\\.io/rollout=${ROLLOUT_ID}"

  --set-string agent.image.registry="${AGENT_REGISTRY}"
  --set-string agent.image.repository="${AGENT_REPOSITORY}"
  --set-string agent.image.tag="${AGENT_TAG}"
  --set-string agent.image.pullPolicy=Always
  --set-string "agent.podAnnotations.e2e\\.unifabric\\.io/rollout=${ROLLOUT_ID}"

  --set-string agent.lldp.image.registry="${AGENT_REGISTRY}"
  --set-string agent.lldp.image.repository="${AGENT_REPOSITORY}"
  --set-string agent.lldp.image.tag="${AGENT_TAG}"
  --set-string agent.lldp.image.pullPolicy=Always

  --set sflow.enabled=true
  --set-string sflow.image.registry="${SFLOW_REGISTRY}"
  --set-string sflow.image.repository="${SFLOW_REPOSITORY}"
  --set-string sflow.image.tag="${SFLOW_TAG}"
  --set-string sflow.image.pullPolicy=Always
  --set-string "sflow.podAnnotations.e2e\\.unifabric\\.io/rollout=${ROLLOUT_ID}"
  --set-string sflow.service.type=NodePort
  --set sflow.service.nodePort="${SFLOW_NODE_PORT}"
  --set sflow.clickhouse.managed.enabled=true
  --set-string sflow.clickhouse.managed.image.pullPolicy=Always
  --set-string "sflow.clickhouse.managed.podAnnotations.e2e\\.unifabric\\.io/rollout=${ROLLOUT_ID}"
  --set sflow.clickhouse.managed.persistence.enabled=true
  --set-string sflow.clickhouse.managed.persistence.type=hostPath
  --set-string sflow.clickhouse.managed.persistence.hostPath.path=/var/lib/unifabric/clickhouse
  --set-string sflow.clickhouse.managed.persistence.hostPath.type=DirectoryOrCreate
  --set-string "sflow.clickhouse.managed.nodeSelector.kubernetes\\.io/hostname=node-gpu-4"

  --set nvidiaTopograph.enable=false
  --set grafanaDashboard.enabled=true
  --set-string grafanaDashboard.kind=GrafanaDashboard

  --set switchTopologyDiscovery.enabled=true

  --set switchTopologyDiscovery.mtls.enabled=true
  --set switchTopologyDiscovery.mtls.autoGenerate=true
  --set-string switchTopologyDiscovery.mtls.controllerSecretName=switch-controller-mtls-controller
  --set-string switchTopologyDiscovery.mtls.switchAgentSecretName=switch-controller-mtls-agent

)

printf '%q' helm
printf ' %q' "${HELM_ARGS[@]}"
printf '\n'
helm "${HELM_ARGS[@]}"
