#!/usr/bin/env bash
# Deploys, scales and removes a busybox service through the root API, then
# makes sure a service that fits no worker gets rejected.
#
# Instance state is compared at root and in containerd. Root only learns about
# the worker through the cluster, so it can look fine while the worker is not.
# That also means this has to run on the worker host, since it calls `ctr`.
#
# Environment:
#   ROOT_API_URL      default http://localhost:10000
#   TIMEOUT_SECONDS   default 300, applied to each wait separately
#   POLL_INTERVAL     default 5
set -euo pipefail

ROOT_API_URL="${ROOT_API_URL:-http://localhost:10000}"
ROOT_API_URL="${ROOT_API_URL%/}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-300}"
POLL_INTERVAL="${POLL_INTERVAL:-5}"

# Pinned by digest so a retag upstream can't change what CI runs.
IMAGE="docker.io/library/busybox:1.36@sha256:73aaf090f3d85aa34ee199857f03fa3a95c8ede2ffd4cc2cdb5b94e566b11662"
# Must match the containerd namespace hardcoded in the NodeEngine.
CTR_NAMESPACE=oakestra
APP_NS=ci
SERVICE_NAME=sleeper
SERVICE_NS=ci

# Taken from the library in this checkout so a new rejection reason doesn't
# need an edit here. The library is stdlib only, so no pip install needed.
UTILS_LIBRARY="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/libraries/oakestra_utils_library"
NEGATIVE_SCHEDULING_STATUSES=$(python3 -c '
import sys
sys.path.insert(0, sys.argv[1])
from oakestra_utils.types.statuses import NegativeSchedulingStatus
print("\n".join(s.value for s in NegativeSchedulingStatus))
' "$UTILS_LIBRARY")
[[ -n "$NEGATIVE_SCHEDULING_STATUSES" ]] || {
  echo "FAIL: no NegativeSchedulingStatus values found in ${UTILS_LIBRARY}" >&2
  exit 1
}

token=""

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

api() {
  local method=$1 path=$2
  shift 2
  curl -fsS --max-time 15 -X "$method" \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    "$@" "${ROOT_API_URL}${path}"
}

http_status() {
  local path=$1
  curl -s -o /dev/null -w '%{http_code}' --max-time 15 \
    -H "Authorization: Bearer ${token}" "${ROOT_API_URL}${path}"
}

# Root calls a service a job and names it "<app>.<app ns>.<service>.<service ns>".
job_name() {
  local app=$1
  echo "${app}.${APP_NS}.${SERVICE_NAME}.${SERVICE_NS}"
}

# root_state and worker_state both print "<instance>:<status>" pairs sorted by
# instance, for example "0:RUNNING 2:RUNNING", or nothing if there are none.
root_state() {
  local service_id=$1
  api GET "/api/service/${service_id}" 2>/dev/null |
    jq -r '[(.instance_list // [])[] | "\(.instance_number):\(.status // "-")"] | sort | join(" ")'
}

# `ctr tasks ls` prints lines like
#   cisanity.ci.sleeper.ci.instance.0.<hash>    1234    RUNNING
# NodeEngine builds that ID from the job name, the instance number and a hash.
worker_state() {
  local job=$1
  sudo ctr -n "$CTR_NAMESPACE" tasks ls 2>/dev/null |
    awk -v prefix="${job}.instance." 'index($1, prefix) == 1 {
      split(substr($1, length(prefix) + 1), rest, ".")
      print rest[1] ":" $3
    }' |
    sort -n | paste -sd' ' -
}

worker_containers() {
  local job=$1
  sudo ctr -n "$CTR_NAMESPACE" containers ls -q 2>/dev/null | grep -F "${job}.instance." || true
}

wait_for() {
  local description=$1
  shift
  local deadline=$((SECONDS + TIMEOUT_SECONDS))
  echo "Waiting up to ${TIMEOUT_SECONDS}s for ${description}"
  until "$@"; do
    if ((SECONDS >= deadline)); then
      echo "Timed out waiting for ${description}" >&2
      return 1
    fi
    sleep "$POLL_INTERVAL"
  done
  echo "OK: ${description}"
}

instances_are() {
  local service_id=$1 job=$2 expected=$3
  local root worker
  root=$(root_state "$service_id") || root="(unreachable)"
  worker=$(worker_state "$job")
  echo "[${SECONDS}s] root='${root}' worker='${worker}' expected='${expected}'"
  # Without this a failed container only shows up as a timeout minutes later.
  if [[ " ${root} ${worker} " == *":FAILED "* ]]; then
    fail "an instance failed on the worker"
  fi
  [[ "$root" == "$expected" && "$worker" == "$expected" ]]
}

