#!/usr/bin/env bash
# Prints this machine's primary IPv4 address, used as SYSTEM_MANAGER_URL / CLUSTER_ADDRESS
# by the debug stack. Linux: `ip route`. macOS: the default route's interface via `ipconfig`.
# Falls back to 127.0.0.1 when neither works (e.g. offline).

addr=""
if command -v ip >/dev/null 2>&1; then
    addr="$(ip -o route get to 8.8.8.8 2>/dev/null | sed -nE 's/.*src ([0-9.]+).*/\1/p')"
fi
if [ -z "$addr" ] && command -v ipconfig >/dev/null 2>&1; then
    iface="$(route -n get default 2>/dev/null | awk '/interface:/ {print $2}')"
    addr="$(ipconfig getifaddr "${iface:-en0}" 2>/dev/null)"
fi
echo "${addr:-127.0.0.1}"
