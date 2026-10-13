#!/usr/bin/env bash
# Deploys a single-container service through the root API, waits until it runs
# on the worker, then undeploys it and waits until the worker removed it again.
# This walks the full root -> cluster -> worker path in both directions.
#
# Must run on the worker host: it inspects containerd directly via `ctr`.
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

APP_NAME=cisanity
APP_NS=ci
SERVICE_NAME=sleeper
SERVICE_NS=ci
# NodeEngine names containerd tasks after the job name plus the instance number.
JOB_NAME="${APP_NAME}.${APP_NS}.${SERVICE_NAME}.${SERVICE_NS}"
# Must match the containerd namespace hardcoded in the NodeEngine.
CTR_NAMESPACE=oakestra

token=""

api() {
  local method=$1 path=$2
  shift 2
  curl -fsS --max-time 15 -X "$method" \
    -H "Authorization: Bearer ${token}" \
    -H "Content-Type: application/json" \
    "$@" "${ROOT_API_URL}${path}"
}

worker_tasks() {
  sudo ctr -n "$CTR_NAMESPACE" tasks ls 2>/dev/null | grep -F "$JOB_NAME" || true
}

# Polls the given predicate until it succeeds or the timeout runs out.
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

service_json() {
  api GET "/api/service/${service_id}" 2>/dev/null
}

instance_running_at_root() {
  local body status
  body=$(service_json) || {
    echo "[${SECONDS}s] could not fetch service"
    return 1
  }
  status=$(jq -r '"job=\(.status // "-") instance=\(.instance_list[0].status // "-")"' <<<"$body")
  echo "[${SECONDS}s] ${status}"
  # A failed container would otherwise only surface as a timeout minutes later.
  if jq -e '.instance_list[0].status == "FAILED"' <<<"$body" >/dev/null; then
    echo "Instance failed on the worker: $(jq -c '.instance_list[0]' <<<"$body")" >&2
    exit 1
  fi
  jq -e '.instance_list[0].status == "RUNNING"' <<<"$body" >/dev/null
}

task_running_on_worker() {
  worker_tasks | grep -q RUNNING
}

instances_gone_at_root() {
  service_json | jq -e '(.instance_list // []) | length == 0' >/dev/null
}

task_gone_on_worker() {
  [[ -z "$(worker_tasks)" ]]
}

echo "Logging in as the default admin"
token=$(curl -fsS --max-time 15 -H "Content-Type: application/json" \
  -d '{"username": "Admin", "password": "Admin"}' \
  "${ROOT_API_URL}/api/auth/login" | jq -er .token)

# busybox keeps the pull small, and the container needs no ports because the
# worker runs without the overlay network.
sla=$(jq -n \
  --arg app "$APP_NAME" --arg app_ns "$APP_NS" \
  --arg svc "$SERVICE_NAME" --arg svc_ns "$SERVICE_NS" '{
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
      memory: 50,
      vcpus: 1,
      vgpus: 0,
      vtpus: 0,
      bandwidth_in: 0,
      bandwidth_out: 0,
      storage: 0,
      code: "docker.io/library/busybox:1.36",
      state: "",
      added_files: []
    }]
  }]
}')

echo "Registering application ${APP_NAME}"
apps=$(api POST /api/application/ -d "$sla")
app_id=$(jq -er --arg name "$APP_NAME" '[.[] | select(.application_name == $name)][0]._id' <<<"$apps")
service_id=$(jq -er --arg name "$APP_NAME" \
  '[.[] | select(.application_name == $name)][0].microservices[0]' <<<"$apps")
echo "Application ${app_id}, service ${service_id}"

echo "Deploying one instance"
api POST "/api/service/${service_id}/instance" >/dev/null

wait_for "instance RUNNING at root" instance_running_at_root
wait_for "task RUNNING in containerd" task_running_on_worker
worker_tasks

echo "Undeploying the instance"
api DELETE "/api/service/${service_id}/instance/0" >/dev/null

wait_for "instance removed at root" instances_gone_at_root
wait_for "task removed from containerd" task_gone_on_worker

echo "Deleting application ${app_id}"
api DELETE "/api/application/${app_id}" >/dev/null

echo "OK: deploy and undeploy round trip passed"
