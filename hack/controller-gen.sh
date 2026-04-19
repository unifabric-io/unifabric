#!/bin/bash

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
CONTROLLER_TOOLS_VERSION=${CONTROLLER_TOOLS_VERSION:-v0.20.1}

cd "${ROOT_DIR}"
GO111MODULE=on GOCACHE="${GOCACHE:-/tmp/unifabric-go-build}" \
  go run "sigs.k8s.io/controller-tools/cmd/controller-gen@${CONTROLLER_TOOLS_VERSION}" "$@"
