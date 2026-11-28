# Oakestra — Developer Context for AI Assistants

## What Oakestra Is

Oakestra is a **lightweight orchestration platform for edge computing**. Unlike Kubernetes or K3s, which were designed for cloud-grade machines, Oakestra targets heterogeneous, resource-constrained edge devices. The entire root + cluster stack runs in ~1 GB RAM. A worker node needs only 100 MB RAM and 50 MB disk.

The platform orchestrates containerised workloads (Docker, containerd) and unikernels across a two-level hierarchy: **root → clusters → workers**. Applications are described via SLA (Service Level Agreement) JSON documents that encode resource constraints, placement preferences, and networking requirements. The scheduler places each microservice on the best-fit worker using a pluggable algorithm (default: `cpumemfit.Scheduler`).

Current develop version: see `version.txt`.

---

## Repository Layout

```
oakestra/
├── root_orchestrator/          # Root-level services (docker-compose)
│   ├── docker-compose.yml
│   ├── override-*.yml          # Composable overrides (network, addons, etc.)
│   ├── system-manager-python/ # Python/Flask – main root API
│   └── service-manager/       # Python/Flask – root service manager (network plugin)
├── cluster_orchestrator/       # Cluster-level services (docker-compose)
│   ├── docker-compose.yml
│   ├── override-*.yml
│   ├── cluster-manager/       # Python/Flask – cluster API + MQTT client
│   └── service-manager/       # Python/Flask – cluster service manager (network plugin)
├── scheduler/                  # Go – shared scheduler (used by both root & cluster)
├── resource-abstractor/        # Python/Flask – resource DB layer (used by both levels)
├── node_engine/                # Go – worker binary (NodeEngine)
├── net_manager/                # Go – worker overlay network daemon (NetManager)
├── libraries/                  # Shared Python packages (UV workspace members)
│   ├── oakestra-utils/         # Enums: statuses, scheduling states (package: oakestra-utils)
│   └── resource-abstractor-client/ # HTTP client for resource-abstractor (package: resource-abstractor-client)
├── addons_engine/              # Optional addons subsystem (root only)
├── addons_marketplace/         # Optional marketplace manager
├── csi/                        # Container Storage Interface driver
├── scripts/                    # Startup and install scripts
│   ├── StartOakestraFull.sh    # 1-DOC: root + cluster on one machine
│   ├── StartOakestraRoot.sh    # Root only
│   ├── StartOakestraCluster.sh # Cluster only
│   └── InstallOakestraWorker.sh# Worker binary installer
├── run-a-cluster/              # Compose-based 1-DOC deployment (1-DOC.yaml + shared config)
├── hack/                       # Platform-specific workarounds (e.g. rpi4b-mongo override)
└── SKILLS/
    └── troubleshoot-oakestra.md # AI troubleshooting skill (keep this in sync)
```

---

## Architecture

### Hierarchy

```
Root Orchestrator  (1 per deployment)
  └── Cluster Orchestrator  (1..N per root)
        └── Worker Node  (1..N per cluster)
```

### Root Orchestrator — services

