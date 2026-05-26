#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

UNIFABRIC_RELEASE="${UNIFABRIC_RELEASE:-unifabric}"
UNIFABRIC_NAMESPACE="${UNIFABRIC_NAMESPACE:-unifabric-system}"
CHART_PATH="${CHART_PATH:-${ROOT_DIR}/chart}"
KUBECONFIG_PATH="${KUBECONFIG_PATH:-${KUBECONFIG:-}}"
IMAGE_REGISTRY="${IMAGE_REGISTRY:-ghcr.io}"
IMAGE_NAMESPACE="${IMAGE_NAMESPACE:-unifabric-io}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
HELM_TIMEOUT="${HELM_TIMEOUT:-15m}"
SCALE_OUT_INTERFACE_SELECTOR="${SCALE_OUT_INTERFACE_SELECTOR:-interface=eth1\\,eth2\\,eth3\\,eth4\\,eth5\\,eth6\\,eth7\\,eth8}"
STORAGE_INTERFACE_SELECTOR="${STORAGE_INTERFACE_SELECTOR:-interface=eth9}"
NVIDIA_TOPOGRAPH_ENABLE="${NVIDIA_TOPOGRAPH_ENABLE:-false}"
GRAFANA_DASHBOARD_ENABLED="${GRAFANA_DASHBOARD_ENABLED:-true}"
GRAFANA_DASHBOARD_KIND="${GRAFANA_DASHBOARD_KIND:-GrafanaDashboard}"
SCALE_OUT_SWITCHES_ENABLED="${SCALE_OUT_SWITCHES_ENABLED:-false}"
SWITCH_TOPOLOGY_ARTIFACT_DIR="${SWITCH_TOPOLOGY_ARTIFACT_DIR:-}"
SWITCH_MTLS_CONTROLLER_SECRET_NAME="${SWITCH_MTLS_CONTROLLER_SECRET_NAME:-switch-controller-mtls-controller}"
SWITCH_MTLS_AGENT_SECRET_NAME="${SWITCH_MTLS_AGENT_SECRET_NAME:-switch-controller-mtls-agent}"

usage() {
  cat <<'EOF'
Usage:
  hack/install-unifabric.sh

Environment:
  UNIFABRIC_RELEASE             Helm release name. Default: unifabric
  UNIFABRIC_NAMESPACE           Kubernetes namespace. Default: unifabric-system
  CHART_PATH                    Helm chart path. Default: ./chart
  KUBECONFIG_PATH               Kubeconfig path. Default: $KUBECONFIG, or Helm default if unset
  IMAGE_REGISTRY                Image registry host. Default: ghcr.io
  IMAGE_NAMESPACE               Image namespace/owner. Default: unifabric-io
  IMAGE_TAG                     Controller and agent image tag. Default: latest
  HELM_TIMEOUT                  Helm wait timeout. Default: 15m
  SCALE_OUT_INTERFACE_SELECTOR  Scale-out interface selector for E2E topology.
  STORAGE_INTERFACE_SELECTOR    Storage interface selector for E2E topology.
  SCALE_OUT_SWITCHES_ENABLED    Enable Switch/SwitchGroup scale-out discovery. Default: false
  SWITCH_TOPOLOGY_ARTIFACT_DIR  Artifact directory produced by e2e/topology/install.sh
  SWITCH_MTLS_CONTROLLER_SECRET_NAME
                                Controller Secret name for pinned mTLS materials
  SWITCH_MTLS_AGENT_SECRET_NAME Switch-agent Secret name for pinned mTLS materials
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
    exit 2
  fi
}

require_command helm
require_command kubectl

if [[ -n "${KUBECONFIG_PATH}" ]]; then
  if [[ ! -f "${KUBECONFIG_PATH}" ]]; then
    echo "KUBECONFIG_PATH does not exist: ${KUBECONFIG_PATH}" >&2
    exit 2
  fi
  export KUBECONFIG="${KUBECONFIG_PATH}"
fi

for value_name in \
  UNIFABRIC_RELEASE \
  UNIFABRIC_NAMESPACE \
  CHART_PATH \
  IMAGE_REGISTRY \
  IMAGE_NAMESPACE \
  IMAGE_TAG \
  HELM_TIMEOUT \
  SCALE_OUT_INTERFACE_SELECTOR \
  STORAGE_INTERFACE_SELECTOR \
  NVIDIA_TOPOGRAPH_ENABLE \
  GRAFANA_DASHBOARD_ENABLED \
  GRAFANA_DASHBOARD_KIND \
  SCALE_OUT_SWITCHES_ENABLED \
  SWITCH_MTLS_CONTROLLER_SECRET_NAME \
  SWITCH_MTLS_AGENT_SECRET_NAME; do
  require_non_empty "${value_name}"
done

SCALE_OUT_SWITCHES_ENABLED="$(printf '%s' "${SCALE_OUT_SWITCHES_ENABLED}" | tr '[:upper:]' '[:lower:]')"
case "${SCALE_OUT_SWITCHES_ENABLED}" in
  true|false) ;;
  *)
    echo "SCALE_OUT_SWITCHES_ENABLED must be true or false (got: ${SCALE_OUT_SWITCHES_ENABLED})" >&2
    exit 2
    ;;
esac

