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
SWITCH_AGENT_IMAGE="${SWITCH_AGENT_IMAGE:-ghcr.io/unifabric-io/unifabric-switch-agent:latest}"
SWITCH_AGENT_CONTAINER_NAME="${SWITCH_AGENT_CONTAINER_NAME:-unifabric-switch-agent}"
SWITCH_AGENT_REMOTE_DIR="${SWITCH_AGENT_REMOTE_DIR:-/opt/unifabric-switch-agent}"
SWITCH_ARTIFACT_DIR="${SWITCH_ARTIFACT_DIR:-}"
SWITCH_CERT_VALIDITY_DAYS="${SWITCH_CERT_VALIDITY_DAYS:-3650}"

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
  SWITCH_AGENT_IMAGE       Switch-agent image deployed to scale-out switches before cluster bootstrap
  SWITCH_AGENT_CONTAINER_NAME
                           Container name used on switches (default: unifabric-switch-agent)
  SWITCH_AGENT_REMOTE_DIR  Remote base directory for switch-agent state (default: /opt/unifabric-switch-agent)
  SWITCH_ARTIFACT_DIR      Local artifact directory for generated mTLS bundles and Switch YAML
  SWITCH_CERT_VALIDITY_DAYS
                           Validity period for generated pinned mTLS certs (default: 3650)
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

if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl binary not found" >&2
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

copy_file_to_remote_tmp() {
  local node="$1"
  local local_path="$2"
  local remote_tmp="$3"
  "${NVAIR_BIN}" cp "${SIM_ARGS[@]}" "${local_path}" "${node}:${remote_tmp}"
}

install_remote_file_with_sudo() {
  local node="$1"
  local local_path="$2"
  local remote_path="$3"
  local mode="$4"
  local remote_tmp="/tmp/$(basename "${remote_path}").$$"

  copy_file_to_remote_tmp "${node}" "${local_path}" "${remote_tmp}"
  run_remote_bash "${node}" "set -euo pipefail
sudo mkdir -p '$(dirname "${remote_path}")'
sudo install -m '${mode}' '${remote_tmp}' '${remote_path}'
rm -f '${remote_tmp}'"
}

generate_self_signed_cert() {
  local common_name="$1"
  local cert_out="$2"
  local key_out="$3"

  openssl req \
    -x509 \
    -newkey rsa:2048 \
    -sha256 \
    -days "${SWITCH_CERT_VALIDITY_DAYS}" \
    -nodes \
    -subj "/CN=${common_name}" \
    -keyout "${key_out}" \
    -out "${cert_out}" \
    >/dev/null 2>&1
}

generate_switch_mtls_artifacts() {
  local controller_dir="$1"
  local switch_agent_dir="$2"

  mkdir -p "${controller_dir}" "${switch_agent_dir}"
  if [[ -f "${controller_dir}/tls.crt" && -f "${controller_dir}/tls.key" && -f "${controller_dir}/peer.crt" && -f "${switch_agent_dir}/tls.crt" && -f "${switch_agent_dir}/tls.key" && -f "${switch_agent_dir}/peer.crt" ]]; then
    log "Reusing existing pinned mTLS artifacts under ${SWITCH_ARTIFACT_DIR}"
    return
  fi

  log "Generating pinned mTLS artifacts under ${SWITCH_ARTIFACT_DIR}"
  generate_self_signed_cert "unifabric-controller" "${controller_dir}/tls.crt" "${controller_dir}/tls.key"
  generate_self_signed_cert "unifabric-switch-agent" "${switch_agent_dir}/tls.crt" "${switch_agent_dir}/tls.key"
  cp "${switch_agent_dir}/tls.crt" "${controller_dir}/peer.crt"
  cp "${controller_dir}/tls.crt" "${switch_agent_dir}/peer.crt"
  chmod 0644 "${controller_dir}/tls.crt" "${controller_dir}/peer.crt" "${switch_agent_dir}/tls.crt" "${switch_agent_dir}/peer.crt"
  chmod 0600 "${controller_dir}/tls.key" "${switch_agent_dir}/tls.key"
}

primary_ip_of_remote() {
  local node="$1"
  local ip_raw

  ip_raw="$(run_remote_bash "${node}" "set -euo pipefail; hostname -I | tr ' ' '\n' | awk 'NF {print; exit}'")"
  printf '%s\n' "${ip_raw}" | awk 'NF {line=$0} END {print line}'
}

