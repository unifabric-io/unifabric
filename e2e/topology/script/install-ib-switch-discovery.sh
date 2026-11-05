#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOPOLOGY_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd "${TOPOLOGY_DIR}/../.." && pwd)"

UNIFABRIC_NAMESPACE="${UNIFABRIC_NAMESPACE:-unifabric-system}"
TOPOGRAPH_NAMESPACE="${TOPOGRAPH_NAMESPACE:-topograph-system}"
TOPOGRAPH_CHART_VERSION="${TOPOGRAPH_CHART_VERSION:-1.1.0}"
TOPOGRAPH_HELM_REPO="${TOPOGRAPH_HELM_REPO:-https://helm.ngc.nvidia.com/nvidia/topograph}"
TOPOGRAPH_IMAGE_REPOSITORY="${TOPOGRAPH_IMAGE_REPOSITORY:-nvcr.io/nvidia/topograph/topograph}"
LOCAL_IMAGE_TAG="${LOCAL_IMAGE_TAG:-local}"
SWITCH_TTL="${SWITCH_TTL:-2m}"

usage() {
  cat <<'EOF'
Usage:
  e2e/topology/script/install-ib-switch-discovery.sh

Local (kind) setup for the ib-switch-discovery scenario. Replays
e2e/topology/ib-sim/small.ibnetdiscover through both the unifabric agent
(ibnetdiscoverConfigMap file mode) and Topograph's infiniband-sim provider,
then installs both charts with scaleUp.mode=nv-gpu-operator (the Node
manifest carries nvidia.com/gpu.clique labels in place of the GPU Operator). No InfiniBand hardware is needed.

Expects unifabric-controller and unifabric-agent images tagged
$LOCAL_IMAGE_TAG (default: local) to be loaded into the cluster already.

Optional environment variables:
  LOCAL_IMAGE_TAG          Controller/agent image tag. Default: local
  SWITCH_TTL               ibDiscovery.switchTTL. Default: 2m
  TOPOGRAPH_CHART_VERSION  Default: 1.1.0
  TOPOGRAPH_IMAGE_REPOSITORY  Default: nvcr.io/nvidia/topograph/topograph
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

for command in helm kubectl; do
  command -v "${command}" >/dev/null 2>&1 || { echo "Missing required command: ${command}" >&2; exit 1; }
done

FIXTURE="${TOPOLOGY_DIR}/ib-sim/small.ibnetdiscover"

for ns in "${UNIFABRIC_NAMESPACE}" "${TOPOGRAPH_NAMESPACE}"; do
  kubectl create namespace "${ns}" --dry-run=client -o yaml | kubectl apply -f -
done

# The agent reads key ibnetdiscover.out; Topograph reads the mounted file name.
kubectl -n "${UNIFABRIC_NAMESPACE}" create configmap ib-sim \
  --from-file=ibnetdiscover.out="${FIXTURE}" --dry-run=client -o yaml | kubectl apply -f -
kubectl -n "${TOPOGRAPH_NAMESPACE}" create configmap ib-sim \
  --from-file=small.ibnetdiscover="${FIXTURE}" --dry-run=client -o yaml | kubectl apply -f -

TOPOGRAPH_VALUES="$(mktemp)"
trap 'rm -f "${TOPOGRAPH_VALUES}"' EXIT
cat > "${TOPOGRAPH_VALUES}" <<'EOF'
provider:
  name: infiniband-sim
  params:
    ibnetdiscoverFileName: /etc/ib-sim/small.ibnetdiscover
engine:
  name: k8s
  params:
    nodeSelector:
      e2e.unifabric.io/topograph-model: "true"
    fabricLabels:
      - scale-out.unifabric.io/tier-1
      - scale-out.unifabric.io/tier-2
      - scale-out.unifabric.io/tier-3
volumes:
  - name: ib-sim
    configMap:
      name: ib-sim
volumeMounts:
  - name: ib-sim
    mountPath: /etc/ib-sim
    readOnly: true
nodeDataBroker:
  enabled: false
nodeObserver:
  topograph:
    trigger:
      nodeSelector:
        e2e.unifabric.io/topograph-model: "true"
EOF

helm repo add nvidia-topograph "${TOPOGRAPH_HELM_REPO}" --force-update
helm repo update nvidia-topograph
helm upgrade --install topograph nvidia-topograph/topograph \
  --version "${TOPOGRAPH_CHART_VERSION}" \
  --namespace "${TOPOGRAPH_NAMESPACE}" \
  --values "${TOPOGRAPH_VALUES}" \
  --set-string image.repository="${TOPOGRAPH_IMAGE_REPOSITORY}" \
  --wait --timeout 10m

helm upgrade --install unifabric "${REPO_ROOT}/chart" \
  --namespace "${UNIFABRIC_NAMESPACE}" \
  --set-string topoDiscovery.scaleUp.mode=nv-gpu-operator \
  --set-string topoDiscovery.scaleOut.mode=unifabric-ib-with-topograph \
  --set topologyAPI.enabled=false \
  --set grafanaDashboard.enabled=false \
  --set grafanaInstance.enabled=false \
  --set serviceMonitor.enabled=false \
  --set-string controller.image.tag="${LOCAL_IMAGE_TAG}" \
  --set-string agent.image.tag="${LOCAL_IMAGE_TAG}" \
  --set-string agent.ibDiscovery.ibnetdiscoverConfigMap.name=ib-sim \
  --set-string agent.config.ibDiscovery.interval=10s \
  --set-string agent.config.ibDiscovery.switchTTL="${SWITCH_TTL}" \
  --wait --timeout 10m
