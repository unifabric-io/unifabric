#!/usr/bin/env bash
# Deploy switch-agent mTLS certificates and recreate the remote container.
set -uo pipefail

usage() {
  cat <<'EOF'
Usage:
  SSH_USER=<user> SWITCH_AGENT_IMAGE=<image> HOSTS=<ip,ip,...> \
    [options] deploy-switch-agent.sh

Options are configured with environment variables:
  SSH_USER             Required remote SSH user
  SSH_PORT             Remote SSH port (default: 22)
  SSH_AUTH_MODE        key or password (default: key)
  SSH_PASSWORD         SSH password; prompted when password mode uses a TTY
  SUDO_AUTH_MODE       passwordless or password
                       (default: passwordless for key, password for SSH password)
  SUDO_PASSWORD        sudo password; prompted when required
  CERT_SOURCE_DIR      Directory containing peer.crt, tls.crt, tls.key
                       (default: ./tmp-switch-mtls)
  HOSTS                Comma-separated switch management IPs
  LLDP_COLLECTION_MODE socket or hostProc (default: socket)
  LLDP_SOCKET_PATH     Host lldpd socket used in socket mode
                       (default: /run/lldpd.socket)
  GRPC_PORT            Host and switch-agent gRPC port (default: 8090)
  REMOTE_UPLOAD_DIR    Temporary remote upload directory
  REMOTE_CERT_DIR      Remote certificate directory
  CONTAINER_NAME       Remote container name
  SWITCH_AGENT_IMAGE   Required preloaded switch-agent image
EOF
}

SSH_USER="${SSH_USER:-}"
SSH_PORT="${SSH_PORT:-22}"
SSH_AUTH_MODE="${SSH_AUTH_MODE:-key}"
SSH_PASSWORD="${SSH_PASSWORD:-}"
SUDO_AUTH_MODE="${SUDO_AUTH_MODE:-}"
SUDO_PASSWORD="${SUDO_PASSWORD:-}"
CERT_SOURCE_DIR="${CERT_SOURCE_DIR:-./tmp-switch-mtls}"
HOSTS_CSV="${HOSTS:-}"
LLDP_COLLECTION_MODE="${LLDP_COLLECTION_MODE:-socket}"
LLDP_SOCKET_PATH="${LLDP_SOCKET_PATH:-/run/lldpd.socket}"
GRPC_PORT="${GRPC_PORT:-8090}"
REMOTE_UPLOAD_DIR="${REMOTE_UPLOAD_DIR:-/tmp/unifabric-switch-agent-${SSH_USER}}"
REMOTE_CERT_DIR="${REMOTE_CERT_DIR:-/opt/unifabric-switch-agent/mtls}"
REMOTE_CERT_OWNER="${REMOTE_CERT_OWNER:-root}"
REMOTE_CERT_GROUP="${REMOTE_CERT_GROUP:-root}"
CONTAINER_NAME="${CONTAINER_NAME:-unifabric-switch-agent}"
SWITCH_AGENT_IMAGE="${SWITCH_AGENT_IMAGE:-}"

if [[ -z "${SSH_USER}" ]]; then
  echo "SSH_USER is required" >&2
  usage >&2
  exit 1
fi

if [[ -z "${SWITCH_AGENT_IMAGE}" ]]; then
  echo "SWITCH_AGENT_IMAGE is required" >&2
  usage >&2
  exit 1
fi

case "${SSH_AUTH_MODE}" in
  key)
    ;;
  password)
    if ! command -v sshpass >/dev/null 2>&1; then
      echo "sshpass is required when SSH_AUTH_MODE=password" >&2
      exit 1
    fi
    if [[ -z "${SSH_PASSWORD}" ]]; then
      if [[ ! -t 0 ]]; then
        echo "SSH_PASSWORD is required when password mode has no interactive terminal" >&2
        exit 1
      fi
      read -r -s -p "SSH password for ${SSH_USER}: " SSH_PASSWORD
      echo
    fi
    ;;
  *)
    echo "SSH_AUTH_MODE must be key or password" >&2
    exit 1
    ;;
esac

if [[ -z "${SUDO_AUTH_MODE}" ]]; then
  if [[ -n "${SUDO_PASSWORD}" || "${SSH_AUTH_MODE}" == "password" ]]; then
    SUDO_AUTH_MODE="password"
  else
    SUDO_AUTH_MODE="passwordless"
  fi
fi