create_or_update_secret() {
  local namespace="$1"
  local secret_name="$2"
  local source_dir="$3"

  kubectl -n "${namespace}" create secret generic "${secret_name}" \
    --from-file=tls.crt="${source_dir}/tls.crt" \
    --from-file=tls.key="${source_dir}/tls.key" \
    --from-file=peer.crt="${source_dir}/peer.crt" \
    --dry-run=client -o yaml | kubectl apply -f -
}

prepare_switch_topology_artifacts() {
  local controller_dir="${SWITCH_TOPOLOGY_ARTIFACT_DIR}/mtls/controller"
  local switch_agent_dir="${SWITCH_TOPOLOGY_ARTIFACT_DIR}/mtls/switch-agent"
  local switch_resources_file="${SWITCH_TOPOLOGY_ARTIFACT_DIR}/switch-resources.yaml"

  if [[ ! -d "${controller_dir}" ]]; then
    echo "Missing controller mTLS bundle directory: ${controller_dir}" >&2
    exit 2
  fi
  if [[ ! -d "${switch_agent_dir}" ]]; then
    echo "Missing switch-agent mTLS bundle directory: ${switch_agent_dir}" >&2
    exit 2
  fi
  if [[ ! -f "${switch_resources_file}" ]]; then
    echo "Missing generated Switch manifest: ${switch_resources_file}" >&2
    exit 2
  fi

  kubectl create namespace "${UNIFABRIC_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  create_or_update_secret "${UNIFABRIC_NAMESPACE}" "${SWITCH_MTLS_CONTROLLER_SECRET_NAME}" "${controller_dir}"
  create_or_update_secret "${UNIFABRIC_NAMESPACE}" "${SWITCH_MTLS_AGENT_SECRET_NAME}" "${switch_agent_dir}"
}

echo "Installing Unifabric release ${UNIFABRIC_RELEASE} into namespace ${UNIFABRIC_NAMESPACE}"
echo "Chart: ${CHART_PATH}"
if [[ -n "${KUBECONFIG_PATH}" ]]; then
  echo "Kubeconfig: ${KUBECONFIG_PATH}"
else
  echo "Kubeconfig: Helm default"
fi
echo "Controller image: ${IMAGE_REGISTRY}/${IMAGE_NAMESPACE}/unifabric-controller:${IMAGE_TAG}"
echo "Agent image: ${IMAGE_REGISTRY}/${IMAGE_NAMESPACE}/unifabric-agent:${IMAGE_TAG}"
echo "LLDP image: ${IMAGE_REGISTRY}/${IMAGE_NAMESPACE}/unifabric-agent:${IMAGE_TAG}"
echo "Switch-based discovery enabled: ${SCALE_OUT_SWITCHES_ENABLED}"
if [[ -n "${SWITCH_TOPOLOGY_ARTIFACT_DIR}" ]]; then
  echo "Switch topology artifact dir: ${SWITCH_TOPOLOGY_ARTIFACT_DIR}"
  SCALE_OUT_SWITCHES_ENABLED="true"
  prepare_switch_topology_artifacts
fi

HELM_ARGS=(
  upgrade --install "${UNIFABRIC_RELEASE}" "${CHART_PATH}"
  --namespace "${UNIFABRIC_NAMESPACE}"
  --create-namespace
  --wait
  --timeout "${HELM_TIMEOUT}"
  --debug
  --set-string nodeTopologyDiscovery.scaleOutInterfaceSelector="${SCALE_OUT_INTERFACE_SELECTOR}"
  --set-string nodeTopologyDiscovery.storageInterfaceSelector="${STORAGE_INTERFACE_SELECTOR}"
  --set-string controller.image.registry="${IMAGE_REGISTRY}"
  --set-string controller.image.repository="${IMAGE_NAMESPACE}/unifabric-controller"
  --set-string controller.image.tag="${IMAGE_TAG}"
  --set-string agent.image.registry="${IMAGE_REGISTRY}"
  --set-string agent.image.repository="${IMAGE_NAMESPACE}/unifabric-agent"
  --set-string agent.image.tag="${IMAGE_TAG}"
  --set-string agent.lldp.image.registry="${IMAGE_REGISTRY}"
  --set-string agent.lldp.image.repository="${IMAGE_NAMESPACE}/unifabric-agent"
  --set-string agent.lldp.image.tag="${IMAGE_TAG}"
  --set nvidiaTopograph.enable="${NVIDIA_TOPOGRAPH_ENABLE}"
  --set grafanaDashboard.enabled="${GRAFANA_DASHBOARD_ENABLED}"
  --set-string grafanaDashboard.kind="${GRAFANA_DASHBOARD_KIND}"
  --set scaleOutDiscovery.switches.enabled="${SCALE_OUT_SWITCHES_ENABLED}"
)

if [[ "${SCALE_OUT_SWITCHES_ENABLED}" == "true" ]]; then
  HELM_ARGS+=(--set scaleOutDiscovery.leafGroups.enabled=false)
fi

helm "${HELM_ARGS[@]}"

if [[ -n "${SWITCH_TOPOLOGY_ARTIFACT_DIR}" ]]; then
  kubectl wait --for=condition=Established --timeout=60s crd/switches.unifabric.io
  kubectl apply -f "${SWITCH_TOPOLOGY_ARTIFACT_DIR}/switch-resources.yaml"
fi
