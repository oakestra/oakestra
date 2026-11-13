#!/bin/sh
# Run a service and act when the certificate files it reads change on disk. The
# orchestrator then only writes the files and needs no access to the Docker socket.
#
# Used by: kong_external (root), cluster_kong_external, mqtt, cluster_service_manager
# (via override-gateway.yml). Identical copies live in root_orchestrator/config/ and
# cluster_orchestrator/config/ — keep them in sync.
#
# Usage: cert-watch.sh <the service's entrypoint and command...>
#
# Configuration (environment variables):
#   CERT_WATCH_FILES     space-separated files to watch (required); a missing file is a
#                        state too, so creating one counts as a change
#   CERT_WATCH_ACTION    what to do once they changed:
#                          command  run CERT_WATCH_COMMAND, e.g. "kong reload" (3 attempts)
#                          signal   send CERT_WATCH_SIGNAL (default: HUP) to the service
#                          restart  stop the service and start it again
#   CERT_WATCH_INTERVAL  seconds between checks (default: 10)
#   CERT_WATCH_SETTLE    seconds a change must stay stable before acting, so files written
#                        one after the other are picked up together (default: 3)

set -u

WATCH_FILES="${CERT_WATCH_FILES:-}"
ACTION="${CERT_WATCH_ACTION:-}"
INTERVAL="${CERT_WATCH_INTERVAL:-10}"
SETTLE="${CERT_WATCH_SETTLE:-3}"

log() { echo "cert-watch: $*" >&2; }

if [ -z "$WATCH_FILES" ] || [ $# -eq 0 ]; then
  log "ERROR: set CERT_WATCH_FILES and pass the service command"
  exit 64
fi
case "$ACTION" in
  command) [ -n "${CERT_WATCH_COMMAND:-}" ] || { log "ERROR: set CERT_WATCH_COMMAND"; exit 64; } ;;
  signal | restart) ;;
  *) log "ERROR: CERT_WATCH_ACTION must be command, signal or restart"; exit 64 ;;
esac

# One checksum over the content of every watched file.
fingerprint() {
  for f in $WATCH_FILES; do
    if [ -r "$f" ]; then
      printf '%s ' "$f"
      cksum < "$f"
    else
      echo "$f missing"
    fi
  done | cksum
}

# Sleep that returns early when a signal arrives, so shutdown is not delayed.
pause() {
  sleep "$1" &
  wait $! 2>/dev/null || true
}

start_service() {
  "$@" &
  SERVICE_PID=$!
  SERVICE_EXITED=0
}

# Wait for the service to exit and keep its status. wait returns early when a trap
# runs, so keep waiting until the process is really gone.
wait_for_service() {
  [ "$SERVICE_EXITED" -eq 0 ] || return 0
  while :; do
    wait "$SERVICE_PID"
    status=$?
    kill -0 "$SERVICE_PID" 2>/dev/null || break
  done
  SERVICE_EXITED=1
}

STOPPING=0
# Pass a stop signal on to the service. A POSIX shell starts background jobs with
# SIGINT ignored, so INT becomes TERM. QUIT is passed on as is: it is Kong's stop
# signal (graceful shutdown), and nginx installs its own handler for it.
# shellcheck disable=SC2329  # invoked from the traps below
forward() {
  STOPPING=1
  kill -s "$1" "$SERVICE_PID" 2>/dev/null || true
}
trap 'forward TERM' TERM
trap 'forward TERM' INT
trap 'forward QUIT' QUIT

act() {
  case "$ACTION" in
    command)
      attempt=1
      until sh -c "$CERT_WATCH_COMMAND"; do
        if [ "$attempt" -ge 3 ]; then
          log "ERROR: '$CERT_WATCH_COMMAND' failed ${attempt} times; the service keeps the previous files"
          return
        fi
        attempt=$((attempt + 1))
        pause 2
      done
      log "'$CERT_WATCH_COMMAND' succeeded"
      ;;
    signal)
      kill -s "${CERT_WATCH_SIGNAL:-HUP}" "$SERVICE_PID" && log "sent ${CERT_WATCH_SIGNAL:-HUP}"
      ;;
    restart)
      kill -s TERM "$SERVICE_PID" 2>/dev/null
      waited=0
      while kill -0 "$SERVICE_PID" 2>/dev/null && [ "$waited" -lt 30 ]; do
        pause 1
        waited=$((waited + 1))
      done
      kill -s KILL "$SERVICE_PID" 2>/dev/null
      wait_for_service
      [ "$STOPPING" -eq 0 ] || return
      start_service "$@"
      log "service restarted"
      ;;
  esac
}

start_service "$@"
log "watching: $WATCH_FILES (every ${INTERVAL}s, action: $ACTION)"
last="$(fingerprint)"

while [ "$STOPPING" -eq 0 ] && kill -0 "$SERVICE_PID" 2>/dev/null; do
  pause "$INTERVAL"
  [ "$STOPPING" -eq 0 ] || break

  current="$(fingerprint)"
  [ "$current" = "$last" ] && continue

  # Files are usually rewritten one after another: wait until they stop changing.
  pause "$SETTLE"
  settled="$(fingerprint)"
  if [ "$settled" != "$current" ]; then
    log "files still changing, checking again on the next round"
    continue
  fi

  log "certificate files changed"
  act "$@"
  # Remember this state either way; the next change triggers a new attempt.
  last="$settled"
done

wait_for_service
log "service exited with status $status"
exit "$status"
