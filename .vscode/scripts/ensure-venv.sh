#!/usr/bin/env bash
# Creates (if needed) a per-component virtualenv under <repo>/.venv/<name> and installs
# the given pip arguments into it. Used by the VS Code debug profiles, so each Python
# component gets its own dependencies: the components pin incompatible versions
# (e.g. Flask 2.x vs 3.x), so a single shared interpreter breaks whichever ran first.
#
# Usage: ensure-venv.sh <name> <fallback-python> <pip install args...>
#   Paths in the pip args are relative to the repository root.
#
# The base interpreter is, in order: $OAK_DEBUG_PYTHON, python3.10 (the version the
# Docker images use), then <fallback-python> (the interpreter selected in VS Code).
# The install is skipped when the requirement files and arguments are unchanged.
# Delete .venv/<name> to rebuild a virtualenv from scratch.
set -euo pipefail

if [ "$#" -lt 3 ]; then
    echo "Usage: $0 <name> <fallback-python> <pip install args...>" >&2
    exit 2
fi

name="$1"
fallback="$2"
shift 2

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
venv="$root/.venv/$name"
cd "$root"

if [ ! -x "$venv/bin/python" ]; then
    base="${OAK_DEBUG_PYTHON:-}"
    if [ -z "$base" ]; then
        base="$(command -v python3.10 || true)"
    fi
    if [ -z "$base" ]; then
        base="$fallback"
    fi
    version="$("$base" -c 'import sys; print("%d.%d" % sys.version_info[:2])')"
    echo "Creating .venv/$name with $base (Python $version)"
    if [ "$version" != "3.10" ]; then
        echo "Note: the Oakestra images run Python 3.10. Some pinned dependencies may not install on $version." >&2
        echo "      Install python3.10 or set OAK_DEBUG_PYTHON, then delete .venv/$name to rebuild it." >&2
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
    echo ".venv/$name: dependencies up to date"
    exit 0
fi

echo "Installing dependencies into .venv/$name"
"$venv/bin/python" -m pip install --disable-pip-version-check "$@"
echo "$want" > "$stamp"
