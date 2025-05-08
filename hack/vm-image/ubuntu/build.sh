#!/usr/bin/env bash

set -euo pipefail

COLOR_RED='\033[0;31m'
COLOR_OFF='\033[0m'

ARCH=$1
VERSION="${2:-"25.04"}"

STORE_DRIVER="$(podman info -f '{{.Store.GraphDriverName}}')"
if [[ "${STORE_DRIVER}" != "btrfs" ]]; then
  echo -e "${COLOR_RED}Podman needs to be configured with the 'btrfs' store driver for the build to work correctly.${COLOR_OFF}" 1>&2
  exit 1
fi

curl \
    --proto '=https' \
    --tlsv1.2 \
    --silent \
    --show-error \
    --fail \
    --location \
    "https://cloud-images.ubuntu.com/releases/${VERSION}/release/ubuntu-${VERSION}-server-cloudimg-${ARCH}-root.tar.xz" \
  | xzcat \
  | podman import --arch "$ARCH" - "ubuntu-cloud:${VERSION}"

podman build --arch "$ARCH" --tag "oakestra-vm-ubuntu:${VERSION}" --build-arg "VERSION=${VERSION}" .
