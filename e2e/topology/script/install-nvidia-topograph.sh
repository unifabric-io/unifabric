#!/usr/bin/env bash

set -euo pipefail

TOPOGRAPH_RELEASE="${TOPOGRAPH_RELEASE:-topograph}"
TOPOGRAPH_NAMESPACE="${TOPOGRAPH_NAMESPACE:-topograph-system}"
TOPOGRAPH_CHART_VERSION="${TOPOGRAPH_CHART_VERSION:-1.0.0}"
TOPOGRAPH_HELM_REPO="${TOPOGRAPH_HELM_REPO:-https://helm.ngc.nvidia.com/nvidia/topograph}"
# Chart 1.0.0 defaults to a ghcr.io/dsx-ai-factory image path that does not
# exist; pin the published NVIDIA image explicitly.
TOPOGRAPH_IMAGE_REPOSITORY="${TOPOGRAPH_IMAGE_REPOSITORY:-ghcr.io/nvidia/topograph}"
TOPOGRAPH_IMAGE_TAG="${TOPOGRAPH_IMAGE_TAG:-v${TOPOGRAPH_CHART_VERSION}}"

HELM_TIMEOUT="10m"

usage() {
  cat <<'EOF'
Usage:
  e2e/topology/script/install-nvidia-topograph.sh

Installs the upstream NVIDIA Topograph chart with its "test" provider and the
k8s engine so it writes real topology labels under the Unifabric label
contract. The engine and the node-observer trigger are scoped to nodes labeled
e2e.unifabric.io/topograph-model=true, so Topograph never touches the real
cluster nodes.

Optional environment variables:
  TOPOGRAPH_RELEASE        Helm release name. Default: topograph
  TOPOGRAPH_NAMESPACE      Target namespace. Default: topograph-system
  TOPOGRAPH_CHART_VERSION  Chart version. Default: 1.0.0
  TOPOGRAPH_HELM_REPO      Helm repository URL.
  TOPOGRAPH_IMAGE_REPOSITORY  Image repository. Default: ghcr.io/nvidia/topograph
  TOPOGRAPH_IMAGE_TAG      Image tag. Default: v<chart version>

Kubeconfig:
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

for command in helm kubectl; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    echo "Missing required command: ${command}" >&2
    exit 1
  fi
done

VALUES_FILE="$(mktemp)"
trap 'rm -f "${VALUES_FILE}"' EXIT

cat > "${VALUES_FILE}" <<'EOF'
provider:
  name: test
  params:
    # Embedded model: 8 hosts (1101..1402) under sw11..sw14 -> sw21/sw22 ->
    # sw3, with accelerator domains nvl1..nvl4.
    modelFileName: medium.yaml

engine:
  name: k8s
  params:
    # Restrict listing and labeling to the kubelet-less model nodes.
    nodeSelector:
      e2e.unifabric.io/topograph-model: "true"
    acceleratorLabel: scale-up.unifabric.io/tier-1
    fabricLabels:
      - scale-out.unifabric.io/tier-1
      - scale-out.unifabric.io/tier-2
      - scale-out.unifabric.io/tier-3

nodeObserver:
  topograph:
    trigger:
      nodeSelector:
        e2e.unifabric.io/topograph-model: "true"

# The test provider computes the topology from the embedded model, so no
# per-node broker is needed and the broker-readiness gate must stay off.
nodeDataBroker:
  enabled: false
EOF

helm repo add nvidia-topograph "${TOPOGRAPH_HELM_REPO}" --force-update
helm repo update nvidia-topograph

HELM_ARGS=(
  upgrade --install "${TOPOGRAPH_RELEASE}" nvidia-topograph/topograph
  --version "${TOPOGRAPH_CHART_VERSION}"
  --namespace "${TOPOGRAPH_NAMESPACE}"
  --create-namespace
  --values "${VALUES_FILE}"
  --set-string image.repository="${TOPOGRAPH_IMAGE_REPOSITORY}"
  --set-string image.tag="${TOPOGRAPH_IMAGE_TAG}"
  --wait
  --timeout "${HELM_TIMEOUT}"
)

printf '%q' helm
printf ' %q' "${HELM_ARGS[@]}"
printf '\n'
helm "${HELM_ARGS[@]}"
