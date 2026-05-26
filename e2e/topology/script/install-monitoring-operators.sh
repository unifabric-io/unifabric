#!/usr/bin/env bash

set -euo pipefail

GRAFANA_OPERATOR_VERSION="${GRAFANA_OPERATOR_VERSION:-5.22.2}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOPOLOGY_DIR="${TOPOLOGY_DIR:-$(cd "${SCRIPT_DIR}/.." && pwd)}"
SIMULATION_NAME="${SIMULATION_NAME:-unifable-e2e-topology}"
TARGET_NODE="${BOOTSTRAP_NODE:-node-gpu-1}"

usage() {
  cat <<'EOF'
Usage:
  e2e/topology/script/install-monitoring-operators.sh

Supported environment variables:
  GRAFANA_OPERATOR_VERSION   Grafana Operator version. Default: 5.22.2
  TOPOLOGY_DIR               Topology directory containing topology.json. Default: e2e/topology
  SIMULATION_NAME            Simulation name. Default: unifable-e2e-topology
  BOOTSTRAP_NODE             Target node fallback
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

yaml_quote() {
  local value="${1//\'/\'\'}"
  printf "'%s'" "${value}"
}

print_command() {
  printf '%q' "$1"
  shift
  printf ' %q' "$@"
  printf '\n'
}

require_command kubectl
require_command helm
require_command nvair

require_non_empty GRAFANA_OPERATOR_VERSION

echo "Installing monitoring operators into namespace: monitoring"

echo "Creating namespace: monitoring"
kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -

echo "Installing Prometheus Operator via kube-prometheus-stack release: prometheus"

HELM_REPO_ADD_ARGS=(repo add prometheus-community https://prometheus-community.github.io/helm-charts)
print_command helm "${HELM_REPO_ADD_ARGS[@]}"
helm "${HELM_REPO_ADD_ARGS[@]}" >/dev/null

HELM_REPO_UPDATE_ARGS=(repo update prometheus-community)
print_command helm "${HELM_REPO_UPDATE_ARGS[@]}"
helm "${HELM_REPO_UPDATE_ARGS[@]}" >/dev/null

PROMETHEUS_HELM_ARGS=(
  upgrade --install prometheus prometheus-community/kube-prometheus-stack
  --namespace monitoring
  --create-namespace
  --set grafana.enabled=false
  --set prometheus.service.type=NodePort
  --set prometheus.service.nodePort=30090
  --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false
  --set prometheus.prometheusSpec.podMonitorSelectorNilUsesHelmValues=false
  --debug
)
print_command helm "${PROMETHEUS_HELM_ARGS[@]}"
helm "${PROMETHEUS_HELM_ARGS[@]}"

echo "Installing Grafana Operator ${GRAFANA_OPERATOR_VERSION} release: grafana-operator"

kubectl apply --server-side --force-conflicts \
  -f "https://github.com/grafana/grafana-operator/releases/download/v${GRAFANA_OPERATOR_VERSION}/crds.yaml"

GRAFANA_OPERATOR_HELM_ARGS=(
  upgrade --install grafana-operator
  oci://ghcr.io/grafana/helm-charts/grafana-operator
  --namespace monitoring
  --create-namespace
  --version "${GRAFANA_OPERATOR_VERSION}"
)
print_command helm "${GRAFANA_OPERATOR_HELM_ARGS[@]}"
helm "${GRAFANA_OPERATOR_HELM_ARGS[@]}"

echo "Creating Grafana instance unifabric-grafana and Prometheus datasource"

kubectl apply -n monitoring -f - <<EOF
apiVersion: grafana.integreatly.org/v1beta1
kind: Grafana
metadata:
  name: unifabric-grafana
  labels:
    dashboards: grafana
spec:
  config:
    log:
      mode: "console"
    auth:
      disable_login_form: "false"
    security:
      admin_user: 'admin'
      admin_password: 'admin'
---
apiVersion: grafana.integreatly.org/v1beta1
kind: GrafanaDatasource
metadata:
  name: prometheus
spec:
  instanceSelector:
    matchLabels:
      dashboards: grafana
  datasource:
    name: Prometheus
    type: prometheus
    access: proxy
    url: 'http://prometheus-kube-prometheus-prometheus.monitoring.svc:9090'
    isDefault: true
    jsonData:
      timeInterval: 15s
EOF

echo "Creating Grafana NodePort service unifabric-grafana-nodeport on port 30300"

kubectl apply -n monitoring -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: unifabric-grafana-nodeport
  labels:
    app.kubernetes.io/name: grafana
    app.kubernetes.io/instance: unifabric-grafana
spec:
  type: NodePort
  selector:
    app: unifabric-grafana
  ports:
    - name: http
      port: 3000
      targetPort: grafana-http
      protocol: TCP
      nodePort: 30300
EOF

echo "Waiting for Prometheus and Grafana Operator rollouts"

kubectl rollout status -n monitoring deployment/prometheus-kube-prometheus-operator --timeout=180s
kubectl rollout status -n monitoring deployment/grafana-operator --timeout=180s

echo "GrafanaDashboard resources with spec.instanceSelector={} will be imported by Grafana instances watched by the operator."
echo "Install Unifabric chart with: --set grafanaDashboard.kind=GrafanaDashboard"

echo "Exposing Prometheus and Grafana UIs through NVIDIA Air forwards"
nvair add forward prometheus -s "${SIMULATION_NAME}" --target-node "${TARGET_NODE}" --target-port 30090
nvair add forward grafana -s "${SIMULATION_NAME}" --target-node "${TARGET_NODE}" --target-port 30300
nvair get forwards -s "${SIMULATION_NAME}"
