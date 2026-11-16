# AGENTS.md

This file provides guidance agents when working with code in this repository.

## Repository Overview

Oakestra Net is the networking layer for the [Oakestra](https://oakestra.io) edge computing platform. It enables service-to-service communication across distributed worker nodes and clusters via a semantic overlay network (semantic addressing maps IPs to load-balancing policies, not just destinations).

Three components:

1. **node-net-manager** (Go) — daemon installed on each worker node; manages network namespaces, iptables rules, TUN interfaces, and the proxy tunnel for container/unikernel traffic
2. **root-service-manager** (Python/Flask) — control-plane component at the root orchestrator; manages global subnetwork allocation and route propagation across clusters
3. **cluster-service-manager** (Python/Flask) — control-plane component at the cluster orchestrator; receives routes from root and distributes them to nodes via MQTT

## Build & Run

### node-net-manager (Go 1.22+)

```bash
# Build for both architectures (from node-net-manager/build/)
cd node-net-manager/build && ./build.sh
# Output: bin/amd64-NetManager, bin/arm64-NetManager

# Install on this machine
./install.sh amd64   # or arm64

# Run tests
cd node-net-manager && go test ./...

# Run a single test package
cd node-net-manager && go test ./proxy/...

# Run the daemon (requires root)
sudo NetManager
```

### root-service-manager / cluster-service-manager (Python 3.10)

```bash
# Install dependencies (from respective service-manager/ directory)
pip install -r requirements.txt

# Run tests
cd root-service-manager/service-manager && pytest
cd cluster-service-manager/service-manager && pytest

# Build Docker images
cd root-service-manager && docker build -t local_root_service_manager service-manager/
cd cluster-service-manager && docker build -t local_cluster_service_manager service-manager/
```

### Local integration with Oakestra orchestrator

To replace the official images with local builds when starting Oakestra:
```bash
export OAKESTRA_BRANCH=develop
export OVERRIDE_FILES=override-local-service-manager.yml
curl -sfL https://raw.githubusercontent.com/oakestra/oakestra/develop/scripts/StartOakestraRoot.sh | sh -
```

## Architecture

### Address Space

| Range | Purpose |
|---|---|
| `10.16.0.0/12` | Container network (all instance IPs) |
| `10.30.0.0/16` | Service Virtual IPs (require proxy translation) |
| `10.19.1.0/24` | Default bridge subnetwork on each node |

Port `50103` is reserved and cannot be used by deployed services.

### node-net-manager internals

- **`env/`** — EnvironmentManager: owns host/namespace management only, creating/destroying network namespaces and dispatching between container (`env/ContainerNetDeployment.go`) and unikernel (`env/UnikernelNetDeployment.go`) deployments; Service IP resolution is delegated to `resolver`
- **`resolver/`** — ServiceResolver: resolves Service IPs, Namespace IPs and Instance IPs against the translation table and kicks off background MQTT table queries on a miss. Netlink-free, which is what lets `proxy` build and test without it
- **`proxy/`** — the datapath, split in two: `Datapath` (`proxy/Datapath.go`) decides what happens to a packet and does no I/O, `Tunnel` (`proxy/ProxyTunnel.go`) owns the TUN device, the listen socket, the per-peer connection pool and the read loops that act on `Datapath`'s verdict. See "Packet path" below
- **`mqtt/`** — subscribes to cluster MQTT topics for route/table-query updates; implements interest registration with a self-destruct timeout when a service is no longer needed
- **`network/`** — iptables rule management (`network/iptables.go`) and NAT utilities
- **`handlers/`** — dispatches deployment/removal requests to the correct manager (container vs unikernel)
- **`server/`** — HTTP REST API accepting requests from NodeEngine (Unix socket at `/etc/netmanager/netmanager.sock`)
- **`TableEntryCache/`** — in-memory cache for the service translation table; `TableManager` keeps three address indexes, rebuilt wholesale on every mutation, and bumps an atomic generation counter each time
- **`events/`** — one atomic "last used" timestamp per job, touched on the packet path and polled by the MQTT interest timeout
- **`clock/`** — a 1Hz coarse Unix-seconds clock. Anything on the packet path that needs a "last used" timestamp reads this instead of `time.Now()`, which costs ~29ns and swamps the lookup it is timing
- **`cmd/`** — Cobra CLI (`root.go` startup, `logs.go`, `status.go`, `version.go`); version injected at build time via `-ldflags`
- **`model/`** — `NetConfiguration` struct mirrors `/etc/netmanager/netcfg.json`

#### Packet path

The outgoing path is split by how warm the flow is. `ProxyCache.Lookup` answers from the flow cache alone whenever the route's `routeGen` still matches `TableManager`'s current generation, so a steady-state packet touches no index at all: one atomic load, one shard mutex, one short bucket scan. Everything that consults the table lives in `Datapath.resolveRoute`, off that path.

A generation bump sends the flow through `ProxyCache.Revalidate`, which *refreshes* the flow (its instance IPs and the destination job's activity stamp) rather than rerouting it - an established connection has to keep the replica it was pinned to. `FlowKey` therefore deliberately excludes the source instance IP: it is a function of the source namespace IP under a given generation, and including it would force a table lookup before the cache could even be asked. For the same reason `Resolver.GetInstanceIP` hands back a single `netip.Addr` rather than a whole `TableEntry` - copying one per packet to read one address out of it used to dominate the outgoing path.

I/O goes through two seams, `TunDevice` (`proxy/TunDevice.go`) and `TunnelSocket` (`proxy/TunnelSocket.go`), each with a `ReadBatch`/`WriteBatch` pair so a batch of packets costs one syscall instead of one per packet. Both directions are batched: ingoing writes a whole batch to the TUN device in one call, outgoing groups one read's packets by destination (`outgoingBatch`) and issues one write per distinct peer. Each peer has its own dialled `tunnelConn` - deliberately not unified with the listen socket, which would swap tunnel traffic's source port from ephemeral to the fixed listen port - carrying an `*ipv4.PacketConn` or `*ipv6.PacketConn` chosen from the peer's address family, since a node can have both v4 and v6 peers at once.

`TunDevice` is backed by `golang.zx2c4.com/wireguard/tun`; `github.com/google/gopacket` is used for test fixtures only.

### root-service-manager internals (Python/Flask)

- **`network/subnetwork_management.py`** — allocates Service VIPs from `10.30.0.0/16`; persists next-available IP in MongoDB
- **`network/routes_interests.py`** — tracks which clusters have interest in which services
- **`operations/`** — service, instance, and cluster lifecycle management
- **`blueprints/`** — Flask-Smorest blueprints exposing OpenAPI endpoints; Swagger UI at `/api/docs`
- Uses JWT (RS256) for auth; public key fetched from root orchestrator

### cluster-service-manager internals (Python/Flask)

- **`interfaces/mqtt_client.py`** — MQTT client (paho-mqtt) connecting to cluster broker
- **`interfaces/root_service_manager_requests.py`** — HTTP calls to root service manager
- **`operations/`** — handles instance deployment/undeployment, propagates routes down to nodes via MQTT

### Communication flow

```
NodeEngine ──HTTP──> node-net-manager (server/)
node-net-manager ──MQTT──> cluster-service-manager ──HTTP──> root-service-manager
root-service-manager ──SocketIO──> cluster-service-manager (route push)
cluster-service-manager ──MQTT──> node-net-manager (table updates)
```

## Configuration

node-net-manager config at `/etc/netmanager/netcfg.json`:
```json
{
  "NodePublicAddress": "address",
  "NodePublicPort": "port",
  "ClusterUrl": "url",
  "ClusterMqttPort": "port",
  "Debug": false
}
```

Set `"Debug": true` to enable verbose logging to `/var/log/oakestra/netmanager.log`.

## Runtime Environment Variables

### root-service-manager

| Variable | Default | Purpose |
|---|---|---|
| `ROOT_MONGO_URL` | — | MongoDB host |
| `ROOT_MONGO_PORT` | — | MongoDB port (typically `10008`) |
| `JWT_GENERATOR_URL` | `localhost` | jwt_generator host (for RS256 public key) |
| `JWT_GENERATOR_PORT` | `10011` | jwt_generator port |
| `MY_PORT` | `10100` | Port this service listens on |

### cluster-service-manager

| Variable | Default | Purpose |
|---|---|---|
| `CLUSTER_MONGO_URL` | — | MongoDB host |
| `CLUSTER_MONGO_PORT` | — | MongoDB port (typically `10108`) |
| `MQTT_BROKER_URL` | — | MQTT broker host |
| `MQTT_BROKER_PORT` | — | MQTT broker port (typically `10003`) |
| `ROOT_SERVICE_MANAGER_URL` | `0.0.0.0` | root-service-manager host |
| `ROOT_SERVICE_MANAGER_PORT` | `5000` | root-service-manager port |
| `MQTT_CERT` | — | Path to TLS certs dir (only when using MQTT auth override) |
| `MY_PORT` | `10200` | Port this service listens on |

---

## CI

- Go tests run on every push via `.github/workflows/node_net_manager_tests.yml`
- Python tests for both service managers run on every push via `root_tests.yml` and `cluster_tests.yml`
- Docker images are built and published via `root_service_manager_image.yml` and `cluster_service_manager_image.yml`
