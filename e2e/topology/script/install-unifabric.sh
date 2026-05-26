#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"

# Only supported custom variables:
#   CONTROLLER_IMAGE
#   AGENT_IMAGE
#   SWITCH_AGENT_IMAGE
#
# Kubeconfig behavior:
#   This script does not define KUBECONFIG_PATH.
#   kubectl and helm will use the native KUBECONFIG environment variable.
#   If KUBECONFIG is unset, they will use the default kubeconfig, usually ~/.kube/config.

CONTROLLER_IMAGE="${CONTROLLER_IMAGE:-}"
AGENT_IMAGE="${AGENT_IMAGE:-}"
SWITCH_AGENT_IMAGE="${SWITCH_AGENT_IMAGE:-}"
IMAGE_REGISTRY="${IMAGE_REGISTRY:-ghcr.io}"
IMAGE_NAMESPACE="${IMAGE_NAMESPACE:-}"
IMAGE_TAG="${IMAGE_TAG:-latest}"
CHART_PATH_INPUT="${CHART_PATH:-chart}"
SWITCH_TOPOLOGY_ARTIFACT_DIR="${SWITCH_TOPOLOGY_ARTIFACT_DIR:-}"

# Fixed values. Extract them later only when needed.
UNIFABRIC_RELEASE="unifabric"
UNIFABRIC_NAMESPACE="unifabric-system"
HELM_TIMEOUT="15m"

SCALE_OUT_INTERFACE_SELECTOR="interface=eth1\\,eth2\\,eth3\\,eth4\\,eth5\\,eth6\\,eth7\\,eth8"
STORAGE_INTERFACE_SELECTOR="interface=eth9"

NVIDIA_TOPOGRAPH_ENABLE="false"
GRAFANA_DASHBOARD_ENABLED="true"
GRAFANA_DASHBOARD_KIND="GrafanaDashboard"

# Switch-based discovery is enabled by default.
SCALE_OUT_SWITCHES_ENABLED="true"
SCALE_OUT_LEAF_GROUPS_ENABLED="false"

SWITCH_MTLS_ENABLED="true"
SWITCH_MTLS_AUTO_GENERATE="true"
SWITCH_MTLS_CONTROLLER_SECRET_NAME="switch-controller-mtls-controller"
SWITCH_MTLS_AGENT_SECRET_NAME="switch-controller-mtls-agent"

