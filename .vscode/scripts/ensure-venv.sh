#!/usr/bin/env bash
# Creates (if needed) a per-component virtualenv at <repo>/<component-dir>/.venv and installs
# the given pip arguments into it. Only for components not managed by the uv workspace: the
# root .venv belongs to `uv sync`, which removes anything it does not track. Used by the VS Code debug profiles, so each Python
# component gets its own dependencies: the components pin incompatible versions
# (e.g. Flask 2.x vs 3.x), so a single shared interpreter breaks whichever ran first.
#
# Usage: ensure-venv.sh <component-dir> <fallback-python> <pip install args...>
#   <component-dir> and paths in the pip args are relative to the repository root.
#
# The base interpreter is, in order: $OAK_DEBUG_PYTHON, python3.10 (the version the
# Docker images use), then <fallback-python> (the interpreter selected in VS Code).
# The install is skipped when the requirement files and arguments are unchanged.
# Delete <component-dir>/.venv to rebuild a virtualenv from scratch.
set -euo pipefail

if [ "$#" -lt 3 ]; then
    echo "Usage: $0 <component-dir> <fallback-python> <pip install args...>" >&2
    exit 2
fi

name="$1"
fallback="$2"
shift 2

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
venv="$root/$name/.venv"
cd "$root"

# A venv whose creation failed half-way (e.g. ensurepip missing) has a python but no pip; rebuild it.
if [ -x "$venv/bin/python" ] && ! "$venv/bin/python" -m pip --version >/dev/null 2>&1; then
    echo "Removing broken $name/.venv (no pip)"
    rm -rf "$venv"
fi

if [ ! -x "$venv/bin/python" ]; then
    base="${OAK_DEBUG_PYTHON:-}"
    if [ -z "$base" ]; then
        base="$(command -v python3.10 || true)"
    fi
    if [ -z "$base" ]; then
        base="$fallback"
    fi
    version="$("$base" -c 'import sys; print("%d.%d" % sys.version_info[:2])')"
    echo "Creating $name/.venv with $base (Python $version)"
    if [ "$version" != "3.10" ]; then
        echo "Note: the Oakestra images run Python 3.10. Some pinned dependencies may not install on $version." >&2
        echo "      Install python3.10 or set OAK_DEBUG_PYTHON, then delete $name/.venv to rebuild it." >&2
    fi
    "$base" -m venv "$venv"
fi

# Fingerprint of the arguments plus every requirements file / local package they reference.
# (Passed with -c rather than a heredoc: macOS ships bash 3.2, which mis-parses heredocs in $(...).)
fingerprint_py='
import hashlib, os, sys
h = hashlib.sha256()
for arg in sys.argv[1:]:
    h.update(arg.encode() + b"\0")
    if os.path.isfile(arg):
        h.update(open(arg, "rb").read())
    elif os.path.isdir(arg):
        for meta in ("pyproject.toml", "setup.py", "setup.cfg"):
            path = os.path.join(arg, meta)
            if os.path.isfile(path):
                h.update(open(path, "rb").read())
print(h.hexdigest())
'
stamp="$venv/.oak-requirements"
want="$("$venv/bin/python" -c "$fingerprint_py" "$@")"

if [ -f "$stamp" ] && [ "$(cat "$stamp")" = "$want" ]; then
    echo "$name/.venv: dependencies up to date"
    exit 0
fi

echo "Installing dependencies into $name/.venv"
"$venv/bin/python" -m pip install --disable-pip-version-check "$@"
echo "$want" > "$stamp"
