#!/usr/bin/env bash

set -Eeuo pipefail

BASE_URL="${SMOKE_BASE_URL:-http://127.0.0.1:8088}"
BASE_URL="${BASE_URL%/}"
FRONTEND_URL="${SMOKE_FRONTEND_URL:-}"
CURL_TIMEOUT="${SMOKE_CURL_TIMEOUT:-10}"

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT
trap 'echo "Smoke Test failed near line ${LINENO}" >&2' ERR

post_form() {
  local endpoint="$1"
  shift
  curl -sS --retry 3 --retry-all-errors --max-time "${CURL_TIMEOUT}" \
    -X POST "${BASE_URL}${endpoint}" "$@"
}

assert_success() {
  local response="$1"
  local step="$2"
  if ! jq -e '.code == 200' >/dev/null <<<"${response}"; then
    echo "${step} failed:" >&2
    echo "${response}" | jq . >&2 || echo "${response}" >&2
    exit 1
  fi
}

extract_required() {
  local response="$1"
  local expression="$2"
  local name="$3"
  local value
  value="$(jq -r "${expression} // empty" <<<"${response}")"
  if [[ -z "${value}" || "${value}" == "null" ]]; then
    echo "${name} is missing in response:" >&2
    echo "${response}" | jq . >&2 || echo "${response}" >&2
    exit 1
  fi
  printf '%s' "${value}"
}

echo "[1/11] API health"
health_response="$(curl -fsS --retry 3 --retry-all-errors --max-time "${CURL_TIMEOUT}" "${BASE_URL}/health")"
jq -e '.status == "ok"' >/dev/null <<<"${health_response}"

if [[ -n "${FRONTEND_URL}" ]]; then
  echo "[2/11] Frontend health"
  frontend_health="$(curl -fsS --retry 3 --retry-all-errors --max-time "${CURL_TIMEOUT}" "${FRONTEND_URL%/}/health")"
  jq -e '.status == "ok"' >/dev/null <<<"${frontend_health}"
else
  echo "[2/11] Frontend health skipped (set SMOKE_FRONTEND_URL to enable)"
fi

seed="${SMOKE_SEED:-$(date +%s)}"
mobile="${SMOKE_MOBILE:-138$(printf '%08d' "$((seed % 100000000))")}"
account="${SMOKE_ACCOUNT:-smoke${seed}}"
email="${SMOKE_EMAIL:-${account}@example.com}"
password="${SMOKE_PASSWORD:-SmokePass!123}"

echo "[3/11] Captcha and registration"
captcha_response="$(post_form /project/login/getCaptcha --data-urlencode "mobile=${mobile}")"
assert_success "${captcha_response}" "captcha"
captcha="$(extract_required "${captcha_response}" '.data' "captcha")"

register_response="$(post_form /project/login/register \
  --data-urlencode "email=${email}" \
  --data-urlencode "name=${account}" \
  --data-urlencode "password=${password}" \
  --data-urlencode "password2=${password}" \
  --data-urlencode "mobile=${mobile}" \
  --data-urlencode "captcha=${captcha}")"
assert_success "${register_response}" "registration"

echo "[4/11] Login"
login_response="$(post_form /project/login \
  --data-urlencode "account=${account}" \
  --data-urlencode "password=${password}")"
assert_success "${login_response}" "login"
access_token="$(extract_required "${login_response}" '.data.tokenList.accessToken' "access token")"
auth_header=( -H "Authorization: Bearer ${access_token}" )

echo "[5/11] Project template and project creation"
template_response="$(post_form /project/project_template "${auth_header[@]}" \
  --data-urlencode "viewType=1" \
  --data-urlencode "page=1" \
  --data-urlencode "pageSize=10")"
assert_success "${template_response}" "project template"
template_code="$(extract_required "${template_response}" '.data.list[0].code' "template code")"

project_response="$(post_form /project/project/save "${auth_header[@]}" \
  --data-urlencode "name=Smoke ${seed}" \
  --data-urlencode "templateCode=${template_code}" \
  --data-urlencode "description=automated end-to-end smoke test")"
assert_success "${project_response}" "project creation"
project_code="$(extract_required "${project_response}" '.data.code' "project code")"

echo "[6/11] Task stage and task creation"
stages_response="$(post_form /project/task_stages "${auth_header[@]}" \
  --data-urlencode "projectCode=${project_code}" \
  --data-urlencode "page=1" \
  --data-urlencode "pageSize=10")"
assert_success "${stages_response}" "task stages"
stage_code="$(extract_required "${stages_response}" '.data.list[0].code' "stage code")"

task_response="$(post_form /project/task/save "${auth_header[@]}" \
  --data-urlencode "project_code=${project_code}" \
  --data-urlencode "stage_code=${stage_code}" \
  --data-urlencode "name=Smoke task ${seed}")"
assert_success "${task_response}" "task creation"
task_code="$(extract_required "${task_response}" '.data.code' "task code")"