usage() {
  cat <<'EOF'
Usage:
  e2e/topology/script/install-unifabric.sh

Required environment variables:
  CONTROLLER_IMAGE      Full controller image.
  AGENT_IMAGE           Full agent image.
  SWITCH_AGENT_IMAGE    Full switch-agent image.

Alternative image environment variables:
  IMAGE_REGISTRY        Image registry. Default: ghcr.io
  IMAGE_NAMESPACE       Image namespace used to derive all three images.
  IMAGE_TAG             Image tag used to derive all three images. Default: latest

Optional environment variables:
  CHART_PATH                    Helm chart path. Default: chart
  SWITCH_TOPOLOGY_ARTIFACT_DIR  Preserved for e2e topology workflows

Kubeconfig:
  This script does not accept KUBECONFIG_PATH.
  Use the standard KUBECONFIG environment variable if needed.

Image format:
  registry/namespace/repository:tag
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

parse_image() {
  local prefix="$1"
  local image="$2"

  if [[ "${image}" != */*:* ]]; then
    echo "Invalid image format for ${prefix}: ${image}" >&2
    echo "Expected format: registry/namespace/repository:tag" >&2
    exit 2
  fi

  local without_tag="${image%:*}"
  local tag="${image##*:}"
  local registry="${without_tag%%/*}"
  local repository="${without_tag#*/}"

  if [[ -z "${registry}" || -z "${repository}" || -z "${tag}" ]]; then
    echo "Invalid image format for ${prefix}: ${image}" >&2
    echo "Expected format: registry/namespace/repository:tag" >&2
    exit 2
  fi

  if [[ "${repository}" == "${without_tag}" ]]; then
    echo "Invalid image format for ${prefix}: ${image}" >&2
    echo "Expected format: registry/namespace/repository:tag" >&2
    exit 2
  fi

  printf -v "${prefix}_REGISTRY" '%s' "${registry}"
  printf -v "${prefix}_REPOSITORY" '%s' "${repository}"
  printf -v "${prefix}_TAG" '%s' "${tag}"
}

resolve_chart_path() {
  if [[ "${CHART_PATH_INPUT}" = /* ]]; then
    CHART_PATH="${CHART_PATH_INPUT}"
  else
    CHART_PATH="${REPO_ROOT}/${CHART_PATH_INPUT}"
  fi
}

populate_default_images() {
  if [[ -z "${IMAGE_NAMESPACE}" ]]; then
    return
  fi

  if [[ -z "${CONTROLLER_IMAGE}" ]]; then
    CONTROLLER_IMAGE="${IMAGE_REGISTRY}/${IMAGE_NAMESPACE}/unifabric-controller:${IMAGE_TAG}"
  fi
  if [[ -z "${AGENT_IMAGE}" ]]; then
    AGENT_IMAGE="${IMAGE_REGISTRY}/${IMAGE_NAMESPACE}/unifabric-agent:${IMAGE_TAG}"
  fi
  if [[ -z "${SWITCH_AGENT_IMAGE}" ]]; then
    SWITCH_AGENT_IMAGE="${IMAGE_REGISTRY}/${IMAGE_NAMESPACE}/unifabric-switch-agent:${IMAGE_TAG}"
  fi
}

require_command helm
require_command kubectl

resolve_chart_path
populate_default_images

require_non_empty CONTROLLER_IMAGE
require_non_empty AGENT_IMAGE
require_non_empty SWITCH_AGENT_IMAGE

if [[ ! -d "${CHART_PATH}" ]]; then
  echo "Helm chart path not found: ${CHART_PATH}" >&2
  exit 2
fi

parse_image CONTROLLER "${CONTROLLER_IMAGE}"
parse_image AGENT "${AGENT_IMAGE}"
parse_image SWITCH_AGENT "${SWITCH_AGENT_IMAGE}"

echo "Installing Unifabric release ${UNIFABRIC_RELEASE} into namespace ${UNIFABRIC_NAMESPACE}"
echo "Chart: ${CHART_PATH}"

if [[ -n "${KUBECONFIG:-}" ]]; then
  echo "Kubeconfig: ${KUBECONFIG}"
else
  echo "Kubeconfig: kubectl/helm default"
fi

echo "Controller image: ${CONTROLLER_IMAGE}"
echo "Agent image: ${AGENT_IMAGE}"
echo "LLDP image: ${AGENT_IMAGE}"
echo "Switch agent image: ${SWITCH_AGENT_IMAGE}"
echo "Switch-based discovery enabled: ${SCALE_OUT_SWITCHES_ENABLED}"
echo "LeafGroups discovery enabled: ${SCALE_OUT_LEAF_GROUPS_ENABLED}"
echo "Switch mTLS enabled: ${SWITCH_MTLS_ENABLED}"
echo "Switch mTLS auto-generate: ${SWITCH_MTLS_AUTO_GENERATE}"
echo "Switch mTLS controller Secret: ${SWITCH_MTLS_CONTROLLER_SECRET_NAME}"
echo "Switch mTLS agent Secret: ${SWITCH_MTLS_AGENT_SECRET_NAME}"
if [[ -n "${SWITCH_TOPOLOGY_ARTIFACT_DIR}" ]]; then
  echo "Switch topology artifact dir: ${SWITCH_TOPOLOGY_ARTIFACT_DIR}"
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

  --set-string controller.image.registry="${CONTROLLER_REGISTRY}"
  --set-string controller.image.repository="${CONTROLLER_REPOSITORY}"
  --set-string controller.image.tag="${CONTROLLER_TAG}"

  --set-string agent.image.registry="${AGENT_REGISTRY}"
  --set-string agent.image.repository="${AGENT_REPOSITORY}"
  --set-string agent.image.tag="${AGENT_TAG}"

  --set-string agent.lldp.image.registry="${AGENT_REGISTRY}"
  --set-string agent.lldp.image.repository="${AGENT_REPOSITORY}"
  --set-string agent.lldp.image.tag="${AGENT_TAG}"

  --set nvidiaTopograph.enable="${NVIDIA_TOPOGRAPH_ENABLE}"
  --set grafanaDashboard.enabled="${GRAFANA_DASHBOARD_ENABLED}"
  --set-string grafanaDashboard.kind="${GRAFANA_DASHBOARD_KIND}"

  --set scaleOutDiscovery.switches.enabled="${SCALE_OUT_SWITCHES_ENABLED}"
  --set scaleOutDiscovery.leafGroups.enabled="${SCALE_OUT_LEAF_GROUPS_ENABLED}"

  --set scaleOutDiscovery.switches.mtls.enabled="${SWITCH_MTLS_ENABLED}"
  --set scaleOutDiscovery.switches.mtls.autoGenerate="${SWITCH_MTLS_AUTO_GENERATE}"
  --set-string scaleOutDiscovery.switches.mtls.controllerSecretName="${SWITCH_MTLS_CONTROLLER_SECRET_NAME}"
  --set-string scaleOutDiscovery.switches.mtls.switchAgentSecretName="${SWITCH_MTLS_AGENT_SECRET_NAME}"

  --set-string scaleOutDiscovery.switches.agent.image.registry="${SWITCH_AGENT_REGISTRY}"
  --set-string scaleOutDiscovery.switches.agent.image.repository="${SWITCH_AGENT_REPOSITORY}"
  --set-string scaleOutDiscovery.switches.agent.image.tag="${SWITCH_AGENT_TAG}"
)

helm "${HELM_ARGS[@]}"
