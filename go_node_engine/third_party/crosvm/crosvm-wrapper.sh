#!/usr/bin/env sh

LD_LIBRARY_PATH="$(realpath "$(dirname "$0")/../lib"):${LD_LIBRARY_PATH}" "$(dirname "$0")/crosvm" "$@"
