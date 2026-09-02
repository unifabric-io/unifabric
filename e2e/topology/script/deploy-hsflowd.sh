#!/usr/bin/env bash

set -euo pipefail

HSFLOWD_IMAGE="${HSFLOWD_IMAGE:-}"
SIMULATION_NAME="${SIMULATION_NAME:-unifable-e2e-topology}"
SWITCH_SUDO_PASSWORD="${SWITCH_SUDO_PASSWORD:-Dangerous1#}"
DEFAULT_SWITCHES=(
  switch-gpu-leaf1
  switch-gpu-leaf2
  switch-gpu-leaf3
  switch-gpu-leaf4
  switch-gpu-spine1
  switch-scaleup-leaf1
  switch-scaleup-leaf2
  switch-storage-leaf1
)
TARGET_SWITCHES=()

usage() {
  cat <<'EOF'
Usage:
  e2e/topology/script/deploy-hsflowd.sh

Required environment variables:
  HSFLOWD_IMAGE                Full hsflowd image to run on switches.

Optional environment variables:
  SIMULATION_NAME              Simulation name. Default: unifable-e2e-topology
  SWITCH_NAMES                 Optional comma/space-separated switch names.
  SWITCH_SUDO_PASSWORD         Switch sudo password for Cumulus VX images.

If SWITCH_NAMES is empty, the script uses its built-in default switch list for
the e2e topology.
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
  echo "[hsflowd-deploy] $*"
}

resolve_target_switches() {
  if [[ -n "${SWITCH_NAMES:-}" ]]; then
    local normalized
    normalized="${SWITCH_NAMES//,/ }"
    read -r -a TARGET_SWITCHES <<< "${normalized}"
    return
  fi

  TARGET_SWITCHES=("${DEFAULT_SWITCHES[@]}")
}

switch_sudo_function() {
  local password_quoted
  printf -v password_quoted "%q" "${SWITCH_SUDO_PASSWORD}"

  cat <<EOF
switch_sudo() {
  sudo -S -p '' -- "\$@" <<< ${password_quoted}
}
EOF
}

require_command nvair

require_non_empty HSFLOWD_IMAGE
resolve_target_switches

if (( ${#TARGET_SWITCHES[@]} == 0 )); then
  echo "No target switches specified." >&2
  usage >&2
  exit 2
fi

SIM_ARGS=(-s "${SIMULATION_NAME}")

log "Target switches: ${TARGET_SWITCHES[*]}"

for switch_name in "${TARGET_SWITCHES[@]}"; do
  log "Deploying hsflowd on ${switch_name}..."

  nvair exec "${switch_name}" "${SIM_ARGS[@]}" -- bash -c "set -euo pipefail
export PATH=\"/usr/sbin:/sbin:\${PATH}\"
$(switch_sudo_function)

if ! command -v docker >/dev/null 2>&1; then
  echo 'docker not found on switch' >&2
  exit 1
fi

if ! command -v tc >/dev/null 2>&1; then
  echo 'tc not found on switch' >&2
  exit 1
fi

for module in psample act_sample cls_matchall sch_ingress; do
  switch_sudo modprobe \"\${module}\"
done

swp_interfaces=()
for netdev_path in /sys/class/net/swp*; do
  [[ -e \"\${netdev_path}\" ]] || continue
  swp_interfaces+=(\"\$(basename \"\${netdev_path}\")\")
done

if (( \${#swp_interfaces[@]} == 0 )); then
  echo 'No swp* interfaces found on switch' >&2
  exit 1
fi

mapfile -t swp_interfaces < <(printf '%s\n' \"\${swp_interfaces[@]}\" | sort -V)
printf 'Configuring psample tc ingress on interfaces: %s\n' \"\${swp_interfaces[*]}\"

for dev in \"\${swp_interfaces[@]}\"; do
  switch_sudo tc qdisc replace dev \"\${dev}\" handle ffff: ingress
  switch_sudo tc filter del dev \"\${dev}\" parent ffff: protocol all pref 1 matchall >/dev/null 2>&1 || true
  switch_sudo tc filter add dev \"\${dev}\" parent ffff: protocol all pref 1 matchall \\
    action sample rate 1 group 1 trunc 128
done

switch_sudo docker pull '$(printf '%q' "${HSFLOWD_IMAGE}")'

switch_sudo docker rm -f unifabric-hsflowd >/dev/null 2>&1 || true

set +e
container_output=\$(switch_sudo docker run -d \\
  --name unifabric-hsflowd \\
  --restart unless-stopped \\
  --network host \\
  --pid host \\
  --privileged \\
  '$(printf '%q' "${HSFLOWD_IMAGE}")' \\
  -d -P -c /var/log/hsflowd.crash 2>&1)
container_status=\$?
set -e
if [[ \"\${container_status}\" != \"0\" ]]; then
  printf 'docker run failed with status %s:\n%s\n' \"\${container_status}\" \"\${container_output}\" >&2
  switch_sudo docker ps -a --filter name=unifabric-hsflowd || true
  exit \"\${container_status}\"
fi
printf 'Started hsflowd container: %s\n' \"\${container_output}\"

container_status_line=\$(switch_sudo docker inspect \\
  --format 'container={{.Name}}|state={{.State.Status}}|running={{.State.Running}}|image={{.Config.Image}}' \\
  unifabric-hsflowd)
if [[ -z \"\${container_status_line}\" ]]; then
  echo 'hsflowd container was not found after docker run' >&2
  exit 1
fi
printf '%s\n' \"\${container_status_line}\"

switch_sudo docker ps \\
  --filter name=unifabric-hsflowd \\
  --format '{{.Names}}' \\
  | grep -qx unifabric-hsflowd"
done

log "hsflowd deployed successfully for ${#TARGET_SWITCHES[@]} switches."
