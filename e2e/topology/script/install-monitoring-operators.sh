#!/usr/bin/env bash

set -euo pipefail

# Supported custom variables:
#   GRAFANA_OPERATOR_VERSION
#   NVAIR_BIN
#   TOPOLOGY_DIR
#   SIMULATION_NAME
#   BOOTSTRAP_NODE
#
# Kubeconfig behavior:
#   This script does not define KUBECONFIG_PATH.
#   kubectl and helm will use the native KUBECONFIG environment variable.
#   If KUBECONFIG is unset, they will use the default kubeconfig, usually ~/.kube/config.

GRAFANA_OPERATOR_VERSION="${GRAFANA_OPERATOR_VERSION:-5.22.2}"
NVAIR_BIN="${NVAIR_BIN:-nvair}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOPOLOGY_DIR="${TOPOLOGY_DIR:-$(cd "${SCRIPT_DIR}/.." && pwd)}"
SIMULATION_NAME="${SIMULATION_NAME:-}"
TARGET_NODE="${BOOTSTRAP_NODE:-node-gpu-1}"

# Fixed values. Extract them later only when needed.
NAMESPACE="monitoring"
PROMETHEUS_RELEASE="prometheus"
GRAFANA_OPERATOR_RELEASE="grafana-operator"
GRAFANA_NAME="unifabric-grafana"
GRAFANA_ADMIN_USER="admin"
GRAFANA_ADMIN_PASSWORD="admin"
PROMETHEUS_DATASOURCE_NAME="prometheus"
PROMETHEUS_NODE_PORT="30090"
GRAFANA_NODE_PORT="30300"

