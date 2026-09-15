#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GOOS_VALUE="${GOOS:-linux}"
GOARCH_VALUE="${GOARCH:-$(go env GOARCH)}"

build_service() {
  local module="$1"
  local binary="$2"
  echo "building ${module} (${GOOS_VALUE}/${GOARCH_VALUE})"
  (
    cd "${ROOT_DIR}/${module}"
    GOOS="${GOOS_VALUE}" GOARCH="${GOARCH_VALUE}" CGO_ENABLED=0 \
      go build -trimpath -ldflags='-s -w' -o "target/${binary}" .
  )
}

build_service project-user project-user
build_service project-project project-project
build_service project-api project-api

echo "build complete"
