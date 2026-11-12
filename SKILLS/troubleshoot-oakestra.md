# Oakestra Troubleshooting Skill

You are a senior Oakestra engineer with deep knowledge of the platform. Your task is to perform a thorough diagnosis of an Oakestra deployment, identify all issues, fix what can be fixed locally, and prepare a structured bug report for anything that requires upstream attention.

Oakestra is an edge computing orchestration platform composed of:
- **Root Orchestrator** – manages multiple clusters (docker-compose in `root_orchestrator/`)
- **Cluster Orchestrator** – manages multiple worker nodes (docker-compose in `cluster_orchestrator/`)
- **Worker Node** – runs workloads (`NodeEngine` + `NetManager` binaries)

Deployments can be:
- **1-DOC** (single machine): root + cluster on the same host, started via `scripts/StartOakestraFull.sh`
- **Root only**: started via `scripts/StartOakestraRoot.sh`
- **Cluster only**: started via `scripts/StartOakestraCluster.sh` (requires SYSTEM_MANAGER_URL pointing to root)
- **Worker only**: NodeEngine + NetManager binaries installed via `scripts/InstallOakestraWorker.sh`

---

## STEP 0 — Detect Deployment Mode

Run the following and interpret the output to understand what is running on this machine:

```bash
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null
systemctl status NodeEngine 2>/dev/null || true
which NodeEngine 2>/dev/null || true
which NetManager 2>/dev/null || true
```

Classify:
- If containers named `system_manager`, `mongo`, `root_scheduler`, `root_resource_abstractor` etc. are present → **Root Orchestrator is here**
- If containers named `cluster_manager`, `cluster_mongo`, `mqtt`, `cluster_scheduler` etc. are present → **Cluster Orchestrator is here**
- If `NodeEngine` binary or systemd service exists → **Worker Node is here**
- Multiple groups present → **1-DOC or mixed deployment**

Note the deployment mode and proceed through the relevant sections.

---

## STEP 1 — System Prerequisites

### 1.1 Docker Version

```bash
docker --version
docker compose version 2>/dev/null || docker-compose --version 2>/dev/null
```

**Requirements:**
- Docker Engine ≥ 20.10 (24+ recommended)
- Docker Compose plugin v2+ (i.e., `docker compose`, not `docker-compose`)

**Fix if outdated:**
```bash
# Check official Docker install docs for your distro
# Quick check on Ubuntu/Debian:
curl -fsSL https://get.docker.com | sh
```

If `docker-compose` (v1) is being used instead of `docker compose` (v2 plugin), the startup scripts may behave incorrectly. Advise user to install the compose plugin.

### 1.2 User Permissions

```bash
groups | grep docker || echo "WARNING: current user not in docker group"
```

If not in the `docker` group, commands need `sudo` and env vars may not propagate correctly. Fix:
```bash
sudo usermod -aG docker $USER
# Then re-login or: newgrp docker
```

### 1.3 Available Resources

```bash
free -h
df -h /
nproc
```

Flag if: RAM < 4GB free, disk < 10GB free, or CPUs < 2.

### 1.4 Required Ports Availability

Check that no foreign process is occupying Oakestra's ports before containers start.

**Root Orchestrator ports:**
```bash
for port in 10000 50052 10099 10004 80 3000 3100 12345 11101 11102; do
  ss -tlnp "sport = :$port" 2>/dev/null | grep -v "State" | head -1 && echo "  ^ port $port" || true
done
```

**Cluster Orchestrator ports:**
```bash
for port in 10003 10110 10100 10101 10105 10009 3001 3101 12346; do
  ss -tlnp "sport = :$port" 2>/dev/null | grep -v "State" | head -1 && echo "  ^ port $port" || true
done
```

Flag any port occupied by a non-Oakestra process. Common conflict: port 80 occupied by nginx/apache, port 3000 by another Grafana.

Mongo (10007, 10008, 10107, 10108), Redis (6379, 6479) and the resource abstractors (11011, 11012) are internal only: they are not published on the host, so do not check them with `ss` or host-side clients. Use `docker exec` (STEP 4, STEP 5, STEP 11). Exception: with `override-network-host.yml` they listen on the host and the port check applies to them too.

---

## STEP 2 — Docker Container Health (Orchestrators)

### 2.1 Container Status Overview

```bash
docker ps -a --format "table {{.Names}}\t{{.Status}}\t{{.RunningFor}}\t{{.Ports}}"
```

For each container, check:
- `Up X seconds` → may indicate a restart loop; compare with uptime of other containers
- `Exited (1)` or `Exited (2)` → crashed; read logs immediately
- `Restarting` → crash loop; read logs
- `Created` (never started) → dependency failed to start

**Expected containers for Root Orchestrator:**
`system_manager`, `mongo`, `mongo_net`, `root_service_manager`, `root_redis`, `root_scheduler`, `root_resource_abstractor`, `jwt_generator`, `grafana`, `loki`, `alloy`, `oakestra-frontend-container`

Optional root containers (if addons enabled):
`root_addons_manager` (Go, also starts and watches the addon containers), `marketplace_manager` (Go, healthcheck `/marketplace healthcheck`)

**Expected containers for Cluster Orchestrator:**
`mqtt`, `cluster_mongo`, `cluster_mongo_net`, `cluster_service_manager`, `cluster_manager`, `cluster_scheduler`, `cluster_resource_abstractor`, `cluster_redis`, `prometheus`, `cluster_grafana`, `cluster_loki`, `cluster_alloy`

Optional cluster containers (if addons enabled):
`cluster_addons_manager`

The addons managers keep their grant signing key in a named volume of their own (`addons_state` on root, `cluster_addons_state` on the cluster). If that volume was removed or recreated, installed addons fail with `grant signature invalid` (see the error table and STEP 12).

### 2.2 Restart Loop Detection

```bash
docker ps --format "{{.Names}}\t{{.Status}}" | grep -E "Restarting|second"
```

For any container in a restart loop:
```bash
docker inspect <container_name> --format '{{.RestartCount}} restarts, last exit: {{.State.FinishedAt}}, exit code: {{.State.ExitCode}}'
```

### 2.3 Container Logs — Errors and Warnings

Pull logs for ALL running or recently exited containers. Focus on errors, tracebacks, connection refused, timeouts, and authentication failures:

```bash
# Get logs for all oakestra containers (last 200 lines each)
for name in $(docker ps -a --format "{{.Names}}" | grep -E "system_manager|mongo|root_service_manager|root_redis|root_scheduler|root_resource_abstractor|jwt_generator|cluster_manager|cluster_service_manager|cluster_mongo|cluster_redis|cluster_scheduler|cluster_resource_abstractor|mqtt|addons|marketplace|alloy|loki|grafana"); do
  echo "===== LOGS: $name ====="
  docker logs --tail 100 "$name" 2>&1 | grep -iE "error|exception|traceback|failed|refused|timeout|fatal|panic|warn" || echo "(no errors in last 100 lines)"
done
```

For containers that exited:
```bash
docker logs <exited_container_name> 2>&1 | tail -50
```

**Common error patterns and what they mean:**

