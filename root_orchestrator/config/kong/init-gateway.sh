#!/bin/sh
# Idempotent — deck gateway sync diffs against live state and applies only changes.
# Uses separate workspaces so external and internal configs never collide.

EXTERNAL_ADMIN="http://kong_external:8001"
INTERNAL_ADMIN="http://kong_internal:8001"

echo "==> Syncing external gateway..."
if ! deck gateway sync \
  --kong-addr "$EXTERNAL_ADMIN" \
  /external.yaml; then
  echo "ERROR: external sync failed" >&2
  exit 1
fi

echo "==> Syncing internal gateway..."
if ! deck gateway sync \
  --kong-addr "$INTERNAL_ADMIN" \
  /internal.yaml; then
  echo "ERROR: internal sync failed" >&2
  exit 1
fi

echo "==> Gateway init complete."
