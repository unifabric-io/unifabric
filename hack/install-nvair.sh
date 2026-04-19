#!/usr/bin/env bash

set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

if command -v nvair >/dev/null 2>&1; then
  echo "nvair already installed at: $(command -v nvair)"
  exit 0
fi

OS="$(uname -s)"
if [[ "${OS}" != "Linux" ]]; then
  echo "Automatic nvair installation is only supported on Linux." >&2
  echo "Detected OS: ${OS}" >&2
  echo "Please install nvair manually by following:" >&2
  echo "https://github.com/unifabric-io/nvair-cli" >&2
  exit 1
fi

ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64)
    PLATFORM_ARCH="amd64"
    ;;
  aarch64)
    PLATFORM_ARCH="arm64"
    ;;
  *)
    echo "Unsupported architecture: ${ARCH}" >&2
    exit 1
    ;;
esac

VERSION="${VERSION:-$(curl -fsSL https://api.github.com/repos/unifabric-io/nvair-cli/releases/latest | awk -F'"' '/"tag_name"[[:space:]]*:/ { tag=$4 } END { print tag }')}"
if [[ -z "${VERSION}" ]]; then
  echo "Failed to resolve nvair version." >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

TARBALL_URL="https://github.com/unifabric-io/nvair-cli/releases/download/${VERSION}/nvair_${VERSION}_linux_${PLATFORM_ARCH}.tar.gz"
TARBALL_PATH="${TMP_DIR}/nvair.tar.gz"

curl -fL -o "${TARBALL_PATH}" "${TARBALL_URL}"
tar -xzf "${TARBALL_PATH}" -C "${TMP_DIR}"
chmod +x "${TMP_DIR}/nvair"

if [[ -w "${INSTALL_DIR}" ]]; then
  mv "${TMP_DIR}/nvair" "${INSTALL_DIR}/nvair"
elif command -v sudo >/dev/null 2>&1; then
  sudo mv "${TMP_DIR}/nvair" "${INSTALL_DIR}/nvair"
else
  echo "No permission to write ${INSTALL_DIR}, and sudo is unavailable." >&2
  exit 1
fi

echo "nvair installed to ${INSTALL_DIR}/nvair (version: ${VERSION})"