case "${SUDO_AUTH_MODE}" in
  passwordless)
    SUDO_PASSWORD=""
    ;;
  password)
    if [[ -z "${SUDO_PASSWORD}" && "${SSH_AUTH_MODE}" == "password" ]]; then
      SUDO_PASSWORD="${SSH_PASSWORD}"
    fi
    if [[ -z "${SUDO_PASSWORD}" ]]; then
      if [[ ! -t 0 ]]; then
        echo "SUDO_PASSWORD is required when sudo password mode has no interactive terminal" >&2
        exit 1
      fi
      read -r -s -p "sudo password for ${SSH_USER}: " SUDO_PASSWORD
      echo
    fi
    ;;
  *)
    echo "SUDO_AUTH_MODE must be passwordless or password" >&2
    exit 1
    ;;
esac

for command_name in ssh scp; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "${command_name} is required" >&2
    exit 1
  fi
done

if (( $# > 0 )); then
  echo "Switch IPs must be provided with the HOSTS environment variable" >&2
  usage >&2
  exit 1
fi

TARGET_HOSTS=()
if [[ -n "${HOSTS_CSV}" ]]; then
  IFS=',' read -r -a configured_hosts <<<"${HOSTS_CSV}"
  for host in "${configured_hosts[@]}"; do
    host="${host#"${host%%[![:space:]]*}"}"
    host="${host%"${host##*[![:space:]]}"}"
    if [[ -n "${host}" ]]; then
      TARGET_HOSTS+=("${host}")
    fi
  done
fi

if (( ${#TARGET_HOSTS[@]} == 0 )); then
  echo "At least one IP is required in the HOSTS environment variable" >&2
  usage >&2
  exit 1
fi

case "${LLDP_COLLECTION_MODE}" in
  socket)
    collection_preflight="\
if [ ! -S '${LLDP_SOCKET_PATH}' ]; then echo 'LLDP socket not found: ${LLDP_SOCKET_PATH}' >&2; exit 1; fi && "
    docker_collection_options="\
-p '${GRPC_PORT}:${GRPC_PORT}' \
-e UNIFABRIC_SWITCH_AGENT_LISTEN_ADDRESS=':${GRPC_PORT}' \
-e UNIFABRIC_SWITCH_AGENT_LLDP_COLLECTION_MODE=socket \
-e UNIFABRIC_SWITCH_AGENT_LLDP_SOCKET_PATH='${LLDP_SOCKET_PATH}' \
-v '${LLDP_SOCKET_PATH}:${LLDP_SOCKET_PATH}'"
    ;;
  hostProc)
    collection_preflight=""
    docker_collection_options="\
--network host \
--uts host \
--privileged \
-e UNIFABRIC_SWITCH_AGENT_LISTEN_ADDRESS=':${GRPC_PORT}' \
-e UNIFABRIC_SWITCH_AGENT_LLDP_COLLECTION_MODE=hostProc \
-v /proc:/host/proc:ro"
    ;;
  *)
    echo "LLDP_COLLECTION_MODE must be socket or hostProc" >&2
    exit 1
    ;;
esac

if [[ ! "${GRPC_PORT}" =~ ^[1-9][0-9]{0,4}$ ]] || (( 10#${GRPC_PORT} > 65535 )); then
  echo "GRPC_PORT must be an integer between 1 and 65535" >&2
  exit 1
fi

CERT_FILES=(peer.crt tls.crt tls.key)
SOURCE_FILES=()
for file in "${CERT_FILES[@]}"; do
  path="${CERT_SOURCE_DIR}/${file}"
  if [[ ! -f "${path}" ]]; then
    echo "Missing certificate file: ${path}" >&2
    exit 1
  fi
  SOURCE_FILES+=("${path}")
done

for value in \
  "${REMOTE_UPLOAD_DIR}" \
  "${REMOTE_CERT_DIR}" \
  "${REMOTE_CERT_OWNER}" \
  "${REMOTE_CERT_GROUP}" \
  "${CONTAINER_NAME}" \
  "${SWITCH_AGENT_IMAGE}" \
  "${LLDP_SOCKET_PATH}"; do
  if [[ "${value}" == *"'"* || "${value}" == *$'\n'* ]]; then
    echo "Remote configuration values must not contain single quotes or newlines" >&2
    exit 1
  fi
done

SSH_OPTIONS=(
  -p "${SSH_PORT}"
  -o ConnectTimeout=10
  -o StrictHostKeyChecking=accept-new
)
SCP_OPTIONS=(
  -P "${SSH_PORT}"
  -o ConnectTimeout=10
  -o StrictHostKeyChecking=accept-new
)
if [[ "${SSH_AUTH_MODE}" == "password" ]]; then
  SSH_OPTIONS+=(
    -o PreferredAuthentications=password,keyboard-interactive
    -o PubkeyAuthentication=no
  )
  SCP_OPTIONS+=(
    -o PreferredAuthentications=password,keyboard-interactive
    -o PubkeyAuthentication=no
  )
fi

run_ssh() {
  if [[ "${SSH_AUTH_MODE}" == "password" ]]; then
    SSHPASS="${SSH_PASSWORD}" sshpass -e ssh "${SSH_OPTIONS[@]}" "$@"
  else
    ssh "${SSH_OPTIONS[@]}" "$@"
  fi
}

run_scp() {
  if [[ "${SSH_AUTH_MODE}" == "password" ]]; then
    SSHPASS="${SSH_PASSWORD}" sshpass -e scp "${SCP_OPTIONS[@]}" "$@"
  else
    scp "${SCP_OPTIONS[@]}" "$@"
  fi
}

FAILED_HOSTS=()
for host in "${TARGET_HOSTS[@]}"; do
  if [[ "${host}" == *[[:space:]\'\"]* ]]; then
    echo "Invalid host value: ${host}" >&2
    FAILED_HOSTS+=("${host}")
    continue
  fi

  target="${SSH_USER}@${host}"
  echo "Deploying switch-agent to ${target}"

  if ! run_ssh "${target}" "mkdir -p '${REMOTE_UPLOAD_DIR}'"; then
    echo "Failed to prepare upload directory: ${host}" >&2
    FAILED_HOSTS+=("${host}")
    continue
  fi

  if ! run_scp "${SOURCE_FILES[@]}" "${target}:${REMOTE_UPLOAD_DIR}/"; then
    echo "Failed to copy certificates: ${host}" >&2
    FAILED_HOSTS+=("${host}")
    continue
  fi

  remote_command="\
install -d -o root -g root -m 0755 '${REMOTE_CERT_DIR}' && \
install -o '${REMOTE_CERT_OWNER}' -g '${REMOTE_CERT_GROUP}' -m 0644 '${REMOTE_UPLOAD_DIR}/peer.crt' '${REMOTE_CERT_DIR}/peer.crt' && \
install -o '${REMOTE_CERT_OWNER}' -g '${REMOTE_CERT_GROUP}' -m 0644 '${REMOTE_UPLOAD_DIR}/tls.crt' '${REMOTE_CERT_DIR}/tls.crt' && \
install -o '${REMOTE_CERT_OWNER}' -g '${REMOTE_CERT_GROUP}' -m 0600 '${REMOTE_UPLOAD_DIR}/tls.key' '${REMOTE_CERT_DIR}/tls.key' && \
rm -f '${REMOTE_UPLOAD_DIR}/peer.crt' '${REMOTE_UPLOAD_DIR}/tls.crt' '${REMOTE_UPLOAD_DIR}/tls.key' && \
${collection_preflight}\
if ! docker image inspect '${SWITCH_AGENT_IMAGE}' >/dev/null 2>&1; then echo 'Local image not found: ${SWITCH_AGENT_IMAGE}' >&2; exit 1; fi && \
if docker container inspect '${CONTAINER_NAME}' >/dev/null 2>&1; then docker rm -f '${CONTAINER_NAME}'; fi && \
docker run -d \
  --name '${CONTAINER_NAME}' \
  --restart unless-stopped \
  ${docker_collection_options} \
  -e UNIFABRIC_SWITCH_AGENT_SWITCH_NAME=\"\$(hostname)\" \
  -v '${REMOTE_CERT_DIR}':/etc/unifabric/switch-mtls:ro \
  '${SWITCH_AGENT_IMAGE}' \
  /usr/bin/unifabric/switch-agent"
  printf -v remote_command_quoted '%q' "${remote_command}"

  if [[ -n "${SUDO_PASSWORD}" ]]; then
    if printf '%s\n' "${SUDO_PASSWORD}" |
      run_ssh "${target}" "sudo -S -p '' sh -c ${remote_command_quoted}"; then
      echo "Completed: ${host}"
    else
      echo "Failed to install or start switch-agent: ${host}" >&2
      FAILED_HOSTS+=("${host}")
    fi
  elif run_ssh "${target}" "sudo -n sh -c ${remote_command_quoted}"; then
    echo "Completed: ${host}"
  else
    echo "Failed to install or start switch-agent: ${host}" >&2
    FAILED_HOSTS+=("${host}")
  fi
done

if (( ${#FAILED_HOSTS[@]} > 0 )); then
  echo "Failed hosts: ${FAILED_HOSTS[*]}" >&2
  exit 1
fi

echo "switch-agent deployed on ${#TARGET_HOSTS[@]} switch(es) with ${LLDP_COLLECTION_MODE} LLDP collection."
