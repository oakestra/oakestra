#!/bin/sh
# One-shot init for the external gateway's public server certificate.
#
# The external gateway presents a publicly trusted server cert from
# /certs/public/ that is a *separate certificate system* from the internal
# mTLS CA. The operator must provide it (BYO): drop fullchain.pem + privkey.pem
# there, e.g. from Let's Encrypt or your own PKI. There is intentionally NO
# auto-generated fallback — the public-facing identity is never derived from
# the internal root CA.
#
# This script:
#   1. waits for the internal CA (needed by Kong to verify client certs),
#   2. fails fast if the BYO public cert is missing,
#   3. normalises ownership so the kong user (UID 1000 in kong:3.6) can read
#      the key (nginx runs as kong and otherwise gets "Permission denied").

set -eu

CERT_DIR="/certs"
PUBLIC_DIR="$CERT_DIR/public"
KONG_UID=1000
KONG_GID=1000
TIMEOUT=120

echo "Waiting for system_manager to generate the internal CA..."
elapsed=0
while [ ! -f "$CERT_DIR/ca.crt" ]; do
    if [ "$elapsed" -ge "$TIMEOUT" ]; then
        echo "ERROR: $CERT_DIR/ca.crt not present after ${TIMEOUT}s." >&2
        echo "       system_manager generates the internal CA at startup — check its logs." >&2
        exit 1
    fi
    sleep 2
    elapsed=$((elapsed + 2))
done

if [ ! -f "$PUBLIC_DIR/fullchain.pem" ] || [ ! -f "$PUBLIC_DIR/privkey.pem" ]; then
    echo "ERROR: public gateway certificate not found." >&2
    echo "       The external gateway requires a bring-your-own public certificate." >&2
    echo "       Provide $PUBLIC_DIR/fullchain.pem and $PUBLIC_DIR/privkey.pem" >&2
    echo "       (e.g. a Let's Encrypt cert, or your own PKI) and restart." >&2
    exit 1
fi

# Make the BYO material readable by the kong user (UID/GID 1000) without making
# the private key world-readable. Applied every run so restarts and fresh drops
# both end up kong-readable.
chown "$KONG_UID:$KONG_GID" "$PUBLIC_DIR" "$PUBLIC_DIR/fullchain.pem" "$PUBLIC_DIR/privkey.pem"
chmod 644 "$PUBLIC_DIR/fullchain.pem"
chmod 600 "$PUBLIC_DIR/privkey.pem"
echo "Public gateway certificate present; normalised ownership for the kong user."
