#!/bin/sh
# Container entrypoint wrapper: copies any *.crt files found in /certs into
# the container's own OS CA trust store (/etc/ssl/certs/ca-certificates.crt)
# before handing off to the real process. This affects only this container's
# filesystem — the host and other containers are unchanged.
#
# This lets operators drop a custom CA (e.g. a self-signed gateway CA) into
# the mounted cert directory and have it trusted automatically when
# ROOT_GATEWAY_TRUST / CLUSTER_GATEWAY_TRUST is set to "system", without
# modifying the container image or rebuilding.
#
# Used by: system_manager, root_service_manager (via override-gateway.yml).

set -e

for f in /certs/*.crt; do
    [ -f "$f" ] || continue
    cp "$f" /usr/local/share/ca-certificates/"$(basename "$f")"
done
update-ca-certificates -f 2>/dev/null || true

# Python's requests (certifi) and gRPC both bundle their own CA stores and
# ignore the OS trust store. Redirect both to the OS store, which now
# includes any custom CAs copied from /certs above.
export REQUESTS_CA_BUNDLE=/etc/ssl/certs/ca-certificates.crt
export GRPC_DEFAULT_SSL_ROOTS_FILE_PATH=/etc/ssl/certs/ca-certificates.crt

exec "$@"