echo "[7/11] Single-part upload and authenticated download"
upload_file="${tmp_dir}/smoke.txt"
download_file="${tmp_dir}/downloaded.txt"
printf 'go-xietong smoke upload %s\n' "${seed}" >"${upload_file}"
upload_size="$(wc -c <"${upload_file}" | tr -d '[:space:]')"
upload_response="$(post_form /project/file/uploadFiles "${auth_header[@]}" \
  --form-string "projectCode=${project_code}" \
  --form-string "taskCode=${task_code}" \
  --form-string "totalChunks=1" \
  --form-string "chunkNumber=1" \
  --form-string "totalSize=${upload_size}" \
  --form-string "filename=smoke.txt" \
  -F "file=@${upload_file};type=text/plain")"
assert_success "${upload_response}" "file upload"
file_url="$(extract_required "${upload_response}" '.data.url' "uploaded file URL")"
curl -fsS --retry 3 --retry-all-errors --max-time "${CURL_TIMEOUT}" \
  "${BASE_URL}${file_url}" "${auth_header[@]}" -o "${download_file}"
cmp -s "${upload_file}" "${download_file}"
unauthorized_download="$(curl -sS --retry 3 --retry-all-errors --max-time "${CURL_TIMEOUT}" \
  "${BASE_URL}${file_url}")"
if jq -e '.code == 200' >/dev/null <<<"${unauthorized_download}"; then
  echo "unauthenticated file download was accepted" >&2
  exit 1
fi

echo "[8/11] Out-of-order chunk upload"
chunk_one="${tmp_dir}/chunk-one.txt"
chunk_two="${tmp_dir}/chunk-two.txt"
chunked_download="${tmp_dir}/chunked-download.txt"
printf 'chunk-one-' >"${chunk_one}"
printf 'chunk-two-%s\n' "${seed}" >"${chunk_two}"
chunk_one_size="$(wc -c <"${chunk_one}" | tr -d '[:space:]')"
chunk_two_size="$(wc -c <"${chunk_two}" | tr -d '[:space:]')"
chunked_size="$((chunk_one_size + chunk_two_size))"
chunk_identifier="smoke-${seed}"

partial_response="$(post_form /project/file/uploadFiles "${auth_header[@]}" \
  --form-string "projectCode=${project_code}" \
  --form-string "taskCode=${task_code}" \
  --form-string "totalChunks=2" \
  --form-string "chunkNumber=2" \
  --form-string "chunkSize=${chunk_two_size}" \
  --form-string "currentChunkSize=${chunk_two_size}" \
  --form-string "totalSize=${chunked_size}" \
  --form-string "identifier=${chunk_identifier}" \
  --form-string "filename=chunked.txt" \
  -F "file=@${chunk_two};type=text/plain")"
assert_success "${partial_response}" "first out-of-order chunk"

chunked_response="$(post_form /project/file/uploadFiles "${auth_header[@]}" \
  --form-string "projectCode=${project_code}" \
  --form-string "taskCode=${task_code}" \
  --form-string "totalChunks=2" \
  --form-string "chunkNumber=1" \
  --form-string "chunkSize=${chunk_one_size}" \
  --form-string "currentChunkSize=${chunk_one_size}" \
  --form-string "totalSize=${chunked_size}" \
  --form-string "identifier=${chunk_identifier}" \
  --form-string "filename=chunked.txt" \
  -F "file=@${chunk_one};type=text/plain")"
assert_success "${chunked_response}" "second out-of-order chunk"
chunked_url="$(extract_required "${chunked_response}" '.data.url' "chunked file URL")"
curl -fsS --retry 3 --retry-all-errors --max-time "${CURL_TIMEOUT}" \
  "${BASE_URL}${chunked_url}" "${auth_header[@]}" -o "${chunked_download}"
cat "${chunk_one}" "${chunk_two}" >"${tmp_dir}/chunked-expected.txt"
cmp -s "${tmp_dir}/chunked-expected.txt" "${chunked_download}"

echo "[9/11] Project health radar"
health_radar_response="$(post_form /project/project/health "${auth_header[@]}" \
  --data-urlencode "projectCode=${project_code}")"
assert_success "${health_radar_response}" "project health radar"
jq -e '.data.score >= 0 and .data.score <= 100 and (.data.method == "rules")' >/dev/null <<<"${health_radar_response}"

echo "[10/11] Weekly report"
report_response="$(post_form /project/report/weekly "${auth_header[@]}" \
  --data-urlencode "projectCode=${project_code}" \
  --data-urlencode "focus=验证交付风险和下一步计划")"
assert_success "${report_response}" "weekly report"
jq -e '.data.markdown != null and (.data.generated_by == "local" or .data.generated_by == "local-fallback" or .data.generated_by == "ai")' >/dev/null <<<"${report_response}"

echo "[11/11] Authenticated project list"
project_list_response="$(post_form /project/project "${auth_header[@]}" \
  --data-urlencode "page=1" \
  --data-urlencode "pageSize=10")"
assert_success "${project_list_response}" "project list"
jq -e '.data.list != null and .data.total >= 1' >/dev/null <<<"${project_list_response}"

echo "Smoke Test passed: registration, login, project, task, upload/download, health radar and weekly report"