instance_count_is() {
  local service_id=$1 expected=$2
  local count
  count=$(api GET "/api/service/${service_id}" 2>/dev/null | jq '(.instance_list // []) | length') || return 1
  [[ "$count" == "$expected" ]]
}

rejected_by_scheduler() {
  local service_id=$1
  local status
  status=$(api GET "/api/service/${service_id}" 2>/dev/null | jq -r '.status // "-"') || return 1
  echo "[${SECONDS}s] job status '${status}'"
  grep -Fxq -- "$status" <<<"$NEGATIVE_SCHEDULING_STATUSES"
}

service_gone_at_root() {
  local service_id=$1
  [[ "$(http_status "/api/service/${service_id}")" == "404" ]]
}

# Prints "<app id> <service id>".
register_app() {
  local app=$1 memory_mb=$2
  local sla apps
  sla=$(jq -n --arg app "$app" --arg app_ns "$APP_NS" --arg svc "$SERVICE_NAME" \
    --arg svc_ns "$SERVICE_NS" --arg image "$IMAGE" --argjson memory "$memory_mb" '{
    sla_version: "v2.0",
    customerID: "Admin",
    applications: [{
      applicationID: "",
      application_name: $app,
      application_namespace: $app_ns,
      application_desc: "E2E sanity check",
      microservices: [{
        microserviceID: "",
        microservice_name: $svc,
        microservice_namespace: $svc_ns,
        virtualization: "container",
        cmd: ["sh", "-c", "tail -f /dev/null"],
        memory: $memory,
        vcpus: 1,
        vgpus: 0,
        vtpus: 0,
        bandwidth_in: 0,
        bandwidth_out: 0,
        storage: 0,
        code: $image,
        state: "",
        added_files: []
      }]
    }]
  }')
  apps=$(api POST /api/application/ -d "$sla")
  jq -er --arg name "$app" \
    '[.[] | select(.application_name == $name)][0] | "\(._id) \(.microservices[0])"' <<<"$apps"
}

echo "Logging in as the default admin"
token=$(curl -fsS --max-time 15 -H "Content-Type: application/json" \
  -d '{"username": "Admin", "password": "Admin"}' \
  "${ROOT_API_URL}/api/auth/login" | jq -er .token)

echo "== Scale up and down"
app=cisanity
job=$(job_name "$app")
ids=$(register_app "$app" 50)
read -r app_id service_id <<<"$ids"
echo "Application ${app_id}, service ${service_id}"

for i in 1 2 3; do
  echo "Requesting instance ${i} of 3"
  api POST "/api/service/${service_id}/instance" >/dev/null
  # Root picks the instance number only once the scheduler answers. Two
  # requests in flight can end up with the same number, so go one at a time.
  wait_for "instance ${i} assigned at root" instance_count_is "$service_id" "$i"
done
wait_for "three instances running" instances_are "$service_id" "$job" "0:RUNNING 1:RUNNING 2:RUNNING"

echo "Removing instance 1 only"
api DELETE "/api/service/${service_id}/instance/1" >/dev/null
wait_for "instances 0 and 2 still running" instances_are "$service_id" "$job" "0:RUNNING 2:RUNNING"

# -1 means all instances. Before #619 this skipped every second one.
echo "Removing all remaining instances"
api DELETE "/api/service/${service_id}/instance/-1" >/dev/null
wait_for "all instances removed" instances_are "$service_id" "$job" ""
leftover=$(worker_containers "$job")
[[ -z "$leftover" ]] || fail "containers left behind in containerd: ${leftover}"

echo "Deleting application ${app_id}"
api DELETE "/api/application/${app_id}" >/dev/null
wait_for "service removed at root" service_gone_at_root "$service_id"

echo "== Unschedulable service"
app=cisanitybig
job=$(job_name "$app")
# 100 TB, nothing fits that.
ids=$(register_app "$app" 100000000)
read -r app_id service_id <<<"$ids"
echo "Application ${app_id}, service ${service_id}"
api POST "/api/service/${service_id}/instance" >/dev/null
wait_for "scheduler rejected the service" rejected_by_scheduler "$service_id"
if [[ -n "$(worker_state "$job")$(worker_containers "$job")" ]]; then
  fail "the rejected service still reached the worker"
fi
api DELETE "/api/application/${app_id}" >/dev/null
wait_for "rejected service removed at root" service_gone_at_root "$service_id"

echo "OK: deployment checks passed"
