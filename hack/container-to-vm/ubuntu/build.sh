#!/usr/bin/env bash

set -euo pipefail

ARCH=$1
VERSION="${2:-"25.04"}"

curl \
    --proto '=https' \
    --tlsv1.2 \
    --silent \
    --show-error \
    --fail \
    --location \
    "https://cloud-images.ubuntu.com/releases/${VERSION}/release/ubuntu-${VERSION}-server-cloudimg-${ARCH}-root.tar.xz" \
  | podman import --arch "$ARCH" - "ubuntu-cloud:${VERSION}"

podman build --arch "$ARCH" --tag "ubuntu-kvm:${VERSION}" --build-arg "VERSION=${VERSION}" .
