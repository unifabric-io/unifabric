#!/usr/bin/env bash

set -euo pipefail

SFLOW_MOCK_IMAGE="${SFLOW_MOCK_IMAGE:-}"
NAMESPACE="${NAMESPACE:-default}"
JOB_NAME="${JOB_NAME:-sflow-mock-sender}"
SFLOW_TARGET="${SFLOW_TARGET:-unifabric-sflow.unifabric-system.svc.cluster.local:6343}"
MOCK_AGENT_IP="${MOCK_AGENT_IP:-10.0.0.11}"
MOCK_COUNT="${MOCK_COUNT:-120}"
MOCK_INTERVAL="${MOCK_INTERVAL:-500ms}"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-300s}"

usage() {
  cat <<'EOF'
Usage:
  e2e/topology/script/run-sflow-mock.sh

Reads the macvlan (net1) IPs of the pd-prefill/pd-decoder test pods deployed
by deploy-pd-macvlan-test.sh, then runs a Kubernetes Job that reuses the
unifabric-sflow image in "mock" mode to send crafted sFlow v5 datagrams to
the unifabric-sflow collector service. The sampled packet headers carry the
two pod IPs, so Elasticsearch documents must resolve to those pods.

Required environment variables:
  SFLOW_MOCK_IMAGE  unifabric-sflow image to run in mock mode.

Optional environment variables:
  NAMESPACE         Namespace of the test pods and Job. Default: default
  JOB_NAME          Job name. Default: sflow-mock-sender
  SFLOW_TARGET      Collector UDP address.
                    Default: unifabric-sflow.unifabric-system.svc.cluster.local:6343
  MOCK_AGENT_IP     sFlow agent (switch) IP in datagrams. Default: 10.0.0.11
  MOCK_COUNT        Number of datagrams to send. Default: 120
  MOCK_INTERVAL     Interval between datagrams. Default: 500ms
  WAIT_TIMEOUT      Job completion timeout. Default: 300s
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

log() {
  echo "[sflow-mock] $*"
}

pod_net1_ip() {
  local role="$1"
  local pod

  pod="$(kubectl -n "${NAMESPACE}" get pods \
    -l "app.kubernetes.io/name=pd-macvlan-test,app.kubernetes.io/component=${role}" \
    -o jsonpath='{.items[0].metadata.name}')"
  kubectl -n "${NAMESPACE}" exec "${pod}" -- \
    sh -c "ip -4 -o addr show dev net1 | awk '{print \$4}' | cut -d/ -f1"
}

require_command kubectl

if [[ -z "${SFLOW_MOCK_IMAGE}" ]]; then
  echo "SFLOW_MOCK_IMAGE must not be empty." >&2
  usage >&2
  exit 2
fi

prefill_ip="$(pod_net1_ip prefill)"
decoder_ip="$(pod_net1_ip decoder)"

if [[ -z "${prefill_ip}" || -z "${decoder_ip}" ]]; then
  echo "Failed to resolve macvlan net1 IPs: prefill='${prefill_ip}' decoder='${decoder_ip}'" >&2
  exit 1
fi

log "prefill net1 IP: ${prefill_ip}"
log "decoder net1 IP: ${decoder_ip}"
log "target collector: ${SFLOW_TARGET}"

kubectl -n "${NAMESPACE}" delete job "${JOB_NAME}" --ignore-not-found

kubectl -n "${NAMESPACE}" apply -f - <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: ${JOB_NAME}
  labels:
    app.kubernetes.io/name: sflow-mock-sender
spec:
  backoffLimit: 1
  template:
    metadata:
      labels:
        app.kubernetes.io/name: sflow-mock-sender
    spec:
      restartPolicy: Never
      tolerations:
        - operator: Exists
      containers:
        - name: mock-sender
          image: ${SFLOW_MOCK_IMAGE}
          imagePullPolicy: IfNotPresent
          command:
            - /usr/bin/unifabric/sflow
            - mock
            - -target=${SFLOW_TARGET}
            - -agent=${MOCK_AGENT_IP}
            - -pods=${prefill_ip},${decoder_ip}
            - -count=${MOCK_COUNT}
            - -interval=${MOCK_INTERVAL}
          resources:
            requests:
              cpu: 10m
              memory: 16Mi
            limits:
              cpu: 100m
              memory: 64Mi
EOF

log "waiting for Job ${JOB_NAME} to complete (timeout ${WAIT_TIMEOUT})"
kubectl -n "${NAMESPACE}" wait --for=condition=complete "job/${JOB_NAME}" --timeout="${WAIT_TIMEOUT}"

kubectl -n "${NAMESPACE}" logs "job/${JOB_NAME}" --tail=20

log "Done."
