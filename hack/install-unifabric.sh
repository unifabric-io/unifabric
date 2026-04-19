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
  GRAFANA_DASHBOARD_KIND; do
  require_non_empty "${value_name}"
done

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

helm upgrade --install "${UNIFABRIC_RELEASE}" "${CHART_PATH}" \
  --namespace "${UNIFABRIC_NAMESPACE}" \
  --create-namespace \
  --wait \
  --timeout "${HELM_TIMEOUT}" \
  --debug \
  --set-string nodeTopologyDiscovery.scaleOutInterfaceSelector="${SCALE_OUT_INTERFACE_SELECTOR}" \
  --set-string nodeTopologyDiscovery.storageInterfaceSelector="${STORAGE_INTERFACE_SELECTOR}" \
  --set-string controller.image.registry="${IMAGE_REGISTRY}" \
  --set-string controller.image.repository="${IMAGE_NAMESPACE}/unifabric-controller" \
  --set-string controller.image.tag="${IMAGE_TAG}" \
  --set-string agent.image.registry="${IMAGE_REGISTRY}" \
  --set-string agent.image.repository="${IMAGE_NAMESPACE}/unifabric-agent" \
  --set-string agent.image.tag="${IMAGE_TAG}" \
  --set-string agent.lldp.image.registry="${IMAGE_REGISTRY}" \
  --set-string agent.lldp.image.repository="${IMAGE_NAMESPACE}/unifabric-agent" \
  --set-string agent.lldp.image.tag="${IMAGE_TAG}" \
  --set nvidiaTopograph.enable="${NVIDIA_TOPOGRAPH_ENABLE}" \
  --set grafanaDashboard.enabled="${GRAFANA_DASHBOARD_ENABLED}" \
  --set-string grafanaDashboard.kind="${GRAFANA_DASHBOARD_KIND}"
