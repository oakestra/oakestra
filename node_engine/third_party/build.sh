#!/usr/bin/env bash
set -euo pipefail

# Determine the directory where this script resides
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Architectures to build
ARCHES=(amd64 arm64)

# Ensure top-level dist dir for combined archives
mkdir --parents "${SCRIPT_DIR}/dist"

for ARCH in "${ARCHES[@]}"; do
  echo "=== Building all deps for linux/${ARCH} ==="

  # Build each dependency into its own tar under dep/dist/<arch>.tar
  for DEP_DIR in "${SCRIPT_DIR}"/*/; do
    [ -d "${DEP_DIR}" ] || continue
    DEP_NAME="$(basename "${DEP_DIR}")"
    # Skip the top-level dist directory
    if [[ "${DEP_NAME}" == "dist" ]]; then
      continue
    fi

    echo "---- Building dep: ${DEP_NAME} ----"
    pushd "${DEP_DIR}" > /dev/null
    ARCH="${ARCH}" ./build.sh
    popd > /dev/null
  done

  # Base tar for this arch (empty to start)
  DEV_TAR="${SCRIPT_DIR}/dist/dev-${ARCH}.tar"
  PROD_TAR="${SCRIPT_DIR}/dist/prod-${ARCH}.tar"
  tar --create --file="${DEV_TAR}" --files-from=/dev/null

  # Concatenate all per-dep tars into the base tar
  echo "Concatenating tarballs for ${ARCH} into development archive..."
  tar --concatenate \
    --file="${DEV_TAR}" \
    --owner=0 \
    --group=0 \
    "${SCRIPT_DIR}"/*/dist/"${ARCH}.tar"

  # Create copy without files not needed for production
  echo "Creating production archive for ${ARCH} by removing development files..."
  cp "${DEV_TAR}" "${PROD_TAR}"
  tar --delete --file="${PROD_TAR}" 'include' >/dev/null 2>&1 || true
  tar --delete --file="${PROD_TAR}" 'lib/pkgconfig' >/dev/null 2>&1 || true

  # Compress the combined archive
  gzip --force "${DEV_TAR}"
  gzip --force "${PROD_TAR}"
  echo "Created archives: ${DEV_TAR}.gz ${PROD_TAR}.gz"
done

echo "All architectures built and archived."
