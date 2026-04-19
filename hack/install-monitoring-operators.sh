#!/usr/bin/env bash

set -euo pipefail

NAMESPACE="${NAMESPACE:-monitoring}"
PROMETHEUS_RELEASE="${PROMETHEUS_RELEASE:-prometheus}"
GRAFANA_OPERATOR_RELEASE="${GRAFANA_OPERATOR_RELEASE:-grafana-operator}"
GRAFANA_OPERATOR_VERSION="${GRAFANA_OPERATOR_VERSION:-5.22.2}"
GRAFANA_NAME="${GRAFANA_NAME:-unifabric-grafana}"
GRAFANA_ADMIN_USER="${GRAFANA_ADMIN_USER:-admin}"
GRAFANA_ADMIN_PASSWORD="${GRAFANA_ADMIN_PASSWORD:-admin}"
INSTALL_GRAFANA_INSTANCE="${INSTALL_GRAFANA_INSTANCE:-true}"
PROMETHEUS_DATASOURCE_NAME="${PROMETHEUS_DATASOURCE_NAME:-prometheus}"
PROMETHEUS_NODE_PORT="${PROMETHEUS_NODE_PORT:-30090}"
GRAFANA_NODE_PORT="${GRAFANA_NODE_PORT:-30300}"

usage() {
  cat <<'EOF'
Usage:
  install-monitoring-operators.sh [flags]

Flags:
  -n, --namespace <name>                    Namespace for monitoring components. Default: monitoring
      --prometheus-release <name>           Helm release name for kube-prometheus-stack. Default: prometheus
      --grafana-operator-release <name>     Helm release name for Grafana Operator. Default: grafana-operator
      --grafana-operator-version <version>  Grafana Operator chart/app version. Default: 5.22.2
      --grafana-name <name>                 Grafana CR name. Default: unifabric-grafana
      --grafana-admin-user <user>           Grafana admin user. Default: admin
      --grafana-admin-password <password>   Grafana admin password. Default: admin
      --prometheus-node-port <port>         NodePort for Prometheus UI. Default: 30090
      --grafana-node-port <port>            NodePort for Grafana UI. Default: 30300
      --skip-grafana-instance               Install operators only; do not create Grafana/GrafanaDatasource CRs.
  -h, --help                                Show this help.

Environment variables with the same uppercase names can also be used.

Notes:
  - Installs Prometheus Operator through prometheus-community/kube-prometheus-stack.
  - Installs Grafana Operator from oci://ghcr.io/grafana/helm-charts/grafana-operator.
  - Creates a Grafana instance and a Prometheus datasource.
  - Exposes Prometheus and Grafana UIs through NodePort Services.
  - Configures Prometheus to discover ServiceMonitor and PodMonitor resources
    resources without requiring Helm release labels.
EOF
}

while (( $# > 0 )); do
  case "${1}" in
    -n|--namespace)
      NAMESPACE="${2:-}"
      shift 2
      ;;
    --prometheus-release)
      PROMETHEUS_RELEASE="${2:-}"
      shift 2
      ;;
    --grafana-operator-release)
      GRAFANA_OPERATOR_RELEASE="${2:-}"
      shift 2
      ;;
    --grafana-operator-version)
      GRAFANA_OPERATOR_VERSION="${2:-}"
      shift 2
      ;;
    --grafana-name)
      GRAFANA_NAME="${2:-}"
      shift 2
      ;;
    --grafana-admin-user)
      GRAFANA_ADMIN_USER="${2:-}"
      shift 2
      ;;
    --grafana-admin-password)
      GRAFANA_ADMIN_PASSWORD="${2:-}"
      shift 2
      ;;
    --prometheus-node-port)
      PROMETHEUS_NODE_PORT="${2:-}"
      shift 2
      ;;
    --grafana-node-port)
      GRAFANA_NODE_PORT="${2:-}"
      shift 2
      ;;
    --skip-grafana-instance)
      INSTALL_GRAFANA_INSTANCE="false"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: ${1}" >&2
      usage >&2
      exit 2
      ;;
  esac
done

for value_name in NAMESPACE PROMETHEUS_RELEASE GRAFANA_OPERATOR_RELEASE GRAFANA_OPERATOR_VERSION GRAFANA_NAME; do
  if [[ -z "${!value_name}" ]]; then
    echo "${value_name} must not be empty." >&2
    exit 2
  fi
done

for value_name in PROMETHEUS_NODE_PORT GRAFANA_NODE_PORT; do
  if ! [[ "${!value_name}" =~ ^[0-9]+$ ]] || (( ${!value_name} < 30000 || ${!value_name} > 32767 )); then
    echo "${value_name} must be a valid Kubernetes NodePort in the range 30000-32767." >&2
    exit 2
  fi
done

require_command() {
  if ! command -v "${1}" >/dev/null 2>&1; then
    echo "Missing required command: ${1}" >&2
    exit 1
  fi
}

yaml_quote() {
  local value="${1//\'/\'\'}"
  printf "'%s'" "${value}"
}

require_command kubectl
require_command helm

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
  --set prometheus.prometheusSpec.podMonitorSelectorNilUsesHelmValues=false --debug

echo "Installing Grafana Operator ${GRAFANA_OPERATOR_VERSION} release: ${GRAFANA_OPERATOR_RELEASE}"
kubectl apply --server-side --force-conflicts \
  -f "https://github.com/grafana/grafana-operator/releases/download/v${GRAFANA_OPERATOR_VERSION}/crds.yaml"
helm upgrade --install "${GRAFANA_OPERATOR_RELEASE}" \
  oci://ghcr.io/grafana/helm-charts/grafana-operator \
  --namespace "${NAMESPACE}" \
  --create-namespace \
  --version "${GRAFANA_OPERATOR_VERSION}"

if [[ "${INSTALL_GRAFANA_INSTANCE}" == "true" ]]; then
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
else
  echo "Skipping Grafana/GrafanaDatasource creation."
fi

echo "Waiting for Prometheus and Grafana Operator rollouts"
kubectl rollout status -n "${NAMESPACE}" "deployment/${PROMETHEUS_RELEASE}-kube-prometheus-operator" --timeout=180s
kubectl rollout status -n "${NAMESPACE}" "deployment/${GRAFANA_OPERATOR_RELEASE}" --timeout=180s

if [[ "${INSTALL_GRAFANA_INSTANCE}" == "true" ]]; then
  echo "GrafanaDashboard resources with spec.instanceSelector={} will be imported by Grafana instances watched by the operator."
  echo "Install this chart with: --set grafanaDashboard.kind=GrafanaDashboard"
fi

NODE_IP="$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}' 2>/dev/null || true)"
if [[ -n "${NODE_IP}" ]]; then
  echo "Prometheus UI: http://${NODE_IP}:${PROMETHEUS_NODE_PORT}"
  if [[ "${INSTALL_GRAFANA_INSTANCE}" == "true" ]]; then
    echo "Grafana UI: http://${NODE_IP}:${GRAFANA_NODE_PORT}"
  fi
else
  echo "Prometheus UI NodePort: ${PROMETHEUS_NODE_PORT}"
  if [[ "${INSTALL_GRAFANA_INSTANCE}" == "true" ]]; then
    echo "Grafana UI NodePort: ${GRAFANA_NODE_PORT}"
  fi
fi
