#!/usr/bin/env bash

set -euo pipefail

SPIDERPOOL_VERSION="${SPIDERPOOL_VERSION:-1.1.1}"
SPIDERPOOL_NAMESPACE="${SPIDERPOOL_NAMESPACE:-spiderpool}"
SPIDERPOOL_RELEASE="${SPIDERPOOL_RELEASE:-spiderpool}"
SPIDERPOOL_REPO_NAME="${SPIDERPOOL_REPO_NAME:-spiderpool}"
SPIDERPOOL_REPO_URL="${SPIDERPOOL_REPO_URL:-https://spidernet-io.github.io/spiderpool}"
SPIDERPOOL_HELM_TIMEOUT="${SPIDERPOOL_HELM_TIMEOUT:-10m}"

SPIDERPOOL_ENABLE_IPV4="${SPIDERPOOL_ENABLE_IPV4:-true}"
SPIDERPOOL_ENABLE_IPV6="${SPIDERPOOL_ENABLE_IPV6:-false}"
SPIDERPOOL_ENABLE_RDMA_SHARED_DEVICE_PLUGIN="${SPIDERPOOL_ENABLE_RDMA_SHARED_DEVICE_PLUGIN:-false}"

MACVLAN_CONFIG_NAME="${MACVLAN_CONFIG_NAME:-macvlan-eth1}"
MACVLAN_MASTER="${MACVLAN_MASTER:-eth1}"
MACVLAN_ENABLE_COORDINATOR="${MACVLAN_ENABLE_COORDINATOR:-false}"
MACVLAN_VLAN_ID="${MACVLAN_VLAN_ID:-}"
MACVLAN_POD_DEFAULT_ROUTE_NIC="${MACVLAN_POD_DEFAULT_ROUTE_NIC:-}"

SPIDERPOOL_CREATE_IPV4_POOL="${SPIDERPOOL_CREATE_IPV4_POOL:-true}"
SPIDERPOOL_IPV4_POOL_NAME="${SPIDERPOOL_IPV4_POOL_NAME:-macvlan-eth1-v4}"
SPIDERPOOL_IPV4_SUBNET="${SPIDERPOOL_IPV4_SUBNET:-172.17.1.0/24}"
SPIDERPOOL_IPV4_GATEWAY="${SPIDERPOOL_IPV4_GATEWAY:-172.17.1.1}"
SPIDERPOOL_IPV4_IP_RANGES="${SPIDERPOOL_IPV4_IP_RANGES:-172.17.1.200-172.17.1.239}"
SPIDERPOOL_IPV4_EXCLUDE_IPS="${SPIDERPOOL_IPV4_EXCLUDE_IPS:-}"

usage() {
  cat <<'EOF'
Usage:
  e2e/topology/script/install-spiderpool-macvlan.sh

Installs Spiderpool with Multus and CNI plugins, then creates a macvlan
SpiderMultusConfig and, by default, an IPv4 SpiderIPPool for that network.

Optional environment variables:
  SPIDERPOOL_VERSION                         Chart version. Default: 1.1.1
  SPIDERPOOL_NAMESPACE                       Namespace. Default: spiderpool
  SPIDERPOOL_RELEASE                         Helm release. Default: spiderpool
  SPIDERPOOL_REPO_URL                        Helm repo URL.
  SPIDERPOOL_HELM_TIMEOUT                    Helm wait timeout. Default: 10m
  SPIDERPOOL_ENABLE_IPV4                     Enable IPv4 IPAM. Default: true
  SPIDERPOOL_ENABLE_IPV6                     Enable IPv6 IPAM. Default: false
  SPIDERPOOL_ENABLE_RDMA_SHARED_DEVICE_PLUGIN
                                             Install RDMA shared device plugin. Default: false

  MACVLAN_CONFIG_NAME                        SpiderMultusConfig/NAD name. Default: macvlan-eth1
  MACVLAN_MASTER                             Host interface used as macvlan master. Default: eth1
  MACVLAN_ENABLE_COORDINATOR                 Chain Spiderpool coordinator for this network.
                                             Default: false. Set true only when the previous
                                             CNI interface also uses coordinator.
  MACVLAN_VLAN_ID                            Optional VLAN ID.
  MACVLAN_POD_DEFAULT_ROUTE_NIC              Optional pod default route NIC.

  SPIDERPOOL_CREATE_IPV4_POOL                true|false. Default: true
  SPIDERPOOL_IPV4_POOL_NAME                  IPv4 pool name. Default: macvlan-eth1-v4
  SPIDERPOOL_IPV4_SUBNET                     IPv4 subnet. Default: 172.17.1.0/24
  SPIDERPOOL_IPV4_GATEWAY                    IPv4 gateway. Default: 172.17.1.1
  SPIDERPOOL_IPV4_IP_RANGES                  Comma/space-separated ranges.
                                             Default: 172.17.1.200-172.17.1.239
  SPIDERPOOL_IPV4_EXCLUDE_IPS                Optional comma/space-separated exclusions.

Pod usage example after installation:
  k8s.v1.cni.cncf.io/networks: spiderpool/macvlan-eth1
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

require_bool() {
  local name="$1"

  case "${!name}" in
    true|false) ;;
    *)
      echo "${name} must be true or false (got: ${!name})" >&2
      exit 2
      ;;
  esac
}