usage() {
  cat <<'EOF'
Usage:
  e2e/topology/script/install-monitoring-operators.sh [--simulation <simulation>] [--target-node <node>]

Supported environment variables:
  GRAFANA_OPERATOR_VERSION   Grafana Operator version. Default: 5.22.2
  NVAIR_BIN                  Path to nvair binary. Default: nvair
  TOPOLOGY_DIR               Topology directory containing topology.json. Default: e2e/topology
  SIMULATION_NAME            Simulation name fallback
  BOOTSTRAP_NODE             Target node fallback

Kubeconfig:
  This script does not accept KUBECONFIG_PATH.
  Use the standard KUBECONFIG environment variable if needed.

Examples:
  e2e/topology/script/install-monitoring-operators.sh --simulation unifable-e2e-topology

  GRAFANA_OPERATOR_VERSION=5.22.2 e2e/topology/script/install-monitoring-operators.sh

  KUBECONFIG=/path/to/kubeconfig \
  GRAFANA_OPERATOR_VERSION=5.22.2 \
  e2e/topology/script/install-monitoring-operators.sh
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
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

resolve_simulation_name() {
  if [[ -n "${SIMULATION_NAME}" ]]; then
    printf '%s\n' "${SIMULATION_NAME}"
    return
  fi

  if [[ ! -f "${TOPOLOGY_DIR}/topology.json" ]]; then
    echo "topology.json not found in ${TOPOLOGY_DIR}" >&2
    exit 2
  fi

  local resolved
  resolved="$(awk -F'"' '/"title"[[:space:]]*:[[:space:]]*"/ { print $4; exit }' "${TOPOLOGY_DIR}/topology.json")"
  if [[ -z "${resolved}" ]]; then
    echo "failed to parse simulation title from ${TOPOLOGY_DIR}/topology.json" >&2
    exit 2
  fi

  printf '%s\n' "${resolved}"
}

while (($# > 0)); do
  case "$1" in
    -s|--simulation)
      if (($# < 2)) || [[ -z "${2:-}" ]]; then
        echo "Missing value for $1" >&2
        usage >&2
        exit 2
      fi
      SIMULATION_NAME="${2}"
      shift 2
      ;;
    -n|--target-node)
      if (($# < 2)) || [[ -z "${2:-}" ]]; then
        echo "Missing value for $1" >&2
        usage >&2
        exit 2
      fi
      TARGET_NODE="${2}"
      shift 2
      ;;
    *)
      echo "Unknown argument: ${1}" >&2
      usage >&2
      exit 2
      ;;
  esac
done

require_command kubectl
require_command helm
require_command "${NVAIR_BIN}"

require_non_empty GRAFANA_OPERATOR_VERSION
SIMULATION_NAME="$(resolve_simulation_name)"

echo "Installing monitoring operators into namespace: ${NAMESPACE}"

if [[ -n "${KUBECONFIG:-}" ]]; then
  echo "Kubeconfig: ${KUBECONFIG}"
else
  echo "Kubeconfig: kubectl/helm default"
fi

echo "Prometheus release: ${PROMETHEUS_RELEASE}"
echo "Grafana Operator release: ${GRAFANA_OPERATOR_RELEASE}"
echo "Grafana Operator version: ${GRAFANA_OPERATOR_VERSION}"
echo "Grafana instance: ${GRAFANA_NAME}"
echo "Prometheus NodePort: ${PROMETHEUS_NODE_PORT}"
echo "Grafana NodePort: ${GRAFANA_NODE_PORT}"
echo "Simulation: ${SIMULATION_NAME}"
echo "Monitoring UI forward target node: ${TARGET_NODE}"

echo "Creating namespace: ${NAMESPACE}"
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

echo "Installing Prometheus Operator via kube-prometheus-stack release: ${PROMETHEUS_RELEASE}"
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null
helm repo update prometheus-community >/dev/null

helm upgrade --install "${PROMETHEUS_RELEASE}" prometheus-community/kube-prometheus-stack \
  --namespace "${NAMESPACE}" \
  --create-namespace \
  --set grafana.enabled=false \
  --set prometheus.service.type=NodePort \
  --set prometheus.service.nodePort="${PROMETHEUS_NODE_PORT}" \
  --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false \
  --set prometheus.prometheusSpec.podMonitorSelectorNilUsesHelmValues=false \
  --debug

echo "Installing Grafana Operator ${GRAFANA_OPERATOR_VERSION} release: ${GRAFANA_OPERATOR_RELEASE}"

kubectl apply --server-side --force-conflicts \
  -f "https://github.com/grafana/grafana-operator/releases/download/v${GRAFANA_OPERATOR_VERSION}/crds.yaml"

helm upgrade --install "${GRAFANA_OPERATOR_RELEASE}" \
  oci://ghcr.io/grafana/helm-charts/grafana-operator \
  --namespace "${NAMESPACE}" \
  --create-namespace \
  --version "${GRAFANA_OPERATOR_VERSION}"

PROMETHEUS_URL="http://${PROMETHEUS_RELEASE}-kube-prometheus-prometheus.${NAMESPACE}.svc:9090"

echo "Creating Grafana instance ${GRAFANA_NAME} and Prometheus datasource"

kubectl apply -n "${NAMESPACE}" -f - <<EOF
apiVersion: grafana.integreatly.org/v1beta1
kind: Grafana
metadata:
  name: ${GRAFANA_NAME}
  labels:
    dashboards: grafana
spec:
  config:
    log:
      mode: "console"
    auth:
      disable_login_form: "false"
    security:
      admin_user: $(yaml_quote "${GRAFANA_ADMIN_USER}")
      admin_password: $(yaml_quote "${GRAFANA_ADMIN_PASSWORD}")
---
apiVersion: grafana.integreatly.org/v1beta1
kind: GrafanaDatasource
metadata:
  name: ${PROMETHEUS_DATASOURCE_NAME}
spec:
  instanceSelector:
    matchLabels:
      dashboards: grafana
  datasource:
    name: Prometheus
    type: prometheus
    access: proxy
    url: $(yaml_quote "${PROMETHEUS_URL}")
    isDefault: true
    jsonData:
      timeInterval: 15s
EOF

echo "Creating Grafana NodePort service ${GRAFANA_NAME}-nodeport on port ${GRAFANA_NODE_PORT}"

kubectl apply -n "${NAMESPACE}" -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: ${GRAFANA_NAME}-nodeport
  labels:
    app.kubernetes.io/name: grafana
    app.kubernetes.io/instance: ${GRAFANA_NAME}
spec:
  type: NodePort
  selector:
    app: ${GRAFANA_NAME}
  ports:
    - name: http
      port: 3000
      targetPort: grafana-http
      protocol: TCP
      nodePort: ${GRAFANA_NODE_PORT}
EOF

echo "Waiting for Prometheus and Grafana Operator rollouts"

kubectl rollout status -n "${NAMESPACE}" "deployment/${PROMETHEUS_RELEASE}-kube-prometheus-operator" --timeout=180s
kubectl rollout status -n "${NAMESPACE}" "deployment/${GRAFANA_OPERATOR_RELEASE}" --timeout=180s

echo "GrafanaDashboard resources with spec.instanceSelector={} will be imported by Grafana instances watched by the operator."
echo "Install Unifabric chart with: --set grafanaDashboard.kind=GrafanaDashboard"

echo "Exposing Prometheus and Grafana UIs through NVIDIA Air forwards"
"${NVAIR_BIN}" add forward prometheus -s "${SIMULATION_NAME}" --target-node "${TARGET_NODE}" --target-port "${PROMETHEUS_NODE_PORT}"
"${NVAIR_BIN}" add forward grafana -s "${SIMULATION_NAME}" --target-node "${TARGET_NODE}" --target-port "${GRAFANA_NODE_PORT}"
"${NVAIR_BIN}" get forwards -s "${SIMULATION_NAME}"

NODE_IP="$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}' 2>/dev/null || true)"

if [[ -n "${NODE_IP}" ]]; then
  echo "Prometheus UI: http://${NODE_IP}:${PROMETHEUS_NODE_PORT}"
  echo "Grafana UI: http://${NODE_IP}:${GRAFANA_NODE_PORT}"
else
  echo "Prometheus UI NodePort: ${PROMETHEUS_NODE_PORT}"
  echo "Grafana UI NodePort: ${GRAFANA_NODE_PORT}"
fi
