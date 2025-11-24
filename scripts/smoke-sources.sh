#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later 
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
TOKEN="${FLWD_TOKEN:-dev-token}"
SOURCE_NAME="${SOURCE_NAME:-demo-local}"
SOURCE_TYPE="${SOURCE_TYPE:-local}"
SOURCE_URL="${SOURCE_URL:-}"
GIT_REF="${GIT_REF:-main}"
JOB_ID="${JOB_ID:-demo}"
REF_PATH="${REF_PATH:-demo}"
RUN_MESSAGE="${RUN_MESSAGE:-hello from sources smoke}"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for this smoke test (set PATH accordingly)." >&2
  exit 2
fi

AUTH_HEADER=(-H "Authorization: Bearer ${TOKEN}")

log() {
  printf '==> %s\n' "$*"
}

curl_json() {
  local method=$1
  local url=$2
  shift 2

  local response
  response="$(curl -sS "${AUTH_HEADER[@]}" -H "Content-Type: application/json" -X "${method}" "${url}" "$@" -w $'\n%{http_code}')" || {
    echo "curl request failed" >&2
    exit 1
  }

  local body code
  body="$(printf '%s\n' "${response}" | sed '$d')"
  code="$(printf '%s\n' "${response}" | tail -n1)"

  case "${code}" in
    200|201)
      printf '%s\n' "${body}"
      ;;
    *)
      printf 'HTTP %s\n%s\n' "${code}" "${body}" >&2
      exit 1
      ;;
  esac
}

curl_text() {
  curl -sS "${AUTH_HEADER[@]}" "$@"
}

log "Checking server readiness (${BASE_URL}/healthz)"
curl -sSf "${BASE_URL}/healthz" >/dev/null || {
  echo "Server health check failed. Ensure flwd :serve is running on ${BASE_URL}." >&2
  exit 1
}

case "${SOURCE_TYPE}" in
  local)
    log "Registering local source '${SOURCE_NAME}' -> ${REF_PATH}"
    register_payload="$(jq -n --arg name "${SOURCE_NAME}" --arg ref "${REF_PATH}" '{type:"local",name:$name,ref:$ref}')"
    ;;
  git)
    if [[ -z "${SOURCE_URL}" ]]; then
      echo "SOURCE_URL is required when SOURCE_TYPE=git" >&2
      exit 2
    fi
    log "Registering git source '${SOURCE_NAME}' -> ${SOURCE_URL} (ref ${GIT_REF})"
    register_payload="$(jq -n --arg name "${SOURCE_NAME}" --arg url "${SOURCE_URL}" --arg ref "${GIT_REF}" '{type:"git",name:$name,url:$url,ref:$ref}')"
    ;;
  *)
    echo "unsupported SOURCE_TYPE: ${SOURCE_TYPE}" >&2
    exit 2
    ;;
esac
curl_json POST "${BASE_URL}/sources" --data "${register_payload}" | jq .

log "Listing jobs that belong to registered source"
curl_text -H "Accept: application/json" "${BASE_URL}/jobs" \
  | jq --arg name "${SOURCE_NAME}" '[.[] | select(.source.name == $name)]'

log "Planning job '${JOB_ID}' via source '${SOURCE_NAME}'"
plan_payload="$(jq -n \
  --arg job "${JOB_ID}" \
  --arg msg "${RUN_MESSAGE}" \
  --arg src "${SOURCE_NAME}" \
  '{job_id:$job,args:{name:$msg},source:{name:$src}}')"
curl_json POST "${BASE_URL}/plans" --data "${plan_payload}" | jq .

log "Starting run for '${JOB_ID}' (source ${SOURCE_NAME})"
run_payload="${plan_payload}"
run_response="$(curl_json POST "${BASE_URL}/runs" --data "${run_payload}")"
printf '%s\n' "${run_response}" | jq .
run_id="$(printf '%s\n' "${run_response}" | jq -r '.id')"

if [[ -z "${run_id}" || "${run_id}" == "null" ]]; then
  echo "Run ID missing from response" >&2
  exit 1
fi

log "Polling run status for ${run_id}"
attempt=0
while true; do
  run_status="$(curl_text "${BASE_URL}/runs/${run_id}" | jq -r '.status')"
  printf '  status: %s\n' "${run_status}"
  case "${run_status}" in
    succeeded|failed|canceled)
      break
      ;;
    *)
      sleep 1
      ;;
  esac
  attempt=$((attempt + 1))
  if (( attempt > 30 )); then
    echo "Run did not complete within timeout window" >&2
    exit 1
  fi
done

log "Fetching run provenance and result"
curl_text "${BASE_URL}/runs/${run_id}" | jq '{id, status, executor, provenance}'

log "Fetching recorded events (NDJSON)"
# NDJSON export is optional and gated; skip until the extension is enabled.

log "Smoke test complete. Run artifacts are available under .flwd/runs/${run_id}/"
