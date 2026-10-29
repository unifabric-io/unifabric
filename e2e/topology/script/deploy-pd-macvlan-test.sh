#!/usr/bin/env bash

set -euo pipefail

NAMESPACE="${NAMESPACE:-default}"
PREFILL_NAME="${PREFILL_NAME:-pd-prefill}"
DECODER_NAME="${DECODER_NAME:-pd-decoder}"
PREFILL_NODE="${PREFILL_NODE:-node-gpu-1}"
DECODER_NODE="${DECODER_NODE:-node-gpu-2}"
MULTUS_NETWORK="${MULTUS_NETWORK:-spiderpool/macvlan-eth1}"
TEST_IMAGE="${TEST_IMAGE:-nicolaka/netshoot:latest}"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-180s}"

usage() {
  cat <<'EOF'
Usage:
  e2e/topology/script/deploy-pd-macvlan-test.sh

Deploys two test Deployments to mimic prefill/decoder separation:
  - pd-prefill on node-gpu-1
  - pd-decoder on node-gpu-2

Both Pods attach to the Spiderpool macvlan network through Multus.

Optional environment variables:
  NAMESPACE       Target namespace. Default: default
  PREFILL_NAME    Prefill Deployment name. Default: pd-prefill
  DECODER_NAME    Decoder Deployment name. Default: pd-decoder
  PREFILL_NODE    Prefill target node. Default: node-gpu-1
  DECODER_NODE    Decoder target node. Default: node-gpu-2
  MULTUS_NETWORK  Multus network annotation value. Default: spiderpool/macvlan-eth1
  TEST_IMAGE      Test container image. Default: nicolaka/netshoot:latest
  WAIT_TIMEOUT    Rollout wait timeout. Default: 180s
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

log() {
  echo "[pd-macvlan-test] $*"
}

yaml_quote() {
  local value="${1//\'/\'\'}"
  printf "'%s'" "${value}"
}

apply_deployment() {
  local name="$1"
  local role="$2"
  local node="$3"

  kubectl -n "${NAMESPACE}" apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${name}
  labels:
    app.kubernetes.io/name: pd-macvlan-test
    app.kubernetes.io/component: ${role}
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: pd-macvlan-test
      app.kubernetes.io/component: ${role}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: pd-macvlan-test
        app.kubernetes.io/component: ${role}
      annotations:
        k8s.v1.cni.cncf.io/networks: $(yaml_quote "${MULTUS_NETWORK}")
    spec:
      nodeSelector:
        kubernetes.io/hostname: ${node}
      tolerations:
        - operator: Exists
      containers:
        - name: ${role}
          image: ${TEST_IMAGE}
          imagePullPolicy: IfNotPresent
          command:
            - sh
            - -c
            - |
              echo "${role} started on \$(hostname)"
              while true; do
                date
                ip -4 addr show dev net1 || true
                sleep 3600
              done
          resources:
            requests:
              cpu: 10m
              memory: 16Mi
            limits:
              cpu: 100m
              memory: 64Mi
EOF
}

pod_for_component() {
  local role="$1"

  kubectl -n "${NAMESPACE}" get pods \
    -l "app.kubernetes.io/name=pd-macvlan-test,app.kubernetes.io/component=${role}" \
    -o jsonpath='{.items[0].metadata.name}'
}

print_pod_network() {
  local role="$1"
  local pod
  local node
  local status

  pod="$(pod_for_component "${role}")"
  node="$(kubectl -n "${NAMESPACE}" get pod "${pod}" -o jsonpath='{.spec.nodeName}')"
  status="$(kubectl -n "${NAMESPACE}" get pod "${pod}" -o jsonpath='{.metadata.annotations.k8s\.v1\.cni\.cncf\.io/network-status}')"

  echo "== ${role}: ${pod} on ${node} =="
  printf '%s\n' "${status}"
  kubectl -n "${NAMESPACE}" exec "${pod}" -- ip -4 addr show dev net1
}

require_command kubectl

require_non_empty NAMESPACE
require_non_empty PREFILL_NAME
require_non_empty DECODER_NAME
require_non_empty PREFILL_NODE
require_non_empty DECODER_NODE
require_non_empty MULTUS_NETWORK
require_non_empty TEST_IMAGE
require_non_empty WAIT_TIMEOUT

log "KUBECONFIG=${KUBECONFIG:-<default>}"
log "namespace=${NAMESPACE}, network=${MULTUS_NETWORK}"
log "prefill=${PREFILL_NAME} on ${PREFILL_NODE}"
log "decoder=${DECODER_NAME} on ${DECODER_NODE}"

kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

apply_deployment "${PREFILL_NAME}" prefill "${PREFILL_NODE}"
apply_deployment "${DECODER_NAME}" decoder "${DECODER_NODE}"

kubectl -n "${NAMESPACE}" rollout status "deployment/${PREFILL_NAME}" --timeout="${WAIT_TIMEOUT}"
kubectl -n "${NAMESPACE}" rollout status "deployment/${DECODER_NAME}" --timeout="${WAIT_TIMEOUT}"

kubectl -n "${NAMESPACE}" get pods \
  -l app.kubernetes.io/name=pd-macvlan-test \
  -o wide

print_pod_network prefill
print_pod_network decoder

log "Done."