print_command() {
  printf '%q' "$1"
  shift
  printf ' %q' "$@"
  printf '\n'
}

yaml_quote() {
  local value="${1//\'/\'\'}"
  printf "'%s'" "${value}"
}

normalize_list() {
  local value="$1"

  value="${value//,/ }"
  # shellcheck disable=SC2086
  printf '%s\n' ${value}
}

emit_yaml_string_list() {
  local indent="$1"
  local values="$2"
  local item

  if [[ -z "${values}" ]]; then
    return
  fi

  while IFS= read -r item; do
    [[ -n "${item}" ]] || continue
    printf '%*s- %s\n' "${indent}" "" "$(yaml_quote "${item}")"
  done < <(normalize_list "${values}")
}

require_command helm
require_command kubectl

require_non_empty SPIDERPOOL_VERSION
require_non_empty SPIDERPOOL_NAMESPACE
require_non_empty SPIDERPOOL_RELEASE
require_non_empty SPIDERPOOL_REPO_URL
require_non_empty MACVLAN_CONFIG_NAME
require_non_empty MACVLAN_MASTER

require_bool SPIDERPOOL_ENABLE_IPV4
require_bool SPIDERPOOL_ENABLE_IPV6
require_bool SPIDERPOOL_ENABLE_RDMA_SHARED_DEVICE_PLUGIN
require_bool SPIDERPOOL_CREATE_IPV4_POOL
require_bool MACVLAN_ENABLE_COORDINATOR

if [[ "${SPIDERPOOL_CREATE_IPV4_POOL}" == "true" ]]; then
  require_non_empty SPIDERPOOL_IPV4_POOL_NAME
  require_non_empty SPIDERPOOL_IPV4_SUBNET
  require_non_empty SPIDERPOOL_IPV4_IP_RANGES
fi

echo "[spiderpool-macvlan] Installing Spiderpool ${SPIDERPOOL_VERSION} into namespace ${SPIDERPOOL_NAMESPACE}"
echo "[spiderpool-macvlan] KUBECONFIG=${KUBECONFIG:-<default>}"
echo "[spiderpool-macvlan] macvlan master=${MACVLAN_MASTER}, config=${MACVLAN_CONFIG_NAME}"

kubectl create namespace "${SPIDERPOOL_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

HELM_REPO_ADD_ARGS=(repo add "${SPIDERPOOL_REPO_NAME}" "${SPIDERPOOL_REPO_URL}")
print_command helm "${HELM_REPO_ADD_ARGS[@]}"
helm "${HELM_REPO_ADD_ARGS[@]}" >/dev/null

HELM_REPO_UPDATE_ARGS=(repo update "${SPIDERPOOL_REPO_NAME}")
print_command helm "${HELM_REPO_UPDATE_ARGS[@]}"
helm "${HELM_REPO_UPDATE_ARGS[@]}" >/dev/null

HELM_ARGS=(
  upgrade --install "${SPIDERPOOL_RELEASE}" "${SPIDERPOOL_REPO_NAME}/spiderpool"
  --namespace "${SPIDERPOOL_NAMESPACE}"
  --create-namespace
  --version "${SPIDERPOOL_VERSION}"
  --wait
  --timeout "${SPIDERPOOL_HELM_TIMEOUT}"

  --set "ipam.enableIPv4=${SPIDERPOOL_ENABLE_IPV4}"
  --set "ipam.enableIPv6=${SPIDERPOOL_ENABLE_IPV6}"
  --set coordinator.enabled=true
  --set-string coordinator.mode=underlay

  --set multus.enableMultusConfig=true
  --set multus.multusCNI.install=true
  --set plugins.installCNI=true

  --set "rdma.rdmaSharedDevicePlugin.install=${SPIDERPOOL_ENABLE_RDMA_SHARED_DEVICE_PLUGIN}"

  --set clusterDefaultPool.installIPv4IPPool=false
  --set clusterDefaultPool.installIPv6IPPool=false
)

print_command helm "${HELM_ARGS[@]}"
helm "${HELM_ARGS[@]}"

echo "[spiderpool-macvlan] Waiting for Spiderpool workloads"
kubectl -n "${SPIDERPOOL_NAMESPACE}" rollout status daemonset/spiderpool-agent --timeout="${SPIDERPOOL_HELM_TIMEOUT}"
kubectl -n "${SPIDERPOOL_NAMESPACE}" rollout status deployment/spiderpool-controller --timeout="${SPIDERPOOL_HELM_TIMEOUT}"