| Container | Language | Ports | Role |
|---|---|---|---|
| `system_manager` | Python/Flask + eventlet | 10000 (REST), 50052 (gRPC) | Central API. Receives deployments, manages clusters, dispatches to root_scheduler. Cluster registration happens via gRPC (port 50052). JWT auth (RS256). Swagger at `/api/docs`. |
| `mongo` | MongoDB 8.0 | 10007 | Stores applications, services, clusters, workers, jobs. |
| `mongo_net` | MongoDB 8.0 | 10008 | Stores network/IP state for the root service manager. |
| `root_service_manager` | Python/Flask | 10099 | Network plugin — assigns service IPs, manages overlay. Source: `root_orchestrator/service-manager/`. See [Networking](#networking). |
| `root_scheduler` | Go | 10004 | Picks which cluster to place a job on. Reads from Redis queue (asynq). Calls resource-abstractor for available resources. |
| `root_resource_abstractor` | Python/Flask | 11011 | DB abstraction layer over mongo (port 10007). Exposes REST for reading/writing cluster and job resource data. |
| `jwt_generator` | Go | 10011 | Issues RS256 key pairs. system_manager fetches the public key on startup. |
| `root_redis` | Redis | 6379 (pw: `rootRedis`) | Job queue (asynq) for root_scheduler. |
| `grafana` | Grafana | 3000 | Dashboards. |
| `loki` | Loki | 3100 | Log aggregation. |
| `alloy` | Grafana Alloy 1.17.0 | 12345 (loopback) | Discovers Root containers, ships opted-in stdout/stderr to the Root-local Loki, and exposes its diagnostic UI locally. |
| `oakestra-frontend-container` | nginx | 80 | Dashboard SPA. Connects to system_manager via `API_ADDRESS` env var. |
| `root_addons_manager` | Python | 11101 | Manages installed addons (optional, disable with override-no-addons.yml). |
| `root_addons_monitor` | Python | — | Monitors running addon containers via docker socket. |
| `root_addons_dashboard` | Python | 11103 | Addons UI. |
| `marketplace_manager` | Python | 11102 | Addon marketplace catalogue. |

### Cluster Orchestrator — services

| Container | Language | Ports | Role |
|---|---|---|---|
| `cluster_manager` | Python/Flask + eventlet | 10100 (REST), 10101 | Registers with root via gRPC. Receives jobs from root, dispatches via MQTT to workers. Runs background job every 15s to push aggregated resource info to system_manager. |
| `cluster_mongo` | MongoDB 8.0 | 10107 | Cluster-level job and node state. |
| `cluster_mongo_net` | MongoDB 8.0 | 10108 | Cluster-level network state. |
| `cluster_service_manager` | Python/Flask | 10110 | Cluster-level network plugin. Talks to root_service_manager and workers via MQTT. Source: `cluster_orchestrator/service-manager/`. |
| `cluster_scheduler` | Go | 10105 | Picks which worker to place a job on. Same binary as root_scheduler; different env vars. |
| `cluster_resource_abstractor` | Python/Flask | 11012 | Same binary as root; points at cluster_mongo. |
| `cluster_redis` | Redis | 6479 (pw: `clusterRedis`) | Job queue for cluster_scheduler. |
| `mqtt` | Eclipse Mosquitto 2.0 | 10003 | Broker for cluster↔worker communication. Has a built-in healthcheck. |
| `cluster_addons_manager` | Python | 11201 | Cluster-level addons manager (optional, disable with override-no-addons.yml). |
| `cluster_addons_monitor` | Python | — | Monitors running addon containers via docker socket at cluster level. |
| `cluster_addons_dashboard` | Python | 11203 | Cluster addons UI. |
| `prometheus` | Prometheus | 10009 (→9090) | Scrapes cluster_manager metrics. |
| `cluster_grafana` | Grafana | 3001 | Cluster dashboards. |
| `cluster_loki` | Loki | 3101 | Cluster log aggregation. |
| `cluster_alloy` | Grafana Alloy 1.17.0 | 12346 (loopback) | Discovers Cluster containers, ships opted-in stdout/stderr to that Cluster's local Loki, and exposes its diagnostic UI locally. |

### Worker Node — binaries

| Binary | Repo | Role |
|---|---|---|
| `NodeEngine` | This repo (`node_engine/`) | Runs on Linux workers. Connects to cluster via MQTT (10003). Receives deployment commands, manages containers/unikernels/VMs. Exposes CLI: `NodeEngine status`, `NodeEngine conf`, `NodeEngine logs`, `NodeEngine stop`. |
| `NetManager` | This repo (`net_manager/`) | Handles P2P overlay networking between workers (port 50103). Paired with NodeEngine, which starts it as the `netmanager` systemd service. |

---

## Key Communication Flows

### Cluster Registration
`cluster_manager` → gRPC (port 50052) → `system_manager`
Two-step handshake (CS1/SC1, CS2/SC2). On success, cluster gets an ID and starts a 15s background loop to push resource aggregates.

### Job Deployment
1. User POSTs SLA to `system_manager` (REST :10000)
2. `system_manager` enqueues a `schedule:job` task into `root_redis` (asynq)
3. `root_scheduler` dequeues, queries `root_resource_abstractor` for available clusters, runs placement algorithm, calls back `system_manager` with chosen cluster ID
4. `system_manager` forwards to `cluster_manager` (REST :10100)
5. `cluster_manager` enqueues into `cluster_redis`; `cluster_scheduler` picks a worker
6. `cluster_manager` sends deployment command to worker's `NodeEngine` via MQTT

### Resource Aggregation
`cluster_manager` → every 15s → `system_manager` REST: pushes CPU/mem/node counts
`NodeEngine` → MQTT → `cluster_manager`: pushes per-worker resource metrics

---

## Networking

The overlay network has three parts: the **NetManager** daemon on every worker (`net_manager/`, Go), the **root service manager**
(`root_orchestrator/service-manager/`, Python/Flask) and the **cluster service manager** (`cluster_orchestrator/service-manager/`, Python/Flask).
It creates a semantic addressing space: an IP address not only names the destination but also encodes a balancing policy.

### Address space

| Range | Purpose |
|---|---|
| `10.16.0.0/12` | Container network (all instance IPs) |
| `10.30.0.0/16` | Service Virtual IPs (require proxy translation) |
| `10.19.1.0/24` | Default bridge subnetwork on each node |

Port `50103` is reserved and cannot be used by deployed services.

### NetManager internals (`net_manager/`)

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

### Root service manager internals (`root_orchestrator/service-manager/`)

- **`network/subnetwork_management.py`** — allocates Service VIPs from `10.30.0.0/16`; persists next-available IP in MongoDB
- **`network/routes_interests.py`** — tracks which clusters have interest in which services
- **`operations/`** — service, instance, and cluster lifecycle management
- **`blueprints/`** — Flask-Smorest blueprints exposing OpenAPI endpoints; Swagger UI at `/api/docs`
- Uses JWT (RS256) for auth; public key fetched from root orchestrator

### Cluster service manager internals (`cluster_orchestrator/service-manager/`)

- **`interfaces/mqtt_client.py`** — MQTT client (paho-mqtt) connecting to cluster broker
- **`interfaces/root_service_manager_requests.py`** — HTTP calls to root service manager
- **`operations/`** — handles instance deployment/undeployment, propagates routes down to nodes via MQTT

### Communication flow

```
NodeEngine ──HTTP──> NetManager (server/)
NetManager ──MQTT──> cluster_service_manager ──HTTP──> root_service_manager
root_service_manager ──SocketIO──> cluster_service_manager (route push)
cluster_service_manager ──MQTT──> NetManager (table updates)
```

### NetManager configuration

NetManager config at `/etc/netmanager/netcfg.json`:
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

### Service manager environment variables

#### Root service manager

| Variable | Default | Purpose |
|---|---|---|
| `ROOT_MONGO_URL` | — | MongoDB host |
| `ROOT_MONGO_PORT` | — | MongoDB port (typically `10008`) |
| `JWT_GENERATOR_URL` | `localhost` | jwt_generator host (for RS256 public key) |
| `JWT_GENERATOR_PORT` | `10011` | jwt_generator port |
| `MY_PORT` | `10100` | Port this service listens on |
| `GATEWAY_ENABLED` | unset | Set by `override-gateway.yml`. Cluster calls then carry the cluster's verified client cert (`X-Client-Cert`), and `net_deploy_status` only accepts instances placed on the calling cluster |
| `ROOT_CA_FILE` | — | Root CA bundle; behind the gateway, a cluster cert must be signed directly by it |

#### Cluster service manager

| Variable | Default | Purpose |
|---|---|---|
| `CLUSTER_MONGO_URL` | — | MongoDB host |
| `CLUSTER_MONGO_PORT` | — | MongoDB port (typically `10108`) |
| `MQTT_BROKER_URL` | — | MQTT broker host |
| `MQTT_BROKER_PORT` | — | MQTT broker port (typically `10003`) |
| `ROOT_SERVICE_MANAGER_URL` | `0.0.0.0` | root service manager host |
| `ROOT_SERVICE_MANAGER_PORT` | `5000` | root service manager port |
| `MQTT_CERT` | — | Path to TLS certs dir (only when using MQTT auth override) |
| `MY_PORT` | `10200` | Port this service listens on |
| `GATEWAY_ENABLED` | unset | Set by `override-gateway.yml`. Calls to the root then use mTLS (`CLUSTER_CERT_FILE`, `CLUSTER_KEY_FILE`, `ROOT_CA_FILE`) and MQTT uses TLS (`MQTT_CERT`). The service won't start if any of these is missing |

---

## Deployment Modes and Compose Overrides

**Startup scripts** in `scripts/` auto-detect IPs, validate Docker versions, download config files from GitHub, and support version pinning via `OAKESTRA_VERSION` (branch triggers source build; tag like `v0.4.401` uses pre-built images).

**Override files** are composable. Set `OVERRIDE_FILES` before running a startup script:
```bash
export OVERRIDE_FILES="override-no-addons.yml,override-network-host.yml"
```

| Override | Effect |
|---|---|
| `override-network-host.yml` | All containers use host network. Container DNS names no longer work — env vars must use IPs, not container names. |
| `override-ipv6-enabled.yml` | Enables IPv6 on the oakestra Docker network. |
| `override-no-network.yml` | Disables the network plugin (root/cluster service managers). |
| `override-no-addons.yml` | Removes addons subsystem from root. |
| `override-no-dashboard.yml` | Removes frontend dashboard from root. |
| `override-no-observe.yml` | Removes Grafana/Loki/Alloy/Prometheus. |
| `override-mosquitto-auth.yml` | Enables MQTT authentication (workers need credentials). |
| `override-images-only.yml` | Forces pre-built images, skips local builds. |

The service managers are built from source like every other component (`docker compose up --build`), so there is no separate override to use a local or pinned network plugin.

---

## Tech Stack Summary

| Layer | Language/Framework | Key deps |
|---|---|---|
| system_manager | Python 3.10, Flask, flask-smorest, flask-socketio, eventlet | grpc, pymongo, flask-jwt-extended |
| cluster_manager | Python 3.10, Flask, flask-smorest, flask-socketio, eventlet | grpc, paho-mqtt, apscheduler, prometheus_client |
| resource-abstractor | Python 3.10, Flask | pymongo, resource_abstractor_client (internal lib) |
| root/cluster scheduler | Go 1.24 | gin, asynq (Redis-backed task queue) |
| NodeEngine | Go | paho-mqtt, cobra CLI |
| NetManager | Go | wireguard-go `tun`, paho-mqtt, cobra CLI |
| root/cluster service manager | Python 3.10, Flask, flask-smorest | pymongo, paho-mqtt (cluster), flask-socketio |
| Databases | MongoDB 8.0, Redis | — |
| Messaging | Eclipse Mosquitto 2.0 (MQTT) | — |
| Observability | Grafana, Loki 2.9.2, Alloy 1.17.0, Prometheus | — |
| Python logging | structlog 26.1.0 + standard-library bridge | JSON schema v1, stdout only |

---

## Python Dependency Management: UV vs. Legacy

| Component                    | Status               | Dependency Manager                                           | Local Env / Build Notes                                                                                              |
|------------------------------|----------------------|--------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------|
| `system-manager-python`      | **Migrated to `uv`** | UV workspace member (`pyproject.toml`)                       | Docker build context is repo root (`../`). Managed via `uv sync` (with `--all-packages` flag if run from repo root). |
| `cluster-manager`            | **Migrated to `uv`** | UV workspace member (`pyproject.toml`)                       | Docker build context is repo root (`../`). Managed via `uv sync` (with `--all-packages` flag if run from repo root). |
| `oakestra-utils`             | **Migrated to `uv`** | UV workspace member (`libraries/oakestra-utils`)             | Installed into `.venv` in editable mode via `uv sync` (with `--all-packages` flag if run from repo root).            |
| `resource-abstractor-client` | **Migrated to `uv`** | UV workspace member (`libraries/resource-abstractor-client`) | Installed into `.venv` in editable mode via `uv sync` (with `--all-packages` flag if run from repo root).            |
| `resource-abstractor`        | *Not yet migrated*   | `requirements.txt` (pip)                                     | Isolated build context. Requires dedicated subfolder `.venv` for local uncontainerized dev.                          |
| `jwt-generator`              | *Not yet migrated*   | `requirements.txt` (pip)                                     | Isolated build context. Requires dedicated subfolder `.venv` for local uncontainerized dev.                          |
| `addons-manager`             | *Not yet migrated*   | `requirements.txt` (pip)                                     | Isolated build context. Requires dedicated subfolder `.venv` for local uncontainerized dev.                          |
| `addons-monitor`             | *Not yet migrated*   | `requirements.txt` (pip)                                     | Isolated build context. Requires dedicated subfolder `.venv` for local uncontainerized dev.                          |
| `marketplace-manager`        | *Not yet migrated*   | `requirements.txt` (pip)                                     | Isolated build context. Requires dedicated subfolder `.venv` for local uncontainerized dev.                          |
| `root service-manager`       | *Not yet migrated*   | `requirements.txt` (pip)                                     | Isolated build context. Requires dedicated subfolder `.venv` for local uncontainerized dev.                          |
| `cluster service-manager`    | *Not yet migrated*   | `requirements.txt` (pip)                                     | Isolated build context. Requires dedicated subfolder `.venv` for local uncontainerized dev.                          |

> [!IMPORTANT]
> **CI Pipelines and Docker Container Builds** build each service in an isolated environment and are unaffected by mixed dependency managers.
> **Local Uncontainerized Development**: Running `uv sync --all-packages` manages packages strictly according to `uv.lock` and will uninstall untracked pip packages from the root `.venv`. When running/debugging non-uv modules locally without Docker, **always** use dedicated secondary virtual environments in their respective subdirectories (e.g., `resource-abstractor/.venv`). Never run `pip install -r requirements.txt` into the root `.venv`.

---

## Development Commands

### Python services

```bash
# Sync all uv workspace services (system_manager, cluster_manager, shared libraries)
uv sync --all-packages

# Run tests for system_manager
uv run --with pytest pytest root_orchestrator/system-manager-python/src/system_manager/tests/

# Run tests for resource-abstractor (non-uv, uses its local venv / pip)
pytest resource-abstractor/tests/

# Run tests for the network service managers (non-uv, use their local venvs / pip)
(cd root_orchestrator/service-manager && pytest)
(cd cluster_orchestrator/service-manager && pytest)

# Lint (ruff is configured in pyproject.toml at repo root; line-length=100)
ruff check .
```

### Go services (scheduler, NodeEngine, NetManager)

```bash
# Build the scheduler
cd scheduler && go build ./...

# Build NodeEngine
cd node_engine && go build -o NodeEngine .

# Build and install NetManager (bin/amd64-NetManager, bin/arm64-NetManager)
cd net_manager/build && ./build.sh && ./install.sh amd64

# Run Go tests (each Go module separately: scheduler/, node_engine/, net_manager/, csi/hostpath/)
go test ./...
cd net_manager && go test -race ./...
```

Note: the Go module names were kept when the folders moved. `node_engine/` is still module `go_node_engine` and
`net_manager/` is module `NetManager`, which is why `-ldflags` use `go_node_engine/cmd.Version` and `NetManager/cmd.Version`.

### Local stack (full root + cluster on one machine)

```bash
# Start everything (detects IPs, pulls/builds images)
./scripts/StartOakestraFull.sh

# Start root only
./scripts/StartOakestraRoot.sh

# Start cluster only (set SYSTEM_MANAGER_URL first)
export SYSTEM_MANAGER_URL=<root-ip>
./scripts/StartOakestraCluster.sh

# Pin to a specific version
export OAKESTRA_VERSION=v0.4.411
./scripts/StartOakestraFull.sh

# Build from source branch
export OAKESTRA_VERSION=develop
./scripts/StartOakestraFull.sh
```

---

## Shared Libraries

- `libraries/oakestra-utils` — Python enums for job statuses (`DeploymentStatus`, `PositiveSchedulingStatus`, `NegativeSchedulingStatus`). Package name: `oakestra-utils`, module: `oakestra_utils`.
- `libraries/resource-abstractor-client` — Python HTTP client for resource-abstractor. Package name: `resource-abstractor-client`, module: `resource_abstractor_client`.
- `libraries/oakestra_logging` — Structured JSON logging configuration, schema, and Gunicorn logger for Python services.

**These libraries are workspace members in the root `pyproject.toml`.**
- **Docker builds** — `system_manager` and `cluster_manager` use the repository root as the build context and install workspace packages with `uv sync`. Other consumers use the local `libraries/` BuildKit context, configured in Compose and CI.
- **Local dev / tests** — Running `uv sync --all-packages` installs the workspace packages into the root `.venv` in editable mode.

---

## Gotchas

- **Host networking breaks container DNS.** When using `override-network-host.yml`, containers can't resolve each other by name — set all env vars to IPs, not container names.
- **Docker build contexts for UV services.** `system-manager-python` and `cluster-manager` require the repository root build context (`context: ../` in `docker-compose.yml`) so that the uv workspace root (`pyproject.toml`, `uv.lock`, `libraries/`) is accessible during image builds.
- **Local venv isolation.** Do not mix `pip install -r requirements.txt` for non-uv services inside the root `.venv`, as subsequent `uv sync [--all-packages]` invocations will clean untracked packages. Use a subfolder virtual environment for non-uv services when running outside of Docker.
- **Structured Python logs.** The JSON shape is fixed in `libraries/oakestra_logging`. Use `LOG_LEVEL` only for verbosity and `OAKESTRA_SERVICE_NAME` for the runtime role. Do not log complete request bodies, credentials, users, SLAs, MQTT payloads, or database records.
- **eventlet monkey-patching.** Both `system_manager` and `cluster_manager` use eventlet. Monkey-patching must happen before Flask/pymongo imports — don't reorder the top of entry-point files.
- **Scheduler is the same binary for root and cluster** — differentiated only by env vars (`SCHEDULER_TYPE`, Redis URL/password). Keep deployment-specific logic out of the binary.

---

## Design Principles (keep these in mind when contributing)

1. **Lightweight first.** The whole orchestrator stack targets ~1 GB RAM. Avoid pulling in heavy dependencies or adding services without strong justification.
2. **Two-level hierarchy is intentional.** Root handles multi-cluster global placement; cluster handles per-worker placement. Keep concerns separated — root doesn't talk to workers directly, cluster doesn't bypass root for scheduling decisions.
3. **Pluggable scheduling.** The scheduler binary is shared between root and cluster, differing only via env vars. New algorithms should implement the `placement.Algorithm` interface in `scheduler/calculate/schedulers/placement/`.
4. **Stateless services, stateful DBs.** Services (schedulers, managers) are designed to restart cleanly. State lives in MongoDB and Redis. Don't add in-process state that survives restarts without an explicit persistence story.
5. **Override-based configuration.** Don't bake deployment-specific config into docker-compose.yml. Add a new `override-*.yml` file instead.
6. **The network plugin lives here.** The former `oakestra-net` repo was merged into this one. Network changes go to `root_orchestrator/service-manager/`, `cluster_orchestrator/service-manager/` and `net_manager/`, and ship in the same release as the rest of the platform.

---

## Updating the Troubleshooting Skill

The troubleshooting skill lives at `SKILLS/troubleshoot-oakestra.md`. It is used by an AI agent to diagnose a live Oakestra deployment. **Keep it in sync with architecture changes.**

### When to update the skill

| Type of change | What to update in the skill |
|---|---|
| **New service added** to a docker-compose | Add the container name to the expected-containers list in STEP 2. Add its `/status` endpoint to STEP 11 (API smoke tests). Add relevant log patterns to STEP 14. |
| **Service removed** | Remove it from expected-containers list, smoke tests, and any fix steps that reference it. |
| **Port changed** | Update STEP 1.4 (port availability check), STEP 11 (smoke tests), the firewall table in STEP 8, and STEP 7.2 (internal connectivity). |
| **New env var** required | Add it to STEP 3 (env var validation) with an explanation of what breaks if it's missing. |
| **New override file added** | Document its effect in STEP 7.3 (network mode check) or wherever it's most relevant. |
| **New database collection** used for important state | Add a query for it in STEP 4 (MongoDB diagnostics). |
| **Redis password or port changed** | Update STEP 5 (Redis diagnostics). |
| **MQTT auth behavior changed** | Update STEP 6 (MQTT diagnostics). |
| **New startup script or changed startup flow** | Update STEP 0 (deployment mode detection) and STEP 13 (image/build issues). |
| **Worker binary changed** (new flags, new config path) | Update STEP 9 (worker node diagnostics). |
| **New common failure mode discovered** | Add a row to the error-patterns table in STEP 2.3 and a fix in STEP 15. |
| **GPU or hardware support changed** | Update STEP 9.6 (GPU support). |

### How to update the skill

The skill is plain markdown. Each STEP is self-contained. Make targeted edits:

- Keep bash commands copy-pasteable and tested.
- When adding new checks, follow the existing pattern: show the command, explain what to look for, describe what it means.
- Keep the error-pattern table in STEP 2.3 up to date — it is the fastest triage tool.
- The diagnosis report template in STEP 16 should reflect the current set of components.

### What the skill does NOT need to track

- Implementation details (function names, internal algorithms) — those belong in code comments.
- Temporary debugging one-liners — only promote something to the skill if it is reusable across deployments.
- Anything already obvious from `docker ps` output.
