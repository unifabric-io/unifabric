#!/usr/bin/env bash

set -euo pipefail

NVAIR_BIN="${NVAIR_BIN:-nvair}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOPOLOGY_DIR="${TOPOLOGY_DIR:-${SCRIPT_DIR}}"
SIMULATION=""
KUBECONFIG_FORWARD_OUT="${KUBECONFIG_FORWARD_OUT:-}"
DELETE_IF_EXISTS="${DELETE_IF_EXISTS:-false}"
BOOTSTRAP_NODE="${BOOTSTRAP_NODE:-gpu-node-1}"
KUBE_API_FORWARD_NAME="${KUBE_API_FORWARD_NAME:-kube-api}"
KUBESPRAY_REPO_URL="${KUBESPRAY_REPO_URL:-https://github.com/kubernetes-sigs/kubespray.git}"
KUBESPRAY_REF="${KUBESPRAY_REF:-v2.26.1}"
KUBESPRAY_DIR="${KUBESPRAY_DIR:-/home/ubuntu/kubespray}"

usage() {
  cat <<'USAGE'
Usage:
  bash examples/simple/install.sh [-o <external-kubeconfig-output>] [--bootstrap-node <name>] [--delete-if-exists]

Options:
  -o, --output             External kubeconfig output path (default: ./kubeconfig-<simulation>-external.yaml)
  -b, --bootstrap-node     Preferred bootstrap node for Kubespray (default: gpu-node-1)
  --delete-if-exists       Delete same-name simulation before create (default: false)
  -h, --help               Show this help

Environment:
  NVAIR_BIN                Path to nvair binary (default: nvair)
  TOPOLOGY_DIR             Topology directory for nvair create (default: examples/simple)
  KUBECONFIG_FORWARD_OUT   External kubeconfig output path (same as --output)
  DELETE_IF_EXISTS         true|false (default: false)
  BOOTSTRAP_NODE           Same as --bootstrap-node
  KUBE_API_FORWARD_NAME    Forward name for Kubernetes API access (default: kube-api)
  KUBESPRAY_REPO_URL       Kubespray git repo URL
  KUBESPRAY_REF            Optional git ref/branch/tag to checkout
  KUBESPRAY_DIR            Kubespray directory on bootstrap node (default: /home/ubuntu/kubespray)
USAGE
}

