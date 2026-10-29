#!/usr/bin/env bash

set -euo pipefail

TOPOGRAPH_RELEASE="${TOPOGRAPH_RELEASE:-topograph}"
TOPOGRAPH_NAMESPACE="${TOPOGRAPH_NAMESPACE:-topograph-system}"

usage() {
  cat <<'EOF'
Usage:
  e2e/topology/script/uninstall-nvidia-topograph.sh

Uninstalls the NVIDIA Topograph release installed by
install-nvidia-topograph.sh and removes the kubelet-less model Node objects.

Optional environment variables:
  TOPOGRAPH_RELEASE    Helm release name. Default: topograph
  TOPOGRAPH_NAMESPACE  Target namespace. Default: topograph-system
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

helm uninstall "${TOPOGRAPH_RELEASE}" --namespace "${TOPOGRAPH_NAMESPACE}" --ignore-not-found --wait
kubectl delete namespace "${TOPOGRAPH_NAMESPACE}" --ignore-not-found --wait=false
kubectl delete node -l e2e.unifabric.io/topograph-model=true --ignore-not-found
