#!/usr/bin/env bash

set -euo pipefail

RELEASE_NAME="${RELEASE_NAME:-unifabric}"
RELEASE_NAMESPACE="${RELEASE_NAMESPACE:-unifabric-system}"

if helm status "${RELEASE_NAME}" --namespace "${RELEASE_NAMESPACE}" >/dev/null 2>&1; then
  helm uninstall "${RELEASE_NAME}" --namespace "${RELEASE_NAMESPACE}" --wait --timeout 10m
fi

# Helm intentionally retains CRDs. The Controller is now absent, so a Topology
# already carrying the reset finalizer cannot finish deletion by itself.
# Remove finalizers from disposable E2E Topology instances, delete all custom
# resources, and delete the CRDs only after their instances are gone.
kubectl delete fabricnodes.unifabric.io --all --ignore-not-found
kubectl delete switches.unifabric.io --all --ignore-not-found

while IFS= read -r topology; do
  if [[ -n "${topology}" ]]; then
    echo "Removing finalizers from ${topology} for E2E cleanup."
    kubectl patch "${topology}" --type=merge -p '{"metadata":{"finalizers":[]}}'
  fi
done < <(kubectl get topologies.unifabric.io -o name 2>/dev/null || true)

kubectl delete topologies.unifabric.io --all --ignore-not-found --wait=false

label_keys=(
  scale-out.unifabric.io/tier-1
  scale-out.unifabric.io/tier-2
  scale-out.unifabric.io/tier-3
  scale-out.unifabric.io/tier-4
  scale-up.unifabric.io/tier-1
  scale-up.unifabric.io/tier-2
  scale-up.unifabric.io/tier-3
  scale-up.unifabric.io/tier-4
  storage.unifabric.io/tier-1
  storage.unifabric.io/tier-2
  storage.unifabric.io/tier-3
  storage.unifabric.io/tier-4
)

for key in "${label_keys[@]}"; do
  kubectl label nodes --all "${key}-" >/dev/null
done

resources_removed=false
for _ in $(seq 1 60); do
  remaining="$(
    {
      kubectl get fabricnodes.unifabric.io -o name 2>/dev/null || true
      kubectl get switches.unifabric.io -o name 2>/dev/null || true
      kubectl get topologies.unifabric.io -o name 2>/dev/null || true
    } | sed '/^[[:space:]]*$/d'
  )"
  if [[ -z "${remaining}" ]]; then
    resources_removed=true
    break
  fi
  sleep 2
done

if [[ "${resources_removed}" != "true" ]]; then
  echo "Timed out waiting for Unifabric custom resources to be removed." >&2
  exit 1
fi

kubectl delete customresourcedefinitions \
  fabricnodes.unifabric.io \
  switches.unifabric.io \
  topologies.unifabric.io \
  --ignore-not-found \
  --wait=true \
  --timeout=5m