| Pattern | Likely Cause |
|---|---|
| `Connection refused` to mongo/redis | DB container not ready yet, or wrong URL env var |
| `authentication failed` (MongoDB) | Unexpected auth enabled on MongoDB |
| `MQTT connection refused` | `mqtt` container not healthy or wrong port |
| `dial tcp ... connection refused` | Dependency not ready, check startup order |
| `no such host` | Wrong container name in env var (DNS not resolving) |
| `timeout` on root_scheduler/cluster_scheduler | Redis not accessible, check REDIS_ADDR env var |
| `JWT` errors in system_manager | jwt_generator container not started |
| Addons or marketplace admin endpoints answer 503 | The JWT public key is not loaded. Check `JWT_GENERATOR_URL`/`JWT_GENERATOR_PORT` (root), or `JWT_PUBLIC_KEY_URL` (cluster plane, must point at `http://<root>:11102/api/v1/marketplace/auth/public-key`), and that jwt_generator / marketplace_manager are up (STEP 12) |
| Addon status `failed` with a permissions or policy message in `status_details` | The definition uses a capability it did not declare (host path, host port, docker socket, host/core network) or something that can never be granted. Fix the definition, not the Mongo record: the engine re-checks the policy before it starts any container |
| Addon `failed` with `grant signature invalid; re-approve the addon` (or `missing`) | The record's signature doesn't match. Usual cause: the `addons_state` / `cluster_addons_state` volume was lost or replaced, so the key changed. Fix: `POST /api/v1/addons/<id>/approve` with the permissions hash (admin token), once per addon. If the volume was never touched, treat it as a tampered database record and a security incident: look at who can write to Mongo before approving anything |
| Addon `failed` with `volume ...` (`exists but is not owned by this addon` / `exists with a different driver or driver options`) | A Docker volume with the addon's derived name already exists and wasn't created by this addon, or has other driver options. Inspect it with `docker volume inspect`; remove it if it is a leftover and holds nothing you need, then let the reconcile loop retry |
| Addon `failed` with `image ... carries the reserved label` | The image itself sets an `oak.*`, `oakestra.*`, `org.oakestra.*` or `com.docker.compose.*` label. This can't be granted; the addon image has to be rebuilt without it |
| Install/update/approve answers 409 with `conflicts with host path of addon X` (or the addon fails with it) | Its host bind sources equal, contain or lie below those of another non-disabled addon and one side is read-write, or an update mounts a path below one the previous version had read-write. Change the paths in the definition or remove the other addon |
| Addon `failed` with `the installed definition does not pass the current permission policy` | The stored definition violates the policy that applies now (policy tightened, or the record was edited). Re-approving doesn't help: update to a version that declares what it needs (`host_root`, exact `host_paths`, ...) or remove the addon |
| Tap source shows `last_error` | Sync failed (https only, blocked private address, body over 5 MiB, serial lower than the last good one, invalid index). The last good snapshot stays in use (STEP 12) |
| `CLUSTER_NAME not set` | Missing env var in cluster startup |
| `SYSTEM_MANAGER_URL not set` | Cluster started without root URL |
| `CLUSTER_ADDRESS env var is not set` (cluster_manager log) | Missing env var — cluster cannot advertise its address to root |
| `cluster_address is required` (system_manager log) | Cluster connected via gRPC but sent empty `cluster_address` |
| `Cluster reachability probe failed` / `cluster not reachable at` (system_manager log) | Root cannot reach `http://CLUSTER_ADDRESS:10100/api/cluster/status` — wrong IP, firewall, or cluster_manager not up |
| HTTP 502 on `/addon-assets/<name>/...` (dashboard) | The addon UI container is not running, or is not on the `oakestra` network with the alias `addonui-<name>` - see STEP 12 |
| `permission denied` opening `/var/run/docker.sock` (Alloy log) | Alloy cannot discover/read container logs; inspect the socket mount and host permissions |
| `loki.write` connection refused | The local Loki is not ready, or `LOKI_URL` is wrong for the selected network mode |

---

## STEP 3 — Environment Variables Validation

Check that critical env vars were correctly injected into each container:

```bash
# Root Orchestrator critical vars
docker exec system_manager env 2>/dev/null | grep -E "ROOT_MONGO|ROOT_SCHEDULER|RESOURCE_ABSTRACTOR|NET_PLUGIN|JWT" || echo "system_manager not running"

docker exec root_scheduler env 2>/dev/null | grep -E "MANAGER_URL|RESOURCE_ABSTRACTOR|REDIS_ADDR" || echo "root_scheduler not running"

docker exec root_resource_abstractor env 2>/dev/null | grep -E "MONGO_URL|MONGO_PORT" || echo "root_resource_abstractor not running"

docker exec root_service_manager env 2>/dev/null | grep -E "SYSTEM_MANAGER|ROOT_MONGO|JWT" || echo "root_service_manager not running"

# oaekstra-frontend-container in 1-DOC
docker exec oakestra-frontend-container env 2>/dev/null | grep -E "API_ADDRESS|ADDONS_MANAGER_ADDRESS|MARKETPLACE_ADDRESS|PRESET_URL" || echo "dashboard not running"

# Addons engine (Go). Distroless images have no shell, so read the env from the container config.
docker inspect root_addons_manager --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null | grep -E "ADDONS_|ADDON_|MARKETPLACE_ADDR|ORCHESTRATION_PLANE|JWT_" || echo "root_addons_manager not running"
docker inspect marketplace_manager --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null | grep -E "ADDON_MARKETPLACE|MARKETPLACE_|JWT_" || echo "marketplace_manager not running"
```

```bash
# Cluster Orchestrator critical vars
docker exec cluster_manager env 2>/dev/null | grep -E "SYSTEM_MANAGER_URL|CLUSTER_NAME|CLUSTER_LOCATION|CLUSTER_ADDRESS|MQTT|CLUSTER_MONGO|CLUSTER_SCHEDULER|RESOURCE_ABSTRACTOR" || echo "cluster_manager not running"

docker exec cluster_scheduler env 2>/dev/null | grep -E "MANAGER_URL|RESOURCE_ABSTRACTOR|REDIS_ADDR" || echo "cluster_scheduler not running"

docker exec cluster_service_manager env 2>/dev/null | grep -E "ROOT_SERVICE_MANAGER_URL|MQTT|CLUSTER_MONGO|SYSTEM_MANAGER" || echo "cluster_service_manager not running"

docker inspect cluster_addons_manager --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null | grep -E "ADDONS_|ADDON_|MARKETPLACE_ADDR|ORCHESTRATION_PLANE|JWT_" || echo "cluster_addons_manager not running"
```