while (($# > 0)); do
  case "$1" in
    -o|--output)
      if (($# < 2)) || [[ -z "${2:-}" ]]; then
        echo "Missing value for $1" >&2
        usage
        exit 1
      fi
      KUBECONFIG_FORWARD_OUT="${2}"
      shift 2
      ;;
    -b|--bootstrap-node)
      if (($# < 2)) || [[ -z "${2:-}" ]]; then
        echo "Missing value for $1" >&2
        usage
        exit 1
      fi
      BOOTSTRAP_NODE="${2}"
      shift 2
      ;;
    --delete-if-exists)
      DELETE_IF_EXISTS="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if ! command -v "${NVAIR_BIN}" >/dev/null 2>&1; then
  echo "nvair binary not found: ${NVAIR_BIN}" >&2
  exit 1
fi

log() {
  echo "[kubespray-install] $*"
}

# Print a green phase banner
step() {
  local GREEN="\033[0;32m"
  local BOLD="\033[1m"
  local RESET="\033[0m"
  echo
  printf "${BOLD}${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}\n"
  printf "${BOLD}${GREEN}  %s${RESET}\n" "$*"
  printf "${BOLD}${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}\n"
}

contains_node() {
  local candidate="$1"
  shift
  local node
  for node in "$@"; do
    if [[ "${node}" == "${candidate}" ]]; then
      return 0
    fi
  done
  return 1
}

run_remote_bash() {
  local node="$1"
  local script="$2"
  "${NVAIR_BIN}" exec "${node}" "${SIM_ARGS[@]}" -- bash -lc "${script}"
}

run_remote_bash_stream() {
  local node="$1"
  local script="$2"
  if [[ -t 0 && -t 1 ]]; then
    "${NVAIR_BIN}" exec "${node}" "${SIM_ARGS[@]}" -it -- bash -lc "${script}"
  else
    log "No TTY detected; falling back to buffered output for ${node}."
    run_remote_bash "${node}" "${script}"
  fi
}

if [[ ! -f "${TOPOLOGY_DIR}/topology.json" ]]; then
  echo "topology.json not found in ${TOPOLOGY_DIR}" >&2
  exit 1
fi

SIMULATION="$(awk -F'"' '/"title"[[:space:]]*:[[:space:]]*"/ { print $4; exit }' "${TOPOLOGY_DIR}/topology.json")"
if [[ -z "${SIMULATION}" ]]; then
  echo "failed to parse simulation title from ${TOPOLOGY_DIR}/topology.json" >&2
  exit 1
fi
SIM_ARGS=(-s "${SIMULATION}")

if [[ -z "${KUBECONFIG_FORWARD_OUT}" ]]; then
  KUBECONFIG_FORWARD_OUT="./kubeconfig-${SIMULATION}-external.yaml"
fi

DELETE_IF_EXISTS="$(printf '%s' "${DELETE_IF_EXISTS}" | tr '[:upper:]' '[:lower:]')"
case "${DELETE_IF_EXISTS}" in
  true|false) ;;
  *)
    echo "DELETE_IF_EXISTS must be true or false (got: ${DELETE_IF_EXISTS})" >&2
    exit 1
    ;;
esac

CREATE_ARGS=(-d "${TOPOLOGY_DIR}")
if [[ "${DELETE_IF_EXISTS}" == "true" ]]; then
  CREATE_ARGS=(--delete-if-exists "${CREATE_ARGS[@]}")
fi

step "PHASE 1/7 — Create Simulation"
log "Creating simulation from ${TOPOLOGY_DIR} (delete-if-exists=${DELETE_IF_EXISTS})..."
log "[CMD] ${NVAIR_BIN} create ${CREATE_ARGS[*]}"
"${NVAIR_BIN}" create "${CREATE_ARGS[@]}"

log "Discovering GPU nodes..."
log "[CMD] ${NVAIR_BIN} get nodes ${SIM_ARGS[*]}"
mapfile -t GPU_NODES < <(
  "${NVAIR_BIN}" get nodes "${SIM_ARGS[@]}" \
    | awk 'NR>1 && ($1 ~ /^node-gpu-/ || $1 ~ /^gpu-node-/) { print $1 }' \
    | sort -V
)

if ((${#GPU_NODES[@]} == 0)); then
  echo "No GPU nodes found (expected names like node-gpu-* or gpu-node-*)." >&2
  exit 1
fi

if ! contains_node "${BOOTSTRAP_NODE}" "${GPU_NODES[@]}"; then
  if contains_node "node-gpu-1" "${GPU_NODES[@]}"; then
    BOOTSTRAP_NODE="node-gpu-1"
  elif contains_node "gpu-node-1" "${GPU_NODES[@]}"; then
    BOOTSTRAP_NODE="gpu-node-1"
  else
    BOOTSTRAP_NODE="${GPU_NODES[0]}"
  fi
fi

log "GPU nodes: ${GPU_NODES[*]}"
log "Bootstrap node: ${BOOTSTRAP_NODE}"

step "PHASE 2/7 — Prepare GPU Nodes"
COMMON_PREP_SCRIPT='set -euo pipefail
if command -v systemctl >/dev/null 2>&1; then
  if systemctl list-unit-files | grep -q "^unattended-upgrades\.service"; then
    sudo systemctl stop unattended-upgrades.service || true
    sudo systemctl disable unattended-upgrades.service || true
  fi
  if systemctl list-unit-files | grep -q "^apt-daily-upgrade\.timer"; then
    sudo systemctl stop apt-daily-upgrade.timer || true
    sudo systemctl disable apt-daily-upgrade.timer || true
  fi
  if systemctl list-unit-files | grep -q "^apt-daily-upgrade\.service"; then
    sudo systemctl stop apt-daily-upgrade.service || true
    sudo systemctl disable apt-daily-upgrade.service || true
  fi
fi
sudo apt-get update -y
sudo DEBIAN_FRONTEND=noninteractive apt-get install -y python3 python3-apt sudo openssh-server curl
if systemctl list-unit-files | grep -q "^ssh\.service"; then
  sudo systemctl enable --now ssh
elif systemctl list-unit-files | grep -q "^sshd\.service"; then
  sudo systemctl enable --now sshd
fi
printf "ubuntu ALL=(ALL) NOPASSWD:ALL\n" | sudo tee /etc/sudoers.d/99-ubuntu-nopasswd >/dev/null
sudo chmod 0440 /etc/sudoers.d/99-ubuntu-nopasswd
mkdir -p "$HOME/.ssh"
chmod 700 "$HOME/.ssh"
touch "$HOME/.ssh/authorized_keys"
chmod 600 "$HOME/.ssh/authorized_keys"'

log "Preparing GPU nodes (python/ssh/sudo)..."
for node in "${GPU_NODES[@]}"; do
  log "Preparing ${node}..."
  run_remote_bash_stream "${node}" "${COMMON_PREP_SCRIPT}"
done

BOOTSTRAP_SETUP_SCRIPT="set -euo pipefail
KUBESPRAY_REPO_URL=$(printf '%q' "${KUBESPRAY_REPO_URL}")
KUBESPRAY_REF=$(printf '%q' "${KUBESPRAY_REF}")
KUBESPRAY_DIR=$(printf '%q' "${KUBESPRAY_DIR}")
sudo apt-get update -y
sudo DEBIAN_FRONTEND=noninteractive apt-get install -y git python3 python3-venv python3-pip rsync openssh-client
if [[ ! -d \"\${KUBESPRAY_DIR}\" ]]; then
  git clone \"\${KUBESPRAY_REPO_URL}\" \"\${KUBESPRAY_DIR}\"
fi
cd \"\${KUBESPRAY_DIR}\"
if [[ -n \"\${KUBESPRAY_REF}\" ]]; then
  git fetch --all --tags
  git checkout \"\${KUBESPRAY_REF}\"
fi
python3 -m venv .venv
source .venv/bin/activate
pip install --upgrade pip
pip install -r requirements.txt
mkdir -p \"\$HOME/.ssh\"
chmod 700 \"\$HOME/.ssh\"
if [[ ! -f \"\$HOME/.ssh/id_ed25519\" ]]; then
  ssh-keygen -t ed25519 -N '' -f \"\$HOME/.ssh/id_ed25519\" -C \"kubespray@\$(hostname)\"
fi"

step "PHASE 3/7 — Setup Bootstrap Node: ${BOOTSTRAP_NODE}"
log "Setting up Kubespray on bootstrap node ${BOOTSTRAP_NODE}..."
run_remote_bash_stream "${BOOTSTRAP_NODE}" "${BOOTSTRAP_SETUP_SCRIPT}"
PUB_RAW="$(run_remote_bash "${BOOTSTRAP_NODE}" "set -euo pipefail; cat \"\$HOME/.ssh/id_ed25519.pub\"")"
PUB_KEY="$(printf '%s\n' "${PUB_RAW}" | awk '/^(ssh-ed25519|ssh-rsa) / {line=$0} END {print line}')"

if [[ -z "${PUB_KEY}" ]]; then
  echo "Failed to get SSH public key from ${BOOTSTRAP_NODE}." >&2
  printf '%s\n' "${PUB_RAW}" >&2
  exit 1
fi

PUB_KEY_B64="$(printf '%s' "${PUB_KEY}" | base64 | tr -d '\n')"

step "PHASE 4/7 — Distribute SSH Keys"
log "Distributing bootstrap SSH public key to all GPU nodes..."
for node in "${GPU_NODES[@]}"; do
  log "Authorizing key on ${node}..."
  run_remote_bash "${node}" "set -euo pipefail
PUB_KEY_B64='${PUB_KEY_B64}'
PUB_KEY=\"\$(printf '%s' \"\${PUB_KEY_B64}\" | base64 -d)\"
mkdir -p \"\$HOME/.ssh\"
chmod 700 \"\$HOME/.ssh\"
touch \"\$HOME/.ssh/authorized_keys\"
grep -qxF \"\${PUB_KEY}\" \"\$HOME/.ssh/authorized_keys\" || echo \"\${PUB_KEY}\" >> \"\$HOME/.ssh/authorized_keys\"
chmod 600 \"\$HOME/.ssh/authorized_keys\""
done

step "PHASE 5/7 — Collect Node IPs"
declare -A NODE_IPS=()
for node in "${GPU_NODES[@]}"; do
  IP_RAW="$(run_remote_bash "${node}" "set -euo pipefail; hostname -I | tr ' ' '\\n' | awk 'NF {print; exit}'")"
  NODE_IPS["${node}"]="$(printf '%s\n' "${IP_RAW}" | awk 'NF {line=$0} END {print line}')"
  if [[ -z "${NODE_IPS[${node}]}" ]]; then
    echo "Failed to detect management IP for ${node}." >&2
    exit 1
  fi
done

step "PHASE 6/7 — Deploy Kubernetes"
log "Generating Kubespray inventory..."
HOSTS_FILE="$(mktemp)"
cat > "${HOSTS_FILE}" <<'YAML_HEAD'
all:
  hosts:
YAML_HEAD

for node in "${GPU_NODES[@]}"; do
  ip="${NODE_IPS[${node}]}"
  cat >> "${HOSTS_FILE}" <<YAML_HOST
    ${node}:
      ansible_host: ${ip}
      ip: ${ip}
      access_ip: ${ip}
      ansible_user: ubuntu
YAML_HOST
done

cat >> "${HOSTS_FILE}" <<YAML_TAIL
  children:
    kube_control_plane:
      hosts:
        ${BOOTSTRAP_NODE}:
    kube_node:
      hosts:
YAML_TAIL

for node in "${GPU_NODES[@]}"; do
  echo "        ${node}:" >> "${HOSTS_FILE}"
done

cat >> "${HOSTS_FILE}" <<YAML_ETCD
    etcd:
      hosts:
        ${BOOTSTRAP_NODE}:
    k8s_cluster:
      children:
        kube_control_plane:
        kube_node:
    calico_rr:
      hosts: {}
YAML_ETCD

run_remote_bash "${BOOTSTRAP_NODE}" "set -euo pipefail
KUBESPRAY_DIR=$(printf '%q' "${KUBESPRAY_DIR}")
cd \"\${KUBESPRAY_DIR}\"
rm -rf inventory/nvair
cp -rfp inventory/sample inventory/nvair
mkdir -p inventory/nvair/group_vars/k8s_cluster"

log "[CMD] ${NVAIR_BIN} cp ${SIM_ARGS[*]} ${HOSTS_FILE} ${BOOTSTRAP_NODE}:${KUBESPRAY_DIR}/inventory/nvair/hosts.yaml"
"${NVAIR_BIN}" cp "${SIM_ARGS[@]}" "${HOSTS_FILE}" "${BOOTSTRAP_NODE}:${KUBESPRAY_DIR}/inventory/nvair/hosts.yaml"
rm -f "${HOSTS_FILE}"

run_remote_bash "${BOOTSTRAP_NODE}" "set -euo pipefail
KUBESPRAY_DIR=$(printf '%q' "${KUBESPRAY_DIR}")
cat > \"\${KUBESPRAY_DIR}/inventory/nvair/group_vars/k8s_cluster/nvair.yml\" <<'EOF_OVERRIDES'
container_manager: containerd
kube_network_plugin: calico
EOF_OVERRIDES"

log "Running Kubespray cluster deployment from ${BOOTSTRAP_NODE}..."
run_remote_bash_stream "${BOOTSTRAP_NODE}" "set -euo pipefail
KUBESPRAY_DIR=$(printf '%q' "${KUBESPRAY_DIR}")
cd \"\${KUBESPRAY_DIR}\"
source .venv/bin/activate
export ANSIBLE_HOST_KEY_CHECKING=False
ansible-playbook -i inventory/nvair/hosts.yaml --become --become-user=root cluster.yml"

log "Preparing kubeconfig on ${BOOTSTRAP_NODE}..."
run_remote_bash "${BOOTSTRAP_NODE}" "set -euo pipefail
mkdir -p \"\$HOME/.kube\"
sudo cp /etc/kubernetes/admin.conf \"\$HOME/.kube/config\"
sudo chown \"\$(id -u):\$(id -g)\" \"\$HOME/.kube/config\""


step "PHASE 7/7 — Expose Kubernetes API"
log "Ensuring forward ${KUBE_API_FORWARD_NAME} for Kubernetes API (${BOOTSTRAP_NODE}:6443)..."
log "[CMD] ${NVAIR_BIN} get forward ${SIM_ARGS[*]}"
FORWARD_LIST="$("${NVAIR_BIN}" get forward "${SIM_ARGS[@]}")"
EXISTING_FORWARD_TARGET="$(awk -v name="${KUBE_API_FORWARD_NAME}" 'NR>1 && $1 == name { print $3; exit }' <<<"${FORWARD_LIST}")"
if [[ -z "${EXISTING_FORWARD_TARGET}" ]]; then
  log "[CMD] ${NVAIR_BIN} add forward ${KUBE_API_FORWARD_NAME} ${SIM_ARGS[*]} --target-node ${BOOTSTRAP_NODE} --target-port 6443"
  "${NVAIR_BIN}" add forward "${KUBE_API_FORWARD_NAME}" "${SIM_ARGS[@]}" --target-node "${BOOTSTRAP_NODE}" --target-port 6443
  log "[CMD] ${NVAIR_BIN} get forward ${SIM_ARGS[*]}"
  FORWARD_LIST="$("${NVAIR_BIN}" get forward "${SIM_ARGS[@]}")"
elif [[ "${EXISTING_FORWARD_TARGET}" != "${BOOTSTRAP_NODE}:6443" ]]; then
  echo "Forward ${KUBE_API_FORWARD_NAME} already points to ${EXISTING_FORWARD_TARGET}; expected ${BOOTSTRAP_NODE}:6443." >&2
  echo "Delete it or set KUBE_API_FORWARD_NAME to a different name." >&2
  exit 1
else
  log "Forward ${KUBE_API_FORWARD_NAME} already exists for ${EXISTING_FORWARD_TARGET}; reusing it."
fi

log "Resolving external forward address for ${KUBE_API_FORWARD_NAME}..."
FORWARD_HOSTPORT="$(awk -v name="${KUBE_API_FORWARD_NAME}" 'NR>1 && $1 == name { print $2; exit }' <<<"${FORWARD_LIST}")"
if [[ -z "${FORWARD_HOSTPORT}" ]]; then
  echo "Failed to resolve forward address for ${KUBE_API_FORWARD_NAME}." >&2
  exit 1
fi

REMOTE_EXTERNAL_KUBECONFIG="/home/ubuntu/.kube/config-external"
log "Generating external kubeconfig on ${BOOTSTRAP_NODE} (installing yq there if needed)..."
run_remote_bash "${BOOTSTRAP_NODE}" "set -euo pipefail
FORWARD_HOSTPORT='${FORWARD_HOSTPORT}'
REMOTE_EXTERNAL_KUBECONFIG='${REMOTE_EXTERNAL_KUBECONFIG}'
if ! command -v yq >/dev/null 2>&1; then
  sudo curl -fsSL -o /usr/local/bin/yq https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64
  sudo chmod +x /usr/local/bin/yq
fi
cp \"\$HOME/.kube/config\" \"\${REMOTE_EXTERNAL_KUBECONFIG}\"
yq -i 'del(.clusters[].cluster.certificate-authority-data)' \"\${REMOTE_EXTERNAL_KUBECONFIG}\"
yq -i '.clusters[].cluster.insecure-skip-tls-verify = true' \"\${REMOTE_EXTERNAL_KUBECONFIG}\"
yq -i \".clusters[].cluster.server = \\\"https://\${FORWARD_HOSTPORT}\\\"\" \"\${REMOTE_EXTERNAL_KUBECONFIG}\""

log "Exporting external kubeconfig to local file: ${KUBECONFIG_FORWARD_OUT}"
log "[CMD] ${NVAIR_BIN} cp ${SIM_ARGS[*]} ${BOOTSTRAP_NODE}:${REMOTE_EXTERNAL_KUBECONFIG} ${KUBECONFIG_FORWARD_OUT}"
"${NVAIR_BIN}" cp "${SIM_ARGS[@]}" "${BOOTSTRAP_NODE}:${REMOTE_EXTERNAL_KUBECONFIG}" "${KUBECONFIG_FORWARD_OUT}"

log "Done."
log "Bootstrap node: ${BOOTSTRAP_NODE}"
log "External kubeconfig file: ${KUBECONFIG_FORWARD_OUT}"
log "External API server: https://${FORWARD_HOSTPORT}"
log "Verify cluster with:"
log "  ${NVAIR_BIN} exec ${BOOTSTRAP_NODE} ${SIM_ARGS[*]} -- kubectl get nodes -o wide"
log "  export KUBECONFIG=${KUBECONFIG_FORWARD_OUT}"
log "  kubectl get pods -A -o wide"
