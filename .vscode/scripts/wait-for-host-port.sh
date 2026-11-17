#!/usr/bin/env bash
# Waits until a container of the debug stack answers on localhost:<port>.
# The stack runs with network_mode: host (override-network-host.yml), so a locally debugged
# component reaches the other containers on localhost. If that never works, fail with an
# explanation instead of letting the debugged component retry forever.
#
# Usage: wait-for-host-port.sh <port> <container-name> [timeout-seconds]

if [ "$#" -lt 2 ]; then
    echo "Usage: $0 <port> <container-name> [timeout-seconds]" >&2
    exit 2
fi

port="$1"
name="$2"
timeout="${3:-60}"

i=0
while [ "$i" -lt "$timeout" ]; do
    if (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
        echo "$name is reachable on localhost:$port"
        exit 0
    fi
    i=$((i + 1))
    sleep 1
done

echo "ERROR: $name is not reachable on localhost:$port after ${timeout}s." >&2
docker ps -a --filter "name=^${name}\$" --format 'Container {{.Names}}: {{.Status}}' >&2

if [ "$(uname)" = "Darwin" ]; then
    cat >&2 <<'EOF'

On macOS, containers with network_mode: host live inside Docker's VM, so their ports are
not on your Mac's localhost unless host networking is turned on:

  Docker Desktop 4.34+: sign in, then Settings > Resources > Network >
                        "Enable host networking", and "Apply and restart".
  OrbStack:             host networking works out of the box.

Then run the debug profile again.
EOF
fi
exit 1