write_switch_resources_manifest() {
  local output_path="$1"
  shift

  : > "${output_path}"
  local switch_name
  for switch_name in "$@"; do
    local mgmt_ip
    mgmt_ip="${SWITCH_MGMT_IPS[${switch_name}]}"
    cat >> "${output_path}" <<EOF
apiVersion: unifabric.io/v1beta1
kind: Switch
metadata:
  name: ${switch_name}
spec:
  mgmtIP: ${mgmt_ip}
  role: ScaleOut
  grpcPort: 8090
---
EOF
  done
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

if [[ -z "${SWITCH_ARTIFACT_DIR}" ]]; then
  SWITCH_ARTIFACT_DIR="${TOPOLOGY_DIR}/.artifacts/${SIMULATION}"
fi
CONTROLLER_MTLS_DIR="${SWITCH_ARTIFACT_DIR}/mtls/controller"
SWITCH_AGENT_MTLS_DIR="${SWITCH_ARTIFACT_DIR}/mtls/switch-agent"
SWITCH_RESOURCES_FILE="${SWITCH_ARTIFACT_DIR}/switch-resources.yaml"

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

step "PHASE 1/8 — Create Simulation"
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

log "Discovering scale-out switches..."
mapfile -t SCALE_OUT_SWITCHES < <(
  "${NVAIR_BIN}" get nodes "${SIM_ARGS[@]}" \
    | awk 'NR>1 && $1 ~ /^switch-gpu-(leaf|spine)/ { print $1 }' \
    | sort -V
)

if ((${#SCALE_OUT_SWITCHES[@]} == 0)); then
  echo "No scale-out switches found (expected names like switch-gpu-leaf* or switch-gpu-spine*)." >&2
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
log "Scale-out switches: ${SCALE_OUT_SWITCHES[*]}"
log "Bootstrap node: ${BOOTSTRAP_NODE}"

declare -A SWITCH_MGMT_IPS=()

step "PHASE 2/8 — Deploy switch-agent to Scale-Out Switches"
mkdir -p "${SWITCH_ARTIFACT_DIR}"
generate_switch_mtls_artifacts "${CONTROLLER_MTLS_DIR}" "${SWITCH_AGENT_MTLS_DIR}"

SWITCH_PREP_SCRIPT="set -euo pipefail
if ! command -v docker >/dev/null 2>&1; then
  echo 'docker not found on switch' >&2
  exit 1
fi
sudo mkdir -p '$(printf '%q' "${SWITCH_AGENT_REMOTE_DIR}/mtls")'"

for switch_name in "${SCALE_OUT_SWITCHES[@]}"; do
  log "Preparing ${switch_name}..."
  run_remote_bash_stream "${switch_name}" "${SWITCH_PREP_SCRIPT}"

  install_remote_file_with_sudo "${switch_name}" "${SWITCH_AGENT_MTLS_DIR}/tls.crt" "${SWITCH_AGENT_REMOTE_DIR}/mtls/tls.crt" 0644
  install_remote_file_with_sudo "${switch_name}" "${SWITCH_AGENT_MTLS_DIR}/tls.key" "${SWITCH_AGENT_REMOTE_DIR}/mtls/tls.key" 0600
  install_remote_file_with_sudo "${switch_name}" "${SWITCH_AGENT_MTLS_DIR}/peer.crt" "${SWITCH_AGENT_REMOTE_DIR}/mtls/peer.crt" 0644

  log "Starting switch-agent on ${switch_name} using ${SWITCH_AGENT_IMAGE}..."
  run_remote_bash_stream "${switch_name}" "set -euo pipefail
sudo docker pull '$(printf '%q' "${SWITCH_AGENT_IMAGE}")'
sudo docker rm -f '$(printf '%q' "${SWITCH_AGENT_CONTAINER_NAME}")' >/dev/null 2>&1 || true
sudo docker run -d \
  --name '$(printf '%q' "${SWITCH_AGENT_CONTAINER_NAME}")' \
  --restart unless-stopped \
  --network host \
  --uts host \
  --privileged \
  -v /proc:/host/proc:ro \
  -v '$(printf '%q' "${SWITCH_AGENT_REMOTE_DIR}")/mtls:/etc/unifabric/switch-mtls:ro' \
  '$(printf '%q' "${SWITCH_AGENT_IMAGE}")' \
  /usr/bin/unifabric/switch-agent
sudo docker ps --filter name='$(printf '%q' "${SWITCH_AGENT_CONTAINER_NAME}")' --format '{{.Names}}' | grep -qx '$(printf '%q' "${SWITCH_AGENT_CONTAINER_NAME}")'"

  SWITCH_MGMT_IPS["${switch_name}"]="$(primary_ip_of_remote "${switch_name}")"
  if [[ -z "${SWITCH_MGMT_IPS[${switch_name}]}" ]]; then
    echo "Failed to detect management IP for ${switch_name}." >&2
    exit 1
  fi
done

write_switch_resources_manifest "${SWITCH_RESOURCES_FILE}" "${SCALE_OUT_SWITCHES[@]}"
log "Switch artifacts written to ${SWITCH_ARTIFACT_DIR}"

step "PHASE 3/8 — Prepare GPU Nodes"
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

step "PHASE 4/8 — Setup Bootstrap Node: ${BOOTSTRAP_NODE}"
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

step "PHASE 5/8 — Distribute SSH Keys"
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

step "PHASE 6/8 — Collect Node IPs"
declare -A NODE_IPS=()
for node in "${GPU_NODES[@]}"; do
  NODE_IPS["${node}"]="$(primary_ip_of_remote "${node}")"
  if [[ -z "${NODE_IPS[${node}]}" ]]; then
    echo "Failed to detect management IP for ${node}." >&2
    exit 1
  fi
done

step "PHASE 7/8 — Deploy Kubernetes"
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


step "PHASE 8/8 — Expose Kubernetes API"
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
log "Switch topology artifact dir: ${SWITCH_ARTIFACT_DIR}"
log "Switch resources manifest: ${SWITCH_RESOURCES_FILE}"
log "External API server: https://${FORWARD_HOSTPORT}"
log "Verify cluster with:"
log "  ${NVAIR_BIN} exec ${BOOTSTRAP_NODE} ${SIM_ARGS[*]} -- kubectl get nodes -o wide"
log "  export KUBECONFIG=${KUBECONFIG_FORWARD_OUT}"
log "  export SWITCH_TOPOLOGY_ARTIFACT_DIR=${SWITCH_ARTIFACT_DIR}"
log "  kubectl get pods -A -o wide"
