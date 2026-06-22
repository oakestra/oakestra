#!/bin/sh
# Idempotent — deck gateway sync diffs against live state and applies only changes.
# Uses separate workspaces so external and internal configs never collide.

EXTERNAL_ADMIN="http://kong_external:8001"
INTERNAL_ADMIN="http://kong_internal:8001"
CA_CERT_UUID="cafe0000-0000-4000-8000-000000000000"
CA_CERT_FILE="/certs/ca.crt"

# Build a deck-compatible state file for the CA cert so it is included in the
# sync without needing curl or any other HTTP client in this image.
# system_manager generates the CA at startup, so the fallback branch should
# only trigger if system_manager failed to come up before this init ran.
CA_CERTS_YAML=/tmp/ca_certs.yaml
if [ -f "$CA_CERT_FILE" ]; then
  {
    printf '_format_version: "3.0"\nca_certificates:\n'
    printf '  - id: "%s"\n' "$CA_CERT_UUID"
    printf '    tags:\n      - oakestra-ca\n'
    printf '    cert: |\n'
    awk '{print "      " $0}' "$CA_CERT_FILE"
  } > "$CA_CERTS_YAML"
  echo "==> CA cert file found — will include in sync"
else
  printf '_format_version: "3.0"\n' > "$CA_CERTS_YAML"
  echo "WARNING: $CA_CERT_FILE not found — system_manager generates it at startup; check its logs" >&2
fi

echo "==> Syncing external gateway..."
if ! deck gateway sync \
  --kong-addr "$EXTERNAL_ADMIN" \
  /external.yaml "$CA_CERTS_YAML"; then
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
