#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HELM_CHART="${ROOT_DIR}/deploy/helm/go-xietong"
MODULES=(project-common project-user project-api project-project)

echo "[1/4] Go tests, vet and race tests"
for module in "${MODULES[@]}"; do
  echo "checking ${module}"
  (
    cd "${ROOT_DIR}/${module}"
    go test ./...
    go vet ./...
    go test -race ./...
  )
done

echo "[2/4] Frontend lint and build"
(
  cd "${ROOT_DIR}/frontend"
  if [[ "${SKIP_NPM_INSTALL:-0}" != "1" ]]; then
    npm ci --legacy-peer-deps
  fi
  npm run lint
  NODE_OPTIONS="${NODE_OPTIONS:---openssl-legacy-provider}" npm run build
)

if ! command -v helm >/dev/null 2>&1; then
  echo "helm is required for chart verification" >&2
  exit 1
fi

echo "[3/4] Helm lint and render"
helm lint "${HELM_CHART}" \
  --set secrets.mysqlRootPassword=verify-root-password \
  --set secrets.mysqlPassword=verify-password \
  --set secrets.jwtAccessSecret=verify-access-secret \
  --set secrets.jwtRefreshSecret=verify-refresh-secret
helm template go-xietong "${HELM_CHART}" \
  --namespace go-xietong \
  --set secrets.mysqlRootPassword=verify-root-password \
  --set secrets.mysqlPassword=verify-password \
  --set secrets.jwtAccessSecret=verify-access-secret \
  --set secrets.jwtRefreshSecret=verify-refresh-secret \
  >/dev/null

echo "[4/4] Git diff check"
git -C "${ROOT_DIR}" diff --check

echo "Verification passed. Run scripts/smoke.sh after Docker Compose is running for the full service chain."