# The chart's auto TLS mode regenerates webhook CA material on each Helm upgrade.
# Restart the controller so the serving cert mounted from the Secret matches the
# webhook caBundle before applying Spiderpool CRs.
echo "[spiderpool-macvlan] Restarting Spiderpool controller to refresh webhook TLS"
kubectl -n "${SPIDERPOOL_NAMESPACE}" rollout restart deployment/spiderpool-controller
kubectl -n "${SPIDERPOOL_NAMESPACE}" rollout status deployment/spiderpool-controller --timeout="${SPIDERPOOL_HELM_TIMEOUT}"

echo "[spiderpool-macvlan] Creating SpiderMultusConfig ${SPIDERPOOL_NAMESPACE}/${MACVLAN_CONFIG_NAME}"
kubectl apply -f - <<EOF
apiVersion: spiderpool.spidernet.io/v2beta1
kind: SpiderMultusConfig
metadata:
  name: ${MACVLAN_CONFIG_NAME}
  namespace: ${SPIDERPOOL_NAMESPACE}
spec:
  cniType: macvlan
  enableCoordinator: ${MACVLAN_ENABLE_COORDINATOR}
$(if [[ "${MACVLAN_ENABLE_COORDINATOR}" == "true" ]]; then cat <<EOF_COORDINATOR
  coordinator:
    mode: underlay
$(if [[ -n "${MACVLAN_POD_DEFAULT_ROUTE_NIC}" ]]; then printf '    podDefaultRouteNIC: %s\n' "$(yaml_quote "${MACVLAN_POD_DEFAULT_ROUTE_NIC}")"; fi)
EOF_COORDINATOR
fi)
  macvlan:
    master:
      - ${MACVLAN_MASTER}
$(if [[ -n "${MACVLAN_VLAN_ID}" ]]; then printf '    vlanID: %s\n' "${MACVLAN_VLAN_ID}"; fi)
$(if [[ "${SPIDERPOOL_CREATE_IPV4_POOL}" == "true" ]]; then cat <<EOF_POOL_REF
    ippools:
      ipv4:
        - ${SPIDERPOOL_IPV4_POOL_NAME}
EOF_POOL_REF
fi)
EOF

if [[ "${SPIDERPOOL_CREATE_IPV4_POOL}" == "true" ]]; then
  echo "[spiderpool-macvlan] Creating SpiderIPPool ${SPIDERPOOL_IPV4_POOL_NAME}"
  kubectl apply -f - <<EOF
apiVersion: spiderpool.spidernet.io/v2beta1
kind: SpiderIPPool
metadata:
  name: ${SPIDERPOOL_IPV4_POOL_NAME}
spec:
  ipVersion: 4
  subnet: ${SPIDERPOOL_IPV4_SUBNET}
$(if [[ -n "${SPIDERPOOL_IPV4_GATEWAY}" ]]; then printf '  gateway: %s\n' "${SPIDERPOOL_IPV4_GATEWAY}"; fi)
  ips:
$(emit_yaml_string_list 4 "${SPIDERPOOL_IPV4_IP_RANGES}")
$(if [[ -n "${SPIDERPOOL_IPV4_EXCLUDE_IPS}" ]]; then cat <<EOF_EXCLUDE
  excludeIPs:
$(emit_yaml_string_list 4 "${SPIDERPOOL_IPV4_EXCLUDE_IPS}")
EOF_EXCLUDE
fi)
  multusName:
    - ${SPIDERPOOL_NAMESPACE}/${MACVLAN_CONFIG_NAME}
EOF
fi

echo "[spiderpool-macvlan] Waiting for generated NetworkAttachmentDefinition ${SPIDERPOOL_NAMESPACE}/${MACVLAN_CONFIG_NAME}"
for _ in $(seq 1 60); do
  if kubectl -n "${SPIDERPOOL_NAMESPACE}" get network-attachment-definition "${MACVLAN_CONFIG_NAME}" >/dev/null 2>&1; then
    break
  fi
  sleep 2
done

kubectl -n "${SPIDERPOOL_NAMESPACE}" get network-attachment-definition "${MACVLAN_CONFIG_NAME}" >/dev/null

echo "[spiderpool-macvlan] Installed resources:"
kubectl -n "${SPIDERPOOL_NAMESPACE}" get pods -o wide
kubectl -n "${SPIDERPOOL_NAMESPACE}" get spidermultusconfig "${MACVLAN_CONFIG_NAME}" -o wide
kubectl -n "${SPIDERPOOL_NAMESPACE}" get network-attachment-definition "${MACVLAN_CONFIG_NAME}" -o name
if [[ "${SPIDERPOOL_CREATE_IPV4_POOL}" == "true" ]]; then
  kubectl get spiderippool "${SPIDERPOOL_IPV4_POOL_NAME}" -o wide
fi

echo "[spiderpool-macvlan] Done. Use annotation: k8s.v1.cni.cncf.io/networks: ${SPIDERPOOL_NAMESPACE}/${MACVLAN_CONFIG_NAME}"
