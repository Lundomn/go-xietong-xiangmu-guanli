#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
K6_IMAGE="${K6_IMAGE:-grafana/k6:latest}"

# Docker Desktop uses host.docker.internal to reach the host. Override
# BASE_URL when k6 runs on Linux or against a remote environment.
BASE_URL_VALUE="${BASE_URL:-http://host.docker.internal:8088}"
RATE_VALUE="${RATE:-20}"
DURATION_VALUE="${DURATION:-30s}"

docker run --rm \
  --add-host=host.docker.internal:host-gateway \
  -e "BASE_URL=${BASE_URL_VALUE}" \
  -e "ACCESS_TOKEN=${ACCESS_TOKEN:-}" \
  -e "PROJECT_CODE=${PROJECT_CODE:-}" \
  -e "RATE=${RATE_VALUE}" \
  -e "DURATION=${DURATION_VALUE}" \
  --mount "type=bind,source=${ROOT_DIR}/loadtest,target=/test,readonly" \
  "${K6_IMAGE}" run /test/k6.js
