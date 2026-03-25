#!/usr/bin/env bash
set -euo pipefail

# Select container engine: prefer podman, then docker
if command -v podman >/dev/null 2>&1; then
  ENGINE=podman
elif command -v docker >/dev/null 2>&1; then
  ENGINE=docker
else
  echo "Error: neither podman nor docker is installed." >&2
  exit 1
fi

mkdir --parents "dist"
"${ENGINE}" build \
  --quiet \
  --platform "linux/${ARCH}" \
  --output "type=tar,dest=dist/${ARCH}.tar" \
  .
