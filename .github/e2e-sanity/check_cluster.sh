#!/usr/bin/env bash
# Polls the root orchestrator until exactly one active cluster with the
# expected name reports the expected number of active worker nodes.
#
# Environment:
#   EXPECTED_CLUSTER_NAME  required
#   EXPECTED_NODES         required
#   ROOT_API_URL           default http://localhost:10000
#   TIMEOUT_SECONDS        default 300
#   POLL_INTERVAL          default 5
set -euo pipefail

: "${EXPECTED_CLUSTER_NAME:?EXPECTED_CLUSTER_NAME is required}"
: "${EXPECTED_NODES:?EXPECTED_NODES is required}"
ROOT_API_URL="${ROOT_API_URL:-http://localhost:10000}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-300}"
POLL_INTERVAL="${POLL_INTERVAL:-5}"

url="${ROOT_API_URL%/}/api/clusters/active"
last_body=""

# The endpoint has returned both a plain JSON array and a JSON string that
# itself contains the array, so normalise before matching.
# shellcheck disable=SC2016  # $name/$nodes are jq variables, not shell ones
match_filter='
  (if type == "string" then fromjson else . end)
  | [.[] | select(.cluster_name == $name)]
  | length == 1 and (.[0].active_nodes // 0) == $nodes
'

deadline=$((SECONDS + TIMEOUT_SECONDS))
echo "Waiting up to ${TIMEOUT_SECONDS}s for cluster '${EXPECTED_CLUSTER_NAME}' with ${EXPECTED_NODES} active node(s) at ${url}"

while true; do
  if body=$(curl -fsS --max-time 10 "$url" 2>&1); then
    last_body="$body"
    result=$(jq -r --arg name "$EXPECTED_CLUSTER_NAME" --argjson nodes "$EXPECTED_NODES" \
      "$match_filter" <<<"$body" 2>/dev/null || echo "unparsable")
    if [[ "$result" == "true" ]]; then
      echo "OK: cluster '${EXPECTED_CLUSTER_NAME}' is active with ${EXPECTED_NODES} node(s)"
      exit 0
    fi
    echo "[${SECONDS}s] not ready yet (${result}): $(tr -d '\n' <<<"$body" | cut -c1-300)"
  else
    last_body="$body"
    echo "[${SECONDS}s] root API not reachable yet: ${body}"
  fi

  if ((SECONDS >= deadline)); then
    echo "Timed out after ${TIMEOUT_SECONDS}s. Last response:" >&2
    echo "$last_body" >&2
    exit 1
  fi
  sleep "$POLL_INTERVAL"
done
