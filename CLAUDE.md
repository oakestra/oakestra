# Oakestra — Developer Context for AI Assistants

## What Oakestra Is

Oakestra is a **lightweight orchestration platform for edge computing**. Unlike Kubernetes or K3s, which were designed for cloud-grade machines, Oakestra targets heterogeneous, resource-constrained edge devices. The entire root + cluster stack runs in ~1 GB RAM. A worker node needs only 100 MB RAM and 50 MB disk.

The platform orchestrates containerised workloads (Docker, containerd) and unikernels across a two-level hierarchy: **root → clusters → workers**. Applications are described via SLA (Service Level Agreement) JSON documents that encode resource constraints, placement preferences, and networking requirements. The scheduler places each microservice on the best-fit worker using a pluggable algorithm (default: `rootBestCpuMemFit`).

Current develop version: see `version.txt`.

---

## Repository Layout

```
oakestra/
├── root_orchestrator/          # Root-level services (docker-compose)
│   ├── docker-compose.yml
│   ├── override-*.yml          # Composable overrides (network, addons, etc.)
│   └── system-manager-python/ # Python/Flask – main root API
├── cluster_orchestrator/       # Cluster-level services (docker-compose)
│   ├── docker-compose.yml
│   ├── override-*.yml
│   └── cluster-manager/       # Python/Flask – cluster API + MQTT client
├── scheduler/                  # Go – shared scheduler (used by both root & cluster)
├── resource-abstractor/        # Python/Flask – resource DB layer (used by both levels)
├── go_node_engine/             # Go – worker binary (NodeEngine)
├── libraries/                  # Shared Python packages
│   ├── oakestra_utils_library/ # Enums: statuses, scheduling states
│   └── resource_abstractor_client/ # HTTP client for resource-abstractor
├── addons_engine/              # Optional addons subsystem (root only)
├── addons_marketplace/         # Optional marketplace manager
├── csi/                        # Container Storage Interface driver
├── scripts/                    # Startup and install scripts
│   ├── StartOakestraFull.sh    # 1-DOC: root + cluster on one machine
│   ├── StartOakestraRoot.sh    # Root only
│   ├── StartOakestraCluster.sh # Cluster only
│   └── InstallOakestraWorker.sh# Worker binary installer
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
| `root_service_manager` | Go (oakestra-net repo) | 10099 | Network plugin — assigns service IPs, manages overlay. External repo: `github.com/oakestra/oakestra-net`. |
| `root_scheduler` | Go | 10004 | Picks which cluster to place a job on. Reads from Redis queue (asynq). Calls resource-abstractor for available resources. |
| `root_resource_abstractor` | Python/Flask | 11011 | DB abstraction layer over mongo (port 10007). Exposes REST for reading/writing cluster and job resource data. |
| `jwt_generator` | Go | 10011 | Issues RS256 key pairs. system_manager fetches the public key on startup. |
| `root_redis` | Redis | 6379 (pw: `rootRedis`) | Job queue (asynq) for root_scheduler. |
| `grafana` | Grafana | 3000 | Dashboards. |
| `loki` | Loki | 3100 | Log aggregation. |
| `promtail` | Promtail | — | Ships Docker container logs to Loki via docker socket. |
| `oakestra-frontend-container` | nginx | 80 | Dashboard SPA. Connects to system_manager via `API_ADDRESS` env var. |
| `addons_manager` | Python | 11101 | Manages installed addons (optional, disable with override-no-addons.yml). |
| `addons_monitor` | Python | — | Monitors running addon containers via docker socket. |
| `addons_dashboard` | Python | 11103 | Addons UI. |
| `marketplace_manager` | Python | 11102 | Addon marketplace catalogue. |

### Cluster Orchestrator — services

| Container | Language | Ports | Role |
|---|---|---|---|
| `cluster_manager` | Python/Flask + eventlet | 10100 (REST), 10101 | Registers with root via gRPC. Receives jobs from root, dispatches via MQTT to workers. Runs background job every 15s to push aggregated resource info to system_manager. |
| `cluster_mongo` | MongoDB 8.0 | 10107 | Cluster-level job and node state. |
| `cluster_mongo_net` | MongoDB 8.0 | 10108 | Cluster-level network state. |
| `cluster_service_manager` | Go (oakestra-net repo) | 10110 | Cluster-level network plugin. Talks to root_service_manager and workers via MQTT. |
| `cluster_scheduler` | Go | 10105 | Picks which worker to place a job on. Same binary as root_scheduler; different env vars. |
| `cluster_resource_abstractor` | Python/Flask | 11012 | Same binary as root; points at cluster_mongo. |
| `cluster_redis` | Redis | 6479 (pw: `clusterRedis`) | Job queue for cluster_scheduler. |
| `mqtt` | Eclipse Mosquitto 2.0 | 10003 | Broker for cluster↔worker communication. Has a built-in healthcheck. |
| `prometheus` | Prometheus | 10009 (→9090) | Scrapes cluster_manager metrics. |
| `cluster_grafana` | Grafana | 3001 | Cluster dashboards. |
| `cluster_loki` | Loki | 3101 | Cluster log aggregation. |
| `cluster_promtail` | Promtail | — | Ships Docker logs to cluster_loki. |

### Worker Node — binaries

| Binary | Repo | Role |
|---|---|---|
| `NodeEngine` | This repo (`go_node_engine/`) | Runs on Linux workers. Connects to cluster via MQTT (10003). Receives deployment commands, manages containers/unikernels/VMs. Exposes CLI: `NodeEngine status`, `NodeEngine conf`, `NodeEngine logs`, `NodeEngine stop`. |
| `NetManager` | `github.com/oakestra/oakestra-net` | Handles P2P overlay networking between workers (port 50103). Paired with NodeEngine. |

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
| `override-no-observe.yml` | Removes Grafana/Loki/Promtail/Prometheus. |
| `override-mosquitto-auth.yml` | Enables MQTT authentication (workers need credentials). |
| `override-images-only.yml` | Forces pre-built images, skips local builds. |
| `override-local-service-manager.yml` | Builds service manager from local source. |

---

## Tech Stack Summary

| Layer | Language/Framework | Key deps |
|---|---|---|
| system_manager | Python 3, Flask, flask-smorest, flask-socketio, eventlet | grpc, pymongo, flask-jwt-extended |
| cluster_manager | Python 3, Flask, flask-smorest, flask-socketio, eventlet | grpc, paho-mqtt, apscheduler, prometheus_client |
| resource-abstractor | Python 3, Flask | pymongo, resource_abstractor_client (internal lib) |
| root/cluster scheduler | Go 1.24 | gin, asynq (Redis-backed task queue) |
| NodeEngine | Go | paho-mqtt, cobra CLI |
| Databases | MongoDB 8.0, Redis | — |
| Messaging | Eclipse Mosquitto 2.0 (MQTT) | — |
| Networking | oakestra-net (external Go repo) | — |
| Observability | Grafana, Loki 2.9.2, Promtail 2.9.2, Prometheus | — |

---

## Shared Libraries

- `libraries/oakestra_utils_library` — Python enums for job statuses (`DeploymentStatus`, `PositiveSchedulingStatus`, `NegativeSchedulingStatus`). Imported by system_manager and cluster_manager. Branch used during build is controlled by the `LIB_BRANCH` build arg (defaults to `develop`).
- `libraries/resource_abstractor_client` — Python HTTP client for resource-abstractor. Reads `RESOURCE_ABSTRACTOR_URL` and `RESOURCE_ABSTRACTOR_PORT` from env.

---

## Design Principles (keep these in mind when contributing)

1. **Lightweight first.** The whole orchestrator stack targets ~1 GB RAM. Avoid pulling in heavy dependencies or adding services without strong justification.
2. **Two-level hierarchy is intentional.** Root handles multi-cluster global placement; cluster handles per-worker placement. Keep concerns separated — root doesn't talk to workers directly, cluster doesn't bypass root for scheduling decisions.
3. **Pluggable scheduling.** The scheduler binary is shared between root and cluster, differing only via env vars. New algorithms should implement the `interfaces.Algorithm` interface in `scheduler/calculate/schedulers/`.
4. **Stateless services, stateful DBs.** Services (schedulers, managers) are designed to restart cleanly. State lives in MongoDB and Redis. Don't add in-process state that survives restarts without an explicit persistence story.
5. **Override-based configuration.** Don't bake deployment-specific config into docker-compose.yml. Add a new `override-*.yml` file instead.
6. **The network plugin is external.** `oakestra-net` (`root_service_manager`, `cluster_service_manager`) is a separate repository. Changes to network behavior need PRs there, not here.

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
