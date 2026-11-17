#!/usr/bin/env bash
# Background preLaunch of "Debug Net Manager": once the debugged NetManager is listening on
# its socket, start the installed NodeEngine (nodeengine systemd service). The NetManager
# must come first, otherwise the NodeEngine fails to register to the network component.
#
# VS Code starts the debug session as soon as this prints the "Waiting for" line, so
# everything that may prompt (sudo) or must happen before the NetManager starts runs above it.
#
# Usage: start-node-engine-after-net-manager.sh [timeout-seconds]

socket=/etc/netmanager/netmanager.sock
timeout="${1:-300}"

sudo -v || exit 1
# A socket left over from a previous run would make the NodeEngine start too early.
sudo rm -f "$socket"

echo "Waiting for NetManager socket $socket"

i=0
while [ "$i" -lt "$timeout" ]; do
    if sudo test -S "$socket"; then
        echo "NetManager is up, starting the NodeEngine"
        sudo systemctl restart nodeengine
        echo "NodeEngine started. Follow it with: tail -f /var/log/oakestra/nodeengine.log"
        exit 0
    fi
    i=$((i + 1))
    sleep 1
done

echo "ERROR: NetManager socket $socket did not appear after ${timeout}s, NodeEngine not started." >&2
exit 1