**Key validations:**
- `SYSTEM_MANAGER_URL` in cluster containers must be the **root machine's IP or hostname**, not `localhost` or `127.0.0.1` (unless 1-DOC)
- `CLUSTER_NAME` and `CLUSTER_LOCATION` must be non-empty in cluster_manager
- `CLUSTER_ADDRESS` must be non-empty in cluster_manager and must be the **IP/hostname at which the ROOT can reach this cluster manager** (typically the cluster host's default-route IP). It must NOT be `localhost`, `127.0.0.1`, `0.0.0.0`, or a Docker-internal address like `172.17.x.x` / `172.19.x.x` (those are the docker bridge gateway and not routable from the root). In 1-DOC deployments, `CLUSTER_ADDRESS` should equal `SYSTEM_MANAGER_URL`.
- `REDIS_ADDR` must match `redis://:rootRedis@root_redis:6379` (root) or `redis://:clusterRedis@cluster_redis:6479` (cluster)
- `CLUSTER_LOCATION` format: `latitude,longitude,radius` (e.g., `48.1,11.6,1000`)
- Addons engine, root: `ORCHESTRATION_PLANE=root`, `MARKETPLACE_ADDR=http://marketplace_manager:11102`, `JWT_GENERATOR_URL=jwt_generator`. Without a loadable JWT key every admin endpoint answers 503.
- Addons engine, cluster: `ORCHESTRATION_PLANE=cluster`, `MARKETPLACE_ADDR=http://<root>:11102` and `JWT_PUBLIC_KEY_URL=http://<root>:11102/api/v1/marketplace/auth/public-key` (the cluster has no jwt_generator). `JWT_PUBLIC_KEY` (static PEM) overrides both. Precedence is `JWT_PUBLIC_KEY` > `JWT_PUBLIC_KEY_URL` > jwt_generator. Only the jwt_generator source refreshes its key: with the URL or static source, a root key rotation (jwt_generator restart) makes valid tokens fail with 401 until the cluster addons manager is restarted. Pin `JWT_PUBLIC_KEY` on clusters, since root to cluster is plain HTTP.
- `MARKETPLACE_OFFICIAL_INDEX` on marketplace_manager: empty means the built-in default, `off` disables the official source (air-gapped, use the import endpoint), anything else must be a valid URL or the marketplace refuses to start. `MARKETPLACE_OFFICIAL_TOKEN` is only needed for a private index. `MARKETPLACE_REGISTRY_ALLOW_PRIVATE=true` is needed when image digests must be resolved against a registry on a private or loopback address (plain http); the default is false. Addon containers get `ADDON_PIDS_LIMIT` (1024), `ADDON_MEMORY_LIMIT` (512m) and `ADDON_CPU_LIMIT` (1); an addon that is OOM-killed or can't fork may just be hitting those.
- `PRESET_URL` on the dashboard is optional. If it's set, the file must exist and be a valid preset, otherwise the dashboard shows a configuration error instead of the login page (see STEP 12)

---

## STEP 4 — Database Diagnostics (MongoDB)

### 4.1 MongoDB Connectivity

```bash
# Root Orchestrator MongoDB
docker exec mongo mongosh --port 10007 --eval "db.adminCommand('ping')" 2>/dev/null || echo "FAIL: mongo (root) not reachable"
docker exec mongo_net mongosh --port 10008 --eval "db.adminCommand('ping')" 2>/dev/null || echo "FAIL: mongo_net (root) not reachable"

# Cluster Orchestrator MongoDB
docker exec cluster_mongo mongosh --port 10107 --eval "db.adminCommand('ping')" 2>/dev/null || echo "FAIL: cluster_mongo not reachable"
docker exec cluster_mongo_net mongosh --port 10108 --eval "db.adminCommand('ping')" 2>/dev/null || echo "FAIL: cluster_mongo_net not reachable"
```

### 4.2 Root MongoDB — Data Consistency

```bash
docker exec mongo mongosh --port 10007 --eval "
  use oakestra_db;
  print('=== Collections ===');
  db.getCollectionNames().forEach(c => print(c + ': ' + db[c].countDocuments() + ' docs'));

  print('\n=== Jobs in non-terminal states ===');
  db.jobs.find({status: {\$nin: ['UNDEPLOYED','COMPLETED','DEAD']}}, {app_name:1, microservice_name:1, status:1, _id:0}).limit(20).forEach(printjson);

  print('\n=== Clusters registered ===');
  db.clusters.find({}, {cluster_name:1, cluster_location:1, active_nodes:1, available_cpu_cores:1, available_memory:1, _id:0}).forEach(printjson);

  print('\n=== Workers registered ===');
  db.nodes.find({}, {node_ip:1, current_cpu:1, current_memory:1, technology:1, _id:0}).limit(20).forEach(printjson);
" 2>/dev/null || echo "Could not query root MongoDB"
```

**What to look for:**
- Clusters/nodes with zero available CPU/memory despite real resources → resource abstractor not syncing
- Jobs stuck in `CLUSTER_SCHEDULED` or `NODE_SCHEDULED` for more than 15 s → worker not acknowledging deployment (node dead or MQTT lost). The cluster will mark them FAILED and reschedule automatically.
- Jobs stuck in `INSTANTIATION` for more than 30 s without heartbeats → worker died during image pull / container creation. Will be marked FAILED and rescheduled automatically.
- Jobs stuck in `INSTANTIATION` indefinitely with fresh heartbeats → normal for large images; wait for the image pull to finish.
- Jobs stuck in `CREATING` → NodeEngine issue on worker
- No clusters registered despite cluster being started → cluster_manager cannot reach system_manager

**Addons data (root Mongo, same instance, separate databases):**

```bash
docker exec mongo mongosh --port 10007 --quiet --eval "
  print('=== addons.installed ===');
  db.getSiblingDB('addons').installed.find({}, {qualified_name:1, version:1, status:1, status_details:1}).forEach(printjson);
  print('=== marketplace.sources ===');
  db.getSiblingDB('marketplace').sources.find({}, {name:1, kind:1, last_synced_at:1, last_error:1}).forEach(printjson);
  print('=== marketplace.catalog (one snapshot per source) ===');
  db.getSiblingDB('marketplace').catalog.find({}, {serial:1, addon_count:1, updated_at:1}).forEach(printjson);
" 2>/dev/null || echo "Could not query addons data (the cluster plane uses cluster_mongo, port 10107)"
```

Look for `installed` documents stuck in `installing`/`updating` or `failed` (read `status_details`), and sources whose `last_error` is set.

The grant key and instance id are not in Mongo. They live in the addons state volume, which must survive together with the Mongo data: `docker volume inspect addons_state` (root) or `cluster_addons_state` (cluster). If it is missing, expect `grant signature invalid` on every installed addon.

### 4.3 Cluster MongoDB — Data Consistency

```bash
docker exec cluster_mongo mongosh --port 10107 --eval "
  use cluster_db;
  print('=== Collections ===');
  db.getCollectionNames().forEach(c => print(c + ': ' + db[c].countDocuments() + ' docs'));

  print('\n=== Worker nodes ===');
  db.nodes.find({}, {node_ip:1, current_cpu:1, current_memory:1, technology:1, _id:0}).limit(20).forEach(printjson);

  print('\n=== Jobs in non-terminal states ===');
  db.jobs.find({status: {\$nin: ['UNDEPLOYED','COMPLETED','DEAD']}}, {app_name:1, microservice_name:1, status:1, _id:0}).limit(20).forEach(printjson);
" 2>/dev/null || echo "Could not query cluster MongoDB"
```

### 4.4 Network MongoDB

```bash
docker exec mongo_net mongosh --port 10008 --eval "
  use oakestra_net_db;
  print('=== Collections ===');
  db.getCollectionNames().forEach(c => print(c + ': ' + db[c].countDocuments() + ' docs'));
  print('\n=== Service IPs ===');
  db.serviceips.find({},{service_ip:1, app_name:1, _id:0}).limit(20).forEach(printjson);
" 2>/dev/null || echo "Could not query root net MongoDB"
```

---

## STEP 5 — Redis Diagnostics

### 5.1 Root Redis

```bash
docker exec root_redis redis-cli -a rootRedis ping 2>/dev/null || echo "FAIL: root_redis not responding"
docker exec root_redis redis-cli -a rootRedis info server 2>/dev/null | grep -E "redis_version|uptime|used_memory_human"
docker exec root_redis redis-cli -a rootRedis llen "asynq:{schedule:job}:pending" 2>/dev/null
docker exec root_redis redis-cli -a rootRedis llen "asynq:{schedule:job}:failed" 2>/dev/null
```

### 5.2 Cluster Redis

```bash
docker exec cluster_redis redis-cli -p 6479 -a clusterRedis ping 2>/dev/null || echo "FAIL: cluster_redis not responding"
docker exec cluster_redis redis-cli -p 6479 -a clusterRedis info server 2>/dev/null | grep -E "redis_version|uptime|used_memory_human"
docker exec cluster_redis redis-cli -p 6479 -a clusterRedis llen "asynq:{schedule:job}:pending" 2>/dev/null
docker exec cluster_redis redis-cli -p 6479 -a clusterRedis llen "asynq:{schedule:job}:failed" 2>/dev/null
```

**What to look for:**
- Many items in `failed` queue → scheduler is failing to process jobs; check scheduler logs
- Redis not responding → check if container is running; check REDIS_ADDR env var includes correct port (root: 6379, cluster: 6479)

---

## STEP 6 — MQTT Broker Diagnostics (Cluster)

```bash
# Check MQTT health (has a built-in healthcheck)
docker inspect mqtt --format '{{.State.Health.Status}}' 2>/dev/null

# Check MQTT is accepting connections
docker exec mqtt mosquitto_sub -h localhost -p 10003 -t '$SYS/#' -C 1 --timeout 5 2>/dev/null | head -5 || echo "WARN: MQTT not accepting connections"

# Check MQTT logs for refused connections or auth errors
docker logs mqtt 2>&1 | tail -50 | grep -iE "error|refused|disconnect|auth"
```

If MQTT is not healthy, `cluster_manager` and `cluster_service_manager` cannot communicate with worker NodeEngines. This blocks all deployment.

Check if `override-mosquitto-auth.yml` is being used — if so, authentication credentials must be provided; without them, workers cannot connect.

---

## STEP 7 — Inter-Service Connectivity

### 7.1 Root → Cluster Connectivity

If cluster is on a separate machine, test from the cluster machine:

```bash
# From cluster machine: can we reach root system_manager?
SYSTEM_MANAGER_URL=$(docker exec cluster_manager env 2>/dev/null | grep SYSTEM_MANAGER_URL | cut -d= -f2)
echo "Root URL: $SYSTEM_MANAGER_URL"
curl -s --connect-timeout 5 "http://${SYSTEM_MANAGER_URL}:10000/api/v1/info" 2>/dev/null | head -100 || echo "FAIL: cannot reach system_manager at $SYSTEM_MANAGER_URL:10000"
```

```bash
# From root machine: can we reach cluster manager?
# (check if cluster registered itself)
curl -s --connect-timeout 5 "http://localhost:10000/api/v1/clusters" 2>/dev/null | python3 -m json.tool 2>/dev/null | head -40
```

### 7.2 Internal Container Connectivity

```bash
# From system_manager: can it reach root_scheduler?
docker exec system_manager curl -s --connect-timeout 3 "http://root_scheduler:10004/status" 2>/dev/null || echo "WARN: system_manager cannot reach root_scheduler"

# From system_manager: can it reach resource_abstractor?
docker exec system_manager curl -s --connect-timeout 3 "http://root_resource_abstractor:11011/status" 2>/dev/null || echo "WARN: system_manager cannot reach resource_abstractor"

# From system_manager: can it reach jwt_generator? (not published on the host, only reachable inside the oakestra network)
docker exec system_manager curl -s --connect-timeout 3 "http://jwt_generator:10011/status" 2>/dev/null || echo "WARN: system_manager cannot reach jwt_generator"

# From cluster_manager: can it reach cluster_scheduler?
docker exec cluster_manager curl -s --connect-timeout 3 "http://cluster_scheduler:10105/status" 2>/dev/null || echo "WARN: cluster_manager cannot reach cluster_scheduler"
```

### 7.3 Network Mode Check

```bash
# Detect if host network mode is used
docker inspect system_manager --format '{{.HostConfig.NetworkMode}}' 2>/dev/null
docker inspect cluster_manager --format '{{.HostConfig.NetworkMode}}' 2>/dev/null
```

If `host` network mode is used (via `override-network-host.yml`), container-to-container DNS names don't work — all URLs must use `0.0.0.0` or the actual host IP. Check that environment variables in host-mode containers use IPs, not container names.

Alloy is the exception for its UI: Root binds `127.0.0.1:12345` and Cluster
binds `127.0.0.1:12346` so the diagnostic server remains local in host mode.

---

## STEP 8 — Firewall and Network Configuration

### 8.1 Host Firewall

```bash
# iptables
sudo iptables -L INPUT -n --line-numbers 2>/dev/null | head -30
# ufw (Ubuntu)
sudo ufw status 2>/dev/null
# firewalld (CentOS/RHEL)
sudo firewall-cmd --list-all 2>/dev/null
```

**Required open ports (between machines in a multi-machine deployment):**

| Port | Service | Direction |
|---|---|---|
| 10000 | Root system_manager REST API | Cluster → Root |
| 10099 | Root service manager | Cluster → Root |
| 10003 | MQTT broker | Worker → Cluster |
| 10100 | Cluster manager | Worker → Cluster |
| 10110 | Cluster service manager | Worker → Cluster |
| 80 | Dashboard | User → Root |

Alloy UI ports `12345` (Root/1-DOC) and `12346` (Cluster) are bound to
loopback only.

**Quick fix — open required ports (adjust interface as needed):**
```bash
# Example for ufw:
sudo ufw allow 10000/tcp
sudo ufw allow 10099/tcp
sudo ufw allow 10003/tcp
sudo ufw allow 10100/tcp
sudo ufw allow 10110/tcp
```

### 8.2 Docker Network Inspection

```bash
docker network ls | grep oakestra
docker network inspect oakestra 2>/dev/null | python3 -m json.tool | grep -E "Name|Subnet|Gateway|IPv6"
```

Check for subnet conflicts with the host network. If the `172.x.x.x` range used by Docker conflicts with the host's LAN, routing issues will occur.

### 8.3 IP Address Configuration

```bash
# Check what IPs were used to start containers
docker exec cluster_manager env 2>/dev/null | grep -E "SYSTEM_MANAGER_URL|CLUSTER_ADDRESS"
docker inspect oakestra-frontend-container --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null | grep API_ADDRESS

# Verify the IPs are reachable
ip addr show | grep "inet " | grep -v "127.0.0.1"

# From the cluster host, verify CLUSTER_ADDRESS is one of THIS host's IPs:
CLUSTER_ADDRESS=$(docker exec cluster_manager env 2>/dev/null | grep ^CLUSTER_ADDRESS= | cut -d= -f2)
ip -o addr show | awk '{print $4}' | cut -d/ -f1 | grep -Fxq "$CLUSTER_ADDRESS" \
    && echo "OK: CLUSTER_ADDRESS=$CLUSTER_ADDRESS matches a host interface" \
    || echo "WARNING: CLUSTER_ADDRESS=$CLUSTER_ADDRESS does NOT match any local interface IP"

# From the root host, verify the cluster manager is reachable at its advertised address:
curl -sS --connect-timeout 5 "http://$CLUSTER_ADDRESS:10100/api/cluster/status" \
    || echo "FAIL: root cannot reach cluster_manager at $CLUSTER_ADDRESS:10100"
```

Common pitfalls:
- Using `localhost`/`127.0.0.1` as `SYSTEM_MANAGER_URL` when cluster and root are on different machines.
- Using a Docker bridge gateway (e.g., `172.17.0.1`, `172.19.0.1`) as `CLUSTER_ADDRESS` — this address is not routable from the root machine and registration will fail the reachability probe. Use the cluster host's default-route IP instead.
- Two clusters registering with the same `CLUSTER_ADDRESS` — the second registration is fine on the wire but every dispatch to it will land on whichever host actually answers `CLUSTER_ADDRESS:10100`.

---

## STEP 9 — Worker Node Diagnostics

### 9.1 NodeEngine Log Location

NodeEngine writes its own log file. The path is configurable in `/etc/oakestra/conf.json` (field `AppLogs`); the default is `/var/log/oakestra/nodeengine.log`.

```bash
# Read the configured log path from conf.json
NODEENGINE_LOG=$(python3 -c "import json,sys; d=json.load(open('/etc/oakestra/conf.json')); print(d.get('AppLogs','/var/log/oakestra/nodeengine.log'))" 2>/dev/null || echo "/var/log/oakestra/nodeengine.log")
echo "NodeEngine log: $NODEENGINE_LOG"

# Tail errors from the log file
tail -100 "$NODEENGINE_LOG" 2>/dev/null | grep -iE "error|fail|refused|panic|timeout|mqtt" || echo "Log file not found or empty at $NODEENGINE_LOG"

# Full last 50 lines (no filter) for context
tail -50 "$NODEENGINE_LOG" 2>/dev/null || true
```

To change the log path: `sudo NodeEngine config applogs /your/custom/path`

### 9.2 NodeEngine Status

```bash
# If running as systemd service
systemctl status NodeEngine 2>/dev/null
# Systemd journal (startup/crash messages, before the log file is open)
journalctl -u NodeEngine -n 50 --no-pager 2>/dev/null | grep -iE "error|fail|refused|panic|timeout|mqtt"

# If running directly
ps aux | grep -i NodeEngine

# NodeEngine version
NodeEngine --version 2>/dev/null || /usr/local/bin/NodeEngine --version 2>/dev/null

# NodeEngine status command
NodeEngine status 2>/dev/null || true
```

### 9.3 NetManager Log Location and Status

NetManager writes its log to a **static path** that cannot be configured: `/var/log/oakestra/netmanager.log`.

```bash
# Tail errors from the NetManager log
tail -100 /var/log/oakestra/netmanager.log 2>/dev/null | grep -iE "error|fail|refused|panic|timeout" || echo "Log file not found or empty at /var/log/oakestra/netmanager.log"

# Full last 50 lines for context
tail -50 /var/log/oakestra/netmanager.log 2>/dev/null || true
```

```bash
systemctl status NetManager 2>/dev/null
journalctl -u NetManager -n 50 --no-pager 2>/dev/null | grep -iE "error|fail|refused|panic|timeout"
ps aux | grep -i NetManager
```

### 9.4 NodeEngine Configuration

```bash
# Default config file locations
cat /etc/oakestra/nodeengine.conf 2>/dev/null || \
cat /usr/local/etc/oakestra/nodeengine.conf 2>/dev/null || \
cat /opt/oakestra/nodeengine.conf 2>/dev/null || \
NodeEngine conf 2>/dev/null || \
echo "Could not find nodeengine config"
```

Check that:
- `MQTT_BROKER_URL` points to the cluster machine's IP (not localhost unless 1-DOC)
- `MQTT_BROKER_PORT` = 10003
- `CLUSTER_SERVICE_MANAGER_URL` and port 10110 are accessible

### 9.5 Worker → Cluster Connectivity

```bash
# Can worker reach MQTT?
MQTT_HOST=$(NodeEngine conf 2>/dev/null | grep -i mqtt | grep -oE "[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+" | head -1)
if [ -n "$MQTT_HOST" ]; then
  nc -zv "$MQTT_HOST" 10003 2>&1 || echo "FAIL: cannot reach MQTT at $MQTT_HOST:10003"
  nc -zv "$MQTT_HOST" 10100 2>&1 || echo "FAIL: cannot reach cluster_manager at $MQTT_HOST:10100"
  nc -zv "$MQTT_HOST" 10110 2>&1 || echo "FAIL: cannot reach cluster_service_manager at $MQTT_HOST:10110"
fi
```

### 9.6 Installed Container Runtimes

NodeEngine supports Docker and containerd (unikernels/VMs may require additional runtimes).

```bash
docker info 2>/dev/null | grep -E "Server Version|Runtimes|Docker Root"
containerd --version 2>/dev/null || echo "containerd not found"
# Check for unikernel support
ls /dev/kvm 2>/dev/null && echo "KVM available (unikernel support)" || echo "KVM not available"
```

### 9.7 GPU Support

```bash
# NVIDIA GPU check
nvidia-smi 2>/dev/null || echo "nvidia-smi not found (no NVIDIA GPU or driver issue)"
docker run --rm --gpus all nvidia/cuda:11.0-base nvidia-smi 2>/dev/null || echo "Docker GPU passthrough not working"

# Check GPU configuration script
ls /usr/local/bin/configure_gpu.sh 2>/dev/null || \
ls /opt/oakestra/configure_gpu.sh 2>/dev/null || \
echo "GPU configure script not found (run go_node_engine/build/configure_gpu.sh if GPU is needed)"
```

---

## STEP 10 — Observability Stack Check

```bash
# Root Grafana
curl -s --connect-timeout 3 "http://localhost:3000/api/health" 2>/dev/null | python3 -m json.tool 2>/dev/null || echo "Root Grafana not responding on :3000"

# Cluster Grafana
curl -s --connect-timeout 3 "http://localhost:3001/api/health" 2>/dev/null | python3 -m json.tool 2>/dev/null || echo "Cluster Grafana not responding on :3001"

# Loki
curl -s --connect-timeout 3 "http://localhost:3100/ready" 2>/dev/null || echo "Root Loki not responding on :3100"
curl -s --connect-timeout 3 "http://localhost:3101/ready" 2>/dev/null || echo "Cluster Loki not responding on :3101"

# Prometheus (cluster)
curl -s --connect-timeout 3 "http://localhost:10009/-/healthy" 2>/dev/null || echo "Prometheus not responding on :10009"

# Alloy diagnostic UIs (loopback only)
curl -fsS --connect-timeout 3 "http://localhost:12345/-/ready" 2>/dev/null || echo "Root/1-DOC Alloy UI not responding on :12345"
curl -fsS --connect-timeout 3 "http://localhost:12346/-/ready" 2>/dev/null || echo "Cluster Alloy UI not responding on :12346"

# Confirm each Alloy has the read-only Docker socket and persistent state volume
docker inspect alloy --format '{{range .Mounts}}{{println .Destination "RW=" .RW}}{{end}}' 2>/dev/null || echo "Root/1-DOC Alloy is not running"
docker inspect cluster_alloy --format '{{range .Mounts}}{{println .Destination "RW=" .RW}}{{end}}' 2>/dev/null || echo "Cluster Alloy is not running"

# Confirm the required labels have reached the local Loki
curl -s --connect-timeout 3 "http://localhost:3100/loki/api/v1/labels" 2>/dev/null | grep -E 'container|compose_service|cluster_id' || echo "Root Loki labels are missing"
curl -s --connect-timeout 3 "http://localhost:3101/loki/api/v1/labels" 2>/dev/null | grep -E 'container|compose_service|cluster_id' || echo "Cluster Loki labels are missing"
```

---

## STEP 11 — API Smoke Tests

```bash
# Alloy diagnostic API
curl -fsS --connect-timeout 3 "http://localhost:12345/-/ready" 2>/dev/null || echo "FAIL: Root/1-DOC Alloy diagnostic API not responding"
curl -fsS --connect-timeout 3 "http://localhost:12346/-/ready" 2>/dev/null || echo "FAIL: Cluster Alloy diagnostic API not responding"

# Root System Manager API
echo "=== System Manager Health ==="
curl -s --connect-timeout 5 "http://localhost:10000/api/v1/info" 2>/dev/null | python3 -m json.tool 2>/dev/null || echo "FAIL: system_manager API not responding"

echo "=== Registered Clusters ==="
curl -s --connect-timeout 5 "http://localhost:10000/api/v1/clusters" 2>/dev/null | python3 -m json.tool 2>/dev/null | head -40 || echo "FAIL or no auth token"

echo "=== Root Scheduler Status ==="
curl -s --connect-timeout 5 "http://localhost:10004/status" 2>/dev/null || echo "FAIL: root_scheduler not responding"

echo "=== Resource Abstractor (Root) ==="
# Internal only (not published on the host), so check it from inside the network.
docker exec system_manager curl -s --connect-timeout 5 "http://root_resource_abstractor:11011/status" 2>/dev/null || echo "FAIL: root_resource_abstractor not responding"

echo "=== Cluster Scheduler Status ==="
curl -s --connect-timeout 5 "http://localhost:10105/status" 2>/dev/null || echo "FAIL: cluster_scheduler not responding"

echo "=== Resource Abstractor (Cluster) ==="
# Internal only (not published on the host), so check it from inside the network.
docker exec cluster_manager curl -s --connect-timeout 5 "http://cluster_resource_abstractor:11012/status" 2>/dev/null || echo "FAIL: cluster_resource_abstractor not responding"

echo "=== JWT Generator ==="
# Port 10011 is deliberately not published on the host (POST /create mints tokens without
# authentication), so check it from inside the network.
docker exec system_manager curl -s --connect-timeout 5 "http://jwt_generator:10011/status" 2>/dev/null || echo "FAIL: jwt_generator not responding"
```

---

## STEP 12 — Addons System (Root Only)

```bash
echo "=== Addons Manager ==="
# Go service: GET / answers "ok". Use port 11201 for cluster_addons_manager.
curl -s --connect-timeout 5 "http://localhost:11101/" 2>/dev/null || echo "addons_manager not responding (may be disabled via override-no-addons.yml)"

echo "=== Marketplace Manager ==="
curl -s --connect-timeout 5 "http://localhost:11102/" 2>/dev/null || echo "marketplace_manager not responding"
docker inspect marketplace_manager --format 'health: {{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' 2>/dev/null

echo "=== Marketplace sources (sync state, last_error) ==="
curl -s --connect-timeout 5 "http://localhost:11102/api/v1/marketplace/sources" 2>/dev/null | python3 -m json.tool 2>/dev/null | grep -E '"name"|last_error|last_synced_at|serial' || echo "could not read sources"

echo "=== Public key served to the cluster plane (200 = key loaded) ==="
curl -s -o /dev/null -w "%{http_code}\n" --connect-timeout 5 "http://localhost:11102/api/v1/marketplace/auth/public-key"

# Only the addons manager mounts the docker socket. The image is distroless (no shell, no ls),
# so look at the container's mounts instead of exec'ing into it.
docker inspect root_addons_manager --format '{{range .Mounts}}{{println .Source}}{{end}}' 2>/dev/null | grep -q docker.sock \
  || echo "WARN: root_addons_manager has no docker socket mounted"

echo "=== UI addons ==="
# Active addons with a dashboard UI. An empty list means none are installed or active yet.
curl -s --connect-timeout 5 "http://localhost:11101/api/v1/ui-addons" 2>/dev/null || echo "addons_manager not responding"
```

Addon UIs and presets need the v2 dashboard (`ghcr.io/oakestra/dashboard-v2`), which only runs with `override-dashboard-v2.yml` (`.yaml` in 1-DOC). The default is v1 (`ghcr.io/oakestra/dashboard`). To see which one is running:

```bash
# Use oaekstra-frontend-container in 1-DOC.
docker inspect oakestra-frontend-container --format '{{.Config.Image}}'
```

The dashboard's nginx fetches each addon UI from `http://addonui-<id>:80/`. The addons manager (root plane) attaches the UI container to the dedicated `oakestra_addon_ui` network with that alias, and the dashboard container is on the same network. `<id>` is the bare name for official addons and `<source>--<name>` for tap and local ones; containers are named `root_addon-<id>--<service>`. If an addon UI doesn't load, or `/addon-assets/<id>/mf-manifest.json` returns 502, check that the network exists, that both the dashboard and the UI container are on it, and try the request from the dashboard container (`oakestra-frontend-container` on root, `oaekstra-frontend-container` in 1-DOC):

```bash
NAME=<addon-id>
# Aliases of every container on the addon UI network. The dashboard and addonui-$NAME should
# both be there.
docker network inspect oakestra_addon_ui --format '{{range .Containers}}{{.Name}} {{end}}' | tr ' ' '\n' | grep . \
  | xargs -I{} docker inspect {} --format '{{.Name}}: {{json .NetworkSettings.Networks.oakestra_addon_ui.Aliases}}'
docker exec oakestra-frontend-container wget -qO /dev/null "http://addonui-$NAME/mf-manifest.json" \
  && echo "OK: addonui-$NAME serves mf-manifest.json" \
  || echo "FAIL: addonui-$NAME not reachable from the dashboard"
```

Installing an addon (`POST /api/v1/addons {name, accept_permissions}`) can fail with 404 (marketplace doesn't know the addon), 502 (marketplace unreachable, check `marketplace_manager`) or 409. A 409 with `permissions` and `permissions_hash` in the body is the permission confirmation: repeat the call with `accept_permissions` set to the hash. Any other 409 means the addon is already installed or conflicts with another one. Updates (`POST /api/v1/addons/<id>/update`) ask again when the new version needs more permissions.

**Admin endpoints answer 503:** the service has no JWT public key yet. On root check that `jwt_generator` is up and `JWT_GENERATOR_URL` is right; on the cluster plane check `JWT_PUBLIC_KEY_URL` and that `curl http://<root>:11102/api/v1/marketplace/auth/public-key` returns a key (port 11102 must be reachable from the cluster host). Look for key-load retries in `docker logs root_addons_manager` / `marketplace_manager`.

**Addon status `failed`:** read `status_details` (`curl .../api/v1/addons/<id>` with an admin token, or the `addons.installed` collection). A permissions/policy message means the definition uses something it did not declare (host path grants are exact, root-equivalent paths need `host_root`, runtime sockets and `/run/docker` need `docker_socket`) or that can't be granted; the definition has to be fixed upstream. A `grant signature invalid` message is fixed with `POST /api/v1/addons/<id>/approve {"accept_permissions": "<hash>"}` (also use it to retry after fixing the cause); see the error table for when that points to a security incident. Otherwise it is usually an image that can't be pulled (offline host, digest missing) or a Docker error from the addons manager log.

**Tap source `last_error`:** `GET /api/v1/marketplace/sources` shows `last_error`, `last_attempt_at`, `last_synced_at` and `serial`. Typical causes: plain http without `allow_insecure`, a private or loopback address without `allow_private`, an index over 5 MiB or a timeout, a redirect to another host, a `serial` lower than the last good one, or an invalid definition. The last good snapshot stays in use until a sync succeeds. Retry with `POST /api/v1/marketplace/sources/<name>/sync`. For an air-gapped host use `MARKETPLACE_OFFICIAL_INDEX=off` and `POST .../sources/<name>/import`.

Addon UIs don't work with `override-network-host.yml`, since the dashboard has no Docker DNS there (and the base file's `oakestra_addon_ui` attachment is dropped in host mode).

An addon that is `active` and listed in `/api/v1/ui-addons` but never appears in the dashboard may be named like a built-in module (`applications`, `infrastructure`, `users`, `organizations`, `help`). The dashboard ignores those and only logs a warning in the browser console.

`override-no-addons.yml` turns off the addon pages through a dashboard preset, so it's normal that they're missing there. If the dashboard shows a configuration error instead of the login page, or the addon pages are gone without that override, check the preset:

```bash
# Use oaekstra-frontend-container in 1-DOC. An empty presetUrl means no preset.
docker exec oakestra-frontend-container cat /usr/share/nginx/html/config.json
docker exec oakestra-frontend-container cat /usr/share/nginx/html/preset.json
```

The error page lists what is wrong with the preset. A 404 for `preset.json` means `PRESET_URL=/preset.json` is set but no file is mounted. If `docker compose` rejects `configs.content` in `override-no-addons.yml`, Docker Compose is older than 2.23.1.

---

## STEP 13 — Docker Image and Build Issues

```bash
# Check which images are using pre-built vs locally built
docker images --format "table {{.Repository}}\t{{.Tag}}\t{{.CreatedSince}}\t{{.Size}}" | grep -E "system_manager|cluster_manager|root_scheduler|cluster_scheduler|resource_abstractor|jwt_generator|addons"

# Check for dangling images / build cache issues
docker images -f "dangling=true" | head -10
```

If images are outdated or from a wrong branch, rebuild:
```bash
# In root_orchestrator/ directory:
docker compose build --no-cache system_manager

# Or pull fresh images (for pre-built):
docker compose pull
```

The shared `libraries/` packages (`oakestra_utils_library`, `resource_abstractor_client`) are built from the repo-local `libraries/` folder via a Buildx named build context — not from a remote git repo, and there is no `LIB_BRANCH` env var. If a build fails on these libraries, check that:
```bash
# The libraries build context resolves (declared in docker-compose.yml as additional_contexts):
ls libraries/oakestra_utils_library libraries/resource_abstractor_client
# BuildKit/Buildx is enabled (named build contexts require it — default on modern Docker):
docker buildx version
```

---

## STEP 14 — Log Deep Dive for Specific Issues

For containers showing problems, do a deep log analysis:

Grafana, Loki, and Alloy are excluded from centralized log collection by default
through `oakestra.logging.enabled: "false"`. Inspect their Docker logs directly;
their absence from Loki does not indicate a collector failure. A Compose
override can set the label to `"true"` for centralized diagnostics.

```bash
# Full logs for a specific container (replace <name>)
docker logs <name> 2>&1

# Search across all container logs for a specific pattern
for name in $(docker ps --format "{{.Names}}"); do
  result=$(docker logs --tail 200 "$name" 2>&1 | grep -iE "error|exception|traceback|refused|timeout|fatal|panic" | head -5)
  if [ -n "$result" ]; then
    echo "===== $name ====="
    echo "$result"
  fi
done
```

**Cluster manager registration failure** — look for:
```bash
docker logs cluster_manager 2>&1 | grep -iE "register|system_manager|refused|timeout|cluster_address" | tail -20
docker logs system_manager 2>&1 | grep -iE "grpc|cluster_address|reachability|register|not reachable" | tail -20
```
Specifically:
- `CLUSTER_ADDRESS env var is not set` in cluster_manager → cluster started without `CLUSTER_ADDRESS`; the cluster aborts the gRPC handshake.
- `cluster_address is required` in system_manager → root received a `CS2Message` with an empty `cluster_address` field (old cluster image talking to a new root).
- `Cluster reachability probe failed for http://<ip>:10100/api/cluster/status` in system_manager → root could not reach the cluster at the advertised address. Causes: wrong `CLUSTER_ADDRESS`, host firewall blocking 10100, cluster_manager not yet listening, or the cluster host is unreachable from the root host. The root aborts registration with `FAILED_PRECONDITION`.

**Scheduler job failure** — look for:
```bash
docker logs root_scheduler 2>&1 | grep -iE "error|failed|no worker|no cluster" | tail -20
docker logs cluster_scheduler 2>&1 | grep -iE "error|failed|no worker" | tail -20
```

**Resource abstractor sync issues:**
```bash
docker logs root_resource_abstractor 2>&1 | grep -iE "error|mongo|failed" | tail -20
docker logs cluster_resource_abstractor 2>&1 | grep -iE "error|mongo|failed" | tail -20
```

---

## STEP 15 — Common Fixes

Apply these fixes directly when the diagnosis matches:

### Fix: Container in restart loop due to dependency not ready
```bash
# Restart the affected container after its dependency is healthy
docker restart <container_name>
```

### Fix: Wrong SYSTEM_MANAGER_URL (using localhost instead of real IP)
```bash
# Get current machine IP
ip route get 1.1.1.1 | awk '{print $7; exit}'
# Then stop and re-run the startup script with the correct IP
# export SYSTEM_MANAGER_URL=<real_ip>
```

### Fix: Port already in use
```bash
# Find and kill the conflicting process
sudo ss -tlnp sport = :<port>
sudo kill -9 <pid>
# Then restart the relevant docker compose stack
```

### Fix: Stale containers from previous deployment
```bash
# Stop and remove all oakestra containers
docker ps -a --format "{{.Names}}" | grep -E "system_manager|mongo|root_|cluster_|jwt_|grafana|loki|alloy|mqtt|addons|marketplace|oakestra" | xargs -r docker rm -f
# Then restart
```

### Fix: MongoDB volume data corruption
```bash
# List volumes
docker volume ls | grep -E "mongodb|oakestra"
# Remove corrupted volume (WARNING: data loss)
docker volume rm <volume_name>
```

### Fix: NodeEngine cannot connect to MQTT
```bash
# Check firewall on cluster machine for port 10003
sudo ufw allow 10003/tcp
# Verify MQTT is listening externally (not just 127.0.0.1)
ss -tlnp sport = :10003
```

### Fix: Docker socket permission denied
```bash
sudo chmod 666 /var/run/docker.sock
# Or add user to docker group (preferred)
sudo usermod -aG docker $USER && newgrp docker
```

### Fix: IPv6 issues / cluster not registering
If the startup script detected an IPv6 address for SYSTEM_MANAGER_URL but the cluster expects IPv4:
- Re-run startup with explicit IPv4: `export SYSTEM_MANAGER_URL=<ipv4_address>`
- Or use `override-ipv6-enabled.yml` if IPv6 is intentional

### Fix: Cluster registration aborted with `cluster not reachable at <ip>:10100`
The root probes `GET /api/cluster/status` against the address the cluster advertised in `CLUSTER_ADDRESS`. If the probe fails, registration is refused with gRPC `FAILED_PRECONDITION`.

1. From the cluster host, confirm `CLUSTER_ADDRESS` is one of this host's real interface IPs (NOT `127.0.0.1`, `0.0.0.0`, or a `172.x.x.x` docker bridge IP):
   ```bash
   docker exec cluster_manager env | grep ^CLUSTER_ADDRESS=
   ip -o addr show | awk '{print $4}' | cut -d/ -f1
   ```
2. From the root host, verify it can actually reach the cluster:
   ```bash
   curl -sS --connect-timeout 5 "http://<CLUSTER_ADDRESS>:10100/api/cluster/status"
   ```
   - Connection refused → cluster_manager not yet up. Wait, or check `docker ps`.
   - Connection timeout → firewall on the cluster host blocks 10100. Open it: `sudo ufw allow 10100/tcp`.
   - No route → wrong `CLUSTER_ADDRESS`. Re-run `StartOakestraCluster.sh` and supply the correct IP at the prompt, or export `CLUSTER_ADDRESS=<routable_ip>` before running.
3. Restart the cluster orchestrator so it re-registers:
   ```bash
   docker compose -f ~/.oakestra/cluster_orchestrator/cluster-orchestrator.yml down
   ./scripts/StartOakestraCluster.sh
   ```

### Fix: Multiple clusters end up with the same advertised IP
If `oak cl ls` shows two clusters with the same IP (e.g., both `172.19.0.1` or both equal to the same host's address):
- The registrations were made before the `CLUSTER_ADDRESS` requirement was introduced, OR
- Both cluster hosts were started with a docker-bridge IP as `CLUSTER_ADDRESS`.

Fix by stopping both clusters, ensuring each one is started with a distinct, **routable-from-root** `CLUSTER_ADDRESS` (the cluster host's default-route IP), and re-registering. The root will now refuse to register a cluster that does not respond at its advertised address, so a misconfiguration shows up immediately at startup instead of silently corrupting job dispatch.

---

## STEP 16 — Compile Diagnosis Report

At the end of the investigation, produce a structured report with this format:

```
# Oakestra Diagnosis Report
Generated: <date>
Machine role: <Root / Cluster / Worker / 1-DOC>

## Summary
<2-3 sentence overview of what was found>

## Critical Issues (blocking deployment)
- [ ] <issue 1> — <root cause> — <fix applied or recommended action>
- [ ] <issue 2> — ...

## Non-Critical Issues (degraded functionality)
- [ ] <issue 1> — <impact> — <recommendation>

## Container Status
| Container | Status | Issues |
|---|---|---|
| system_manager | Running / Crashed / Missing | <notes> |
| ... | | |

## Database Status
- Root MongoDB: <OK / ERROR + details>
- Root Net MongoDB: <OK / ERROR + details>
- Cluster MongoDB: <OK / ERROR + details>
- Redis (root): <OK / ERROR + details>
- Redis (cluster): <OK / ERROR + details>

## Network Status
- Firewall: <open / blocked ports>
- Inter-service DNS: <OK / broken — host mode?>
- Root↔Cluster connectivity: <OK / unreachable at URL:port>
- Worker→Cluster MQTT: <OK / unreachable>

## Fixes Applied
- <list any changes made during this session>

## Escalation Required
<If this is an Oakestra bug rather than a configuration issue, include:>
- Oakestra version: (from version.txt or image tags)
- Deployment type: <1-DOC / multi-machine>
- Steps to reproduce:
- Expected behavior:
- Actual behavior:
- Relevant logs (attach):
  - <container_name>: <key error lines>
- Environment:
  - OS: <uname -a>
  - Docker: <version>
  - Architecture: <amd64/arm64>
```

For bugs requiring escalation, direct the user to open an issue at:
**https://github.com/oakestra/oakestra/issues**

Include the full diagnosis report as the issue body.

---

## STEP 17: Troubleshoot Worker CSI Plugin

This step helps diagnose issues with the Container Storage Interface (CSI) plugin on a worker node, specifically for the `hostpath` provider.

### 17.1 Check CSI Plugin Registration

Verify that the `hostpath` CSI plugin is registered with the NodeEngine.

**Command:**
```bash
sudo NodeEngine config csi list
```

**Expected Output:**
The output should list the `csi.oakestra.io/hostpath` plugin with its socket.
```
┌──────────────────────────┬──────────────────────────────────────────┐
│ DRIVER                   │ SOCKET                                   │
├──────────────────────────┼──────────────────────────────────────────┤
│ csi.oakestra.io/hostpath │ unix:///var/lib/oakestra/csi/hostpath.sock │
└──────────────────────────┴──────────────────────────────────────────┘
```

**What it means:**
- **If the plugin is listed:** The NodeEngine knows about the CSI plugin.
- **If the plugin is NOT listed:** The NodeEngine has not registered the plugin.
    - **Fix:** Register the plugin using the command from the `csi/hostpath/README.md`:
      ```bash
      sudo NodeEngine config csi add csi.oakestra.io/hostpath unix:///var/lib/oakestra/csi/hostpath.sock
      ```
      Then restart the NodeEngine.

### 17.2 Check CSI Plugin Container

The CSI plugin runs as a Docker container. Check if it's running correctly.

**Command:**
```bash
docker ps --filter "name=oakestra-hostpath-csi"
```

**Expected Output:**
The `oakestra-hostpath-csi` container should be listed and in the `Up` status.

**What it means:**
- **If the container is `Up`:** The plugin container is running.
- **If the container is not running or restarting:** There is a problem with the container itself.
    - **Fix:**
        1. Check the container logs: `docker logs oakestra-hostpath-csi`
        2. Ensure the container was started with the correct parameters as specified in `csi/hostpath/README.md`. It needs to be `--privileged` and have the correct volume mounts.

### 17.3 Check Mount Propagation

The volume mounts for the CSI plugin **must** have `rshared` propagation. This is a common point of failure.

**Command:**
```bash
docker inspect oakestra-hostpath-csi --format '{{json .HostConfig.Mounts}}' | python3 -m json.tool
```

**Expected Output:**
Look for `"Propagation": "rshared"` on the CSI-related mounts.
```json
[
    {
        "Type": "bind",
        "Source": "/var/lib/oakestra/csi",
        "Target": "/var/lib/oakestra/csi",
        "BindOptions": {
            "Propagation": "rshared"
        }
    },
    {
        "Type": "bind",
        "Source": "/mnt/oakestra/hostpath",
        "Target": "/mnt/oakestra/hostpath",
        "BindOptions": {
            "Propagation": "rshared"
        }
    }
]
```

**What it means:**
- **If `rshared` is present:** Mount propagation is likely correct.
- **If `rshared` is missing or different:** Mounts created by the CSI plugin will not be visible to other containers.
    - **Fix:** Stop and remove the `oakestra-hostpath-csi` container and restart it, ensuring the volume mounts have the `:rshared` flag (e.g., `-v /var/lib/oakestra/csi:/var/lib/oakestra/csi:rshared`).

### 17.4 Check for CSI Socket File

The NodeEngine communicates with the CSI plugin over a Unix socket. Check if the socket file exists.

**Command:**
```bash
sudo ls -l /var/lib/oakestra/csi/hostpath.sock
```

**Expected Output:**
A socket file should be present.
```
srw-rw-rw- 1 root root 0 Mar 25 10:00 /var/lib/oakestra/csi/hostpath.sock
```

**What it means:**
- **If the socket exists:** The CSI plugin container has created the socket.
- **If the socket does NOT exist:**
    - The CSI plugin container may not be running or may have failed to start. Check its logs (`docker logs oakestra-hostpath-csi`).
    - The volume mount `/var/lib/oakestra/csi` might be incorrect.

### 17.5 Check NodeEngine Logs for CSI Errors

Inspect the NodeEngine logs for any errors related to CSI.

**Command:**
```bash
sudo NodeEngine logs | grep -i "csi"
```

**Look for errors like:**
- `connection error`
- `plugin not found`
- `failed to probe`
- `NodePublishVolume failed`

**What it means:**
These logs can point to connectivity issues with the socket, permission problems, or failures during the volume mounting process.

### 17.6 Check CSI Plugin Logs

Inspect the logs from the CSI plugin container itself.

**Command:**
```bash
docker logs oakestra-hostpath-csi
```

**Look for errors like:**
- `permission denied` when trying to mount.
- `path not found` for the source host path.
- Errors from the gRPC server.

**What it means:**
These are low-level logs from the plugin. `permission denied` often means the container is not running with `--privileged`. `path not found` means the host directory you want to mount is not available inside the CSI plugin container.

### 17.7 Summary of Common CSI Issues and Fixes

| Symptom | Likely Cause | Fix |
|---|---|---|
| Volume is empty inside application container. | Mount propagation is not `rshared`. | Re-create CSI container with `:rshared` on volume mounts. |
| `NodeEngine` logs show "plugin not found" or "failed to probe". | CSI container not running, or socket path mismatch. | Start the CSI container. Verify socket path in `NodeEngine` config matches the `-endpoint` of the CSI container. |
| CSI container logs show "permission denied". | Container is not privileged. | Re-create CSI container with the `--privileged` flag. |
| CSI container logs show "path not found". | The source host path is not mounted into the CSI container. | Add another `-v /path/on/host:/path/on/host` mount to the CSI container's `docker run` command. |
| Application deployment fails with "volume not available". | CSI plugin is not registered or not running. | Follow steps 17.1 and 17.2. |
