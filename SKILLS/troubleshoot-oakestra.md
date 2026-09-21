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
for port in 10000 50052 10007 10008 10099 6379 10004 11011 10011 80 3000 3100 11101 11102 11103; do
  ss -tlnp "sport = :$port" 2>/dev/null | grep -v "State" | head -1 && echo "  ^ port $port" || true
done
```

**Cluster Orchestrator ports:**
```bash
for port in 10003 10107 10108 10110 10100 10101 10105 11012 6479 10009 3001 3101; do
  ss -tlnp "sport = :$port" 2>/dev/null | grep -v "State" | head -1 && echo "  ^ port $port" || true
done
```

Flag any port occupied by a non-Oakestra process. Common conflict: port 80 occupied by nginx/apache, port 3000 by another Grafana, port 6379 by a system Redis.

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
`system_manager`, `mongo`, `mongo_net`, `root_service_manager`, `root_redis`, `root_scheduler`, `root_resource_abstractor`, `jwt_generator`, `grafana`, `loki`, `promtail`, `oakestra-frontend-container`

Optional root containers (if addons enabled):
`addons_manager`, `addons_monitor`, `addons_dashboard`, `marketplace_manager`

**Expected containers for Cluster Orchestrator:**
`mqtt`, `cluster_mongo`, `cluster_mongo_net`, `cluster_service_manager`, `cluster_manager`, `cluster_scheduler`, `cluster_resource_abstractor`, `cluster_redis`, `prometheus`, `cluster_grafana`, `cluster_loki`, `cluster_promtail`

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
for name in $(docker ps -a --format "{{.Names}}" | grep -E "system_manager|mongo|root_service_manager|root_redis|root_scheduler|root_resource_abstractor|jwt_generator|cluster_manager|cluster_service_manager|cluster_mongo|cluster_redis|cluster_scheduler|cluster_resource_abstractor|mqtt|addons|marketplace|promtail|loki|grafana"); do
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
| `CLUSTER_NAME not set` | Missing env var in cluster startup |
| `SYSTEM_MANAGER_URL not set` | Cluster started without root URL |
| `CLUSTER_ADDRESS env var is not set` (cluster_manager log) | Missing env var — cluster cannot advertise its address to root |
| `cluster_address is required` (system_manager log) | Cluster connected via gRPC but sent empty `cluster_address` |
| `Cluster reachability probe failed` / `cluster not reachable at` (system_manager log) | Root cannot reach `http://CLUSTER_ADDRESS:10100/api/cluster/status` — wrong IP, firewall, or cluster_manager not up |
| `Automatic cluster certificate renewal FAILED` / `renewal after root CA rotation failed` (cluster_manager, gateway mode) | Cluster could not renew its client cert in-band; it keeps the current one and retries (daily, or every 5 min after a rotation). See STEP 18.7 |
| `The root CA expires in` / `The cluster intermediate CA expires in` (warnings, gateway mode) | A CA is within 90 days of expiry — rotate/replace it before then (STEP 18.7) |
| `grace period ended … must be re-registered` (system_manager) / `Worker migration … ended` (cluster_manager) | A root CA rotation or intermediate replacement finished; clusters/workers that did not renew in time are locked out and need a new token (STEP 18.7) |
| `certificate signature failure` / `unable to get certificate CRL` / `unable to get local issuer certificate` (Kong or mosquitto logs, gateway mode) | Client cert chains to a CA that is no longer trusted, or no CRL for its issuer is loaded — typically a peer that missed a rotation/migration deadline (STEP 18.6, 18.7) |
| `Worker certificate renewal failed` (NodeEngine log) | Worker could not renew over `/api/certs/worker-renew`; it keeps its current cert and the cluster asks again every 10 min (STEP 18.7) |

---

## STEP 3 — Environment Variables Validation

Check that critical env vars were correctly injected into each container:

```bash
# Root Orchestrator critical vars
docker exec system_manager env 2>/dev/null | grep -E "ROOT_MONGO|ROOT_SCHEDULER|RESOURCE_ABSTRACTOR|NET_PLUGIN|JWT" || echo "system_manager not running"

docker exec root_scheduler env 2>/dev/null | grep -E "MANAGER_URL|RESOURCE_ABSTRACTOR|REDIS_ADDR" || echo "root_scheduler not running"

docker exec root_resource_abstractor env 2>/dev/null | grep -E "MONGO_URL|MONGO_PORT" || echo "root_resource_abstractor not running"

docker exec root_service_manager env 2>/dev/null | grep -E "SYSTEM_MANAGER|ROOT_MONGO|JWT" || echo "root_service_manager not running"
```

```bash
# Cluster Orchestrator critical vars
docker exec cluster_manager env 2>/dev/null | grep -E "SYSTEM_MANAGER_URL|CLUSTER_NAME|CLUSTER_LOCATION|CLUSTER_ADDRESS|MQTT|CLUSTER_MONGO|CLUSTER_SCHEDULER|RESOURCE_ABSTRACTOR" || echo "cluster_manager not running"

docker exec cluster_scheduler env 2>/dev/null | grep -E "MANAGER_URL|RESOURCE_ABSTRACTOR|REDIS_ADDR" || echo "cluster_scheduler not running"

docker exec cluster_service_manager env 2>/dev/null | grep -E "ROOT_SERVICE_MANAGER_URL|MQTT|CLUSTER_MONGO|SYSTEM_MANAGER" || echo "cluster_service_manager not running"
```

**Key validations:**
- `SYSTEM_MANAGER_URL` in cluster containers must be the **root machine's IP or hostname**, not `localhost` or `127.0.0.1` (unless 1-DOC)
- `CLUSTER_NAME` and `CLUSTER_LOCATION` must be non-empty in cluster_manager
- `CLUSTER_ADDRESS` must be non-empty in cluster_manager and must be the **IP/hostname at which the ROOT can reach this cluster manager** (typically the cluster host's default-route IP). It must NOT be `localhost`, `127.0.0.1`, `0.0.0.0`, or a Docker-internal address like `172.17.x.x` / `172.19.x.x` (those are the docker bridge gateway and not routable from the root). In 1-DOC deployments, `CLUSTER_ADDRESS` should equal `SYSTEM_MANAGER_URL`.
- `REDIS_ADDR` must match `redis://:rootRedis@root_redis:6379` (root) or `redis://:clusterRedis@cluster_redis:6479` (cluster)
- `CLUSTER_LOCATION` format: `latitude,longitude,radius` (e.g., `48.1,11.6,1000`)

```bash
# Gateway deployments only (override-gateway.yml): TLS/token bootstrap vars
docker exec system_manager env 2>/dev/null | grep -E "GATEWAY_ENABLED|ROOT_CERT_FILE|ROOT_PUBLIC_ADDRESS|REGISTRATION_TOKEN_TTL|CLUSTER_GATEWAY_TRUST|ROTATION_GRACE_PERIOD_HOURS|CRL_REFRESH_INTERVAL_HOURS"
docker exec cluster_manager env 2>/dev/null | grep -E "GATEWAY_ENABLED|CLUSTER_CERT_FILE|ROOT_CA_FILE|ROOT_GATEWAY_TRUST|SYSTEM_MANAGER_USE_TLS|CERT_RENEW_CHECK_INTERVAL_HOURS|INTERMEDIATE_GRACE_PERIOD_HOURS|WORKER_CERT_RENEW_DAYS|CONTAINER_NAME"
docker exec cluster_cert_bootstrap env 2>/dev/null | grep -E "CLUSTER_REGISTRATION_TOKEN|ROOT_GATEWAY_TRUST|CLUSTER_NAME|CLUSTER_ADDRESS|MQTT_CONTAINER_NAME"
```

**Gateway-mode validations:**
- `ROOT_GATEWAY_TRUST` / `CLUSTER_GATEWAY_TRUST`: how an internal component verifies the peer gateway's **public** server cert. Default `system` (OS trust store, for a publicly trusted BYO cert); set to a **path inside the container** (e.g. `/certs/root-gateway-ca.crt`, a file placed in the mounted cert directory) for a privately-issued BYO cert; an explicitly empty value means the internal root CA. `ROOT_GATEWAY_TRUST=insecure` skips verification for HTTPS calls (bootstrap, cluster_service_manager), but `cluster_manager` refuses to start with it (`Invalid ROOT_GATEWAY_TRUST … not supported for the gRPC channel`) because gRPC cannot skip server verification; a path that does not exist in the container also stops it at startup. It does *not* affect the client cert presented or MQTT trust (always the internal root CA)
- `CLUSTER_REGISTRATION_TOKEN` is needed on the *first* cluster start. `cluster_cert_bootstrap` stores a hash of the last redeemed token in `registration_token.sha256`: the same token again is ignored, but a **different** token re-registers the cluster on the next start (new client cert and new intermediate CA — every worker must re-bootstrap). A new token that cannot be redeemed (expired, used, mistyped) makes the bootstrap exit 1 and the cluster does not start
- `REGISTRATION_TOKEN_TTL_MINUTES` (root, default 10) controls how long minted registration tokens stay valid
- Certificate lifecycle knobs (all optional): `ROTATION_GRACE_PERIOD_HOURS` (root, default 24) — how long the old root CA stays trusted after `/api/certs/rotate`; `CERT_RENEW_CHECK_INTERVAL_HOURS` (cluster, default 24) — how often cluster certs are checked for expiry; `INTERMEDIATE_GRACE_PERIOD_HOURS` (cluster, default 24) — how long workers may migrate after the intermediate CA is replaced, **must stay well above 10 minutes** (workers are asked to renew at most every 10 min); `WORKER_CERT_RENEW_DAYS` (cluster, default 30) — renew worker certs this long before expiry
- `MQTT_CONTAINER_NAME`, `KONG_EXTERNAL_CONTAINER_NAME`, `CLUSTER_SERVICE_MANAGER_CONTAINER_NAME` (cluster, defaults `mqtt`, `cluster_kong_external`, `cluster_service_manager`): containers cluster_manager and the bootstrap reload or restart after a renewal via the docker socket — only change them if the containers were renamed

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

In gateway mode (`override-gateway.yml`) the broker listens with TLS on 8883 and presents `mqtt_server.crt`, issued by the cluster intermediate CA (not the root-issued `cluster.crt`). Check what it presents and that it verifies for the cluster address:

```bash
# Leaf must be CN=<cluster>-mqtt-broker, issued by the cluster intermediate (CN=<cluster>...)
echo | openssl s_client -connect <CLUSTER_ADDRESS>:8883 2>/dev/null | grep -E "^ *[01] s:"
# Must print "Verify return code: 0 (ok)" (use -verify_hostname instead for a DNS name)
echo | openssl s_client -connect <CLUSTER_ADDRESS>:8883 -CAfile cluster_orchestrator/config/certs/ca.crt -verify_ip <CLUSTER_ADDRESS> 2>/dev/null | grep "Verify return code"
```

A hostname mismatch means `CLUSTER_ADDRESS` was different when `cluster_cert_bootstrap` issued the cert — restart the cluster so it is re-issued. The healthcheck unhealthy right after a re-registration usually means mosquitto still holds the previous CA's CRL; `docker kill -s HUP mqtt` reloads it (the bootstrap normally does this itself).

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

# From system_manager: can it reach jwt_generator?
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

# Check promtail can access docker socket
docker exec promtail ls /var/run/docker.sock 2>/dev/null || echo "WARN: promtail cannot access docker socket (logs won't be collected)"
docker exec cluster_promtail ls /var/run/docker.sock 2>/dev/null || echo "WARN: cluster_promtail cannot access docker socket"
```

---

## STEP 11 — API Smoke Tests

```bash
# Root System Manager API
echo "=== System Manager Health ==="
curl -s --connect-timeout 5 "http://localhost:10000/api/v1/info" 2>/dev/null | python3 -m json.tool 2>/dev/null || echo "FAIL: system_manager API not responding"

echo "=== Registered Clusters ==="
curl -s --connect-timeout 5 "http://localhost:10000/api/v1/clusters" 2>/dev/null | python3 -m json.tool 2>/dev/null | head -40 || echo "FAIL or no auth token"

echo "=== Root Scheduler Status ==="
curl -s --connect-timeout 5 "http://localhost:10004/status" 2>/dev/null || echo "FAIL: root_scheduler not responding"

echo "=== Resource Abstractor (Root) ==="
curl -s --connect-timeout 5 "http://localhost:11011/status" 2>/dev/null || echo "FAIL: root_resource_abstractor not responding"

echo "=== Cluster Scheduler Status ==="
curl -s --connect-timeout 5 "http://localhost:10105/status" 2>/dev/null || echo "FAIL: cluster_scheduler not responding"

echo "=== Resource Abstractor (Cluster) ==="
curl -s --connect-timeout 5 "http://localhost:11012/status" 2>/dev/null || echo "FAIL: cluster_resource_abstractor not responding"

echo "=== JWT Generator ==="
curl -s --connect-timeout 5 "http://localhost:10011/status" 2>/dev/null || echo "FAIL: jwt_generator not responding"
```

---

## STEP 12 — Addons System (Root Only)

```bash
echo "=== Addons Manager ==="
curl -s --connect-timeout 5 "http://localhost:11101/status" 2>/dev/null || echo "addons_manager not responding (may be disabled via override-no-addons.yml)"

echo "=== Marketplace Manager ==="
curl -s --connect-timeout 5 "http://localhost:11102/status" 2>/dev/null || echo "marketplace_manager not responding"

# Check addons_monitor docker socket access
docker exec addons_monitor ls /var/run/docker.sock 2>/dev/null || echo "WARN: addons_monitor cannot access docker socket"
```

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
docker ps -a --format "{{.Names}}" | grep -E "system_manager|mongo|root_|cluster_|jwt_|grafana|loki|promtail|mqtt|addons|marketplace|oakestra" | xargs -r docker rm -f
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

### Fix: Cluster locked out after a root CA rotation or expired certificate (gateway mode)
Mint a new cluster token at the root and restart the cluster with it — `cluster_cert_bootstrap` redeems any token it has not redeemed before:
```bash
# CLUSTER_REGISTRATION_TOKEN=<new token> SYSTEM_MANAGER_URL=<root> CLUSTER_ADDRESS=<addr> ./StartOakestraCluster.sh
```
This issues a new intermediate CA, so re-bootstrap every worker of that cluster afterwards (see the next fix).

### Fix: Worker locked out (missed an intermediate migration, or its cert expired)
Mint a worker token (`POST /api/tokens/worker`, STEP 18.4) and re-register the worker: `sudo NodeEngine -a <cluster_address> -p 8443 -s --token <token>`. The worker generates a new key and CSR; the old files are replaced.

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

## Certificate Status (gateway mode only)
- Earliest expiry: <certificate, date>
- Rotation / worker migration in progress: <none / until date>
- Renewal errors in logs: <none / which component>

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

---

## STEP 18 — Gateway, Certificates, and Registration Tokens (override-gateway.yml)

Only applies when the deployment uses `override-gateway.yml`. In gateway mode:

- All external traffic enters through Kong: root `kong_external` (:443), cluster `cluster_kong_external` (:8443 TLS, :8080 cleartext). Internal admin gateways listen on loopback only: root `kong_internal` (127.0.0.1:8000), cluster `cluster_kong_internal` (127.0.0.1:8888).
- The gateways present a **public server certificate** from `<certs>/public/fullchain.pem|privkey.pem` — a separate certificate system from the internal mTLS CA. It is **bring-your-own and required** (no auto-generated fallback); the init containers fail fast if it's missing.
- **User endpoints** need no client certificate (app-level JWT auth). **Machine-to-machine routes** (cluster registration gRPC, `/api/information`, `/api/certs/cluster-renew` at the root; `/api/net/*`, `/api/node/register`, `/api/service`, `/api/result/deploy`, `/api/certs/worker-token`, `/api/certs/worker-renew` at the cluster) require an mTLS client certificate signed by the internal root CA — without one they return `401 mTLS client certificate required`. The cluster's `/api/certs/renew` and `/api/certs/refresh` are on the internal gateway only.
- New clusters/workers obtain their certificates automatically with **one-time registration tokens** (default TTL 10 min, single use). Afterwards every certificate is renewed automatically — see 18.7 for the lifecycle and what stays manual.

### 18.1 Check the cert-init containers

```bash
# Root: cert_init checks the internal CA + requires the BYO public cert
docker logs cert_init 2>/dev/null | tail -20
# Cluster: cluster_cert_bootstrap redeems the registration token and writes all cert material
docker logs cluster_cert_bootstrap 2>/dev/null | tail -30
```

**What it means:**
- `cert_init` exits non-zero if system_manager never wrote `ca.crt` (check `docker logs system_manager`), **or if the BYO public cert `public/fullchain.pem`+`privkey.pem` is missing** — it is required, there is no fallback.
- `cluster_cert_bootstrap` redeems `CLUSTER_REGISTRATION_TOKEN` when the mTLS files are missing **or** the token differs from the last one it redeemed (`NEW CLUSTER_REGISTRATION_TOKEN — re-registering the cluster`). On a re-registration it also reloads the running `mqtt` and `cluster_kong_external`. On every start it re-signs the CRL and issues `cluster_mqtt.crt` and `mqtt_server.crt`.
- `cluster_cert_bootstrap` exits 1 with an explicit message when: the mTLS certs are missing and `CLUSTER_REGISTRATION_TOKEN` is unset; a token was rejected (invalid/expired/already used → mint a fresh one); the client cert has expired and no new token is set; or the BYO public cert is missing. Kong/mosquitto/cluster_manager **will not start** until this container succeeds.

### 18.2 Verify certificate material on disk

```bash
# Root (default ROOT_CERT_PATH=root_orchestrator/config/certs)
ls -la root_orchestrator/config/certs/ root_orchestrator/config/certs/public/
# Cluster (default CLUSTER_CERT_PATH=cluster_orchestrator/config/certs)
ls -la cluster_orchestrator/config/certs/ cluster_orchestrator/config/certs/public/
# Worker
ls -la /etc/oakestra/certs/
```

Expected files — root: `ca.crt ca.key server.crt server.key revoked.crl public/fullchain.pem public/privkey.pem`; cluster: `ca.crt cluster.crt cluster.key cluster_ca.crt cluster_ca.key registration_token.sha256 public/*`, plus `cluster_revoked.crl cluster_mqtt.crt cluster_mqtt.key mqtt_server.crt mqtt_server.key` which `cluster_cert_bootstrap` writes on every start; worker: `ca.crt worker.crt worker.key` (the key is generated on the worker). Keys must be mode 600. The `public/*` files are operator-provided (BYO) and owned by the kong user (UID 1000).

Only while a transition is running: root `ca.old.crt ca.old.key ca.old.expiry` (root CA rotation grace period; `ca.crt` then holds the new CA followed by the old one), cluster `cluster_ca.old.crt cluster_ca.old.key cluster_ca.old.expiry` (workers migrating to a new intermediate). The `*.expiry` file holds the deadline; the files disappear when it passes.

On workers, NetManager's `/etc/netmanager/netcfg.json` `MqttCert`/`MqttKey`/`MqttCa` should point at the NodeEngine-bootstrapped files in `/etc/oakestra/certs/` (worker.crt, worker.key, ca.crt).

### 18.3 Smoke-test the TLS split

```bash
# Public route, no client cert -> must succeed (-k only if the BYO cert is privately issued)
curl -ks https://<ROOT_IP>/api/certs/ca.crt | head -2
# Machine-to-machine route without client cert -> must return 401
curl -ks -o /dev/null -w "%{http_code}\n" -X POST https://<ROOT_IP>/api/information/test
# Worker registration without client cert -> must return 401
curl -ks -o /dev/null -w "%{http_code}\n" -X POST https://<CLUSTER_ADDRESS>:8443/api/node/register
# Cleartext listener on the cluster must also reject machine routes (401)
curl -s -o /dev/null -w "%{http_code}\n" -X POST http://<CLUSTER_ADDRESS>:8080/api/node/register
```

### 18.4 Registration token flow

```bash
# 1. Login as Admin and mint a cluster token (also via dashboard/CLI)
TOKEN_JSON=$(curl -ks -X POST https://<ROOT_IP>/api/tokens/cluster -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d '{}')
echo "$TOKEN_JSON"   # contains token, expires_at, suggested_command

# 2. Start the cluster with the one-liner from suggested_command, e.g.:
# CLUSTER_REGISTRATION_TOKEN=<token> SYSTEM_MANAGER_URL=<root> CLUSTER_NAME=c1 CLUSTER_LOCATION=48.1,11.6,1000 ./StartOakestraCluster.sh

# 3. Mint a worker token for a registered cluster
curl -ks -X POST https://<ROOT_IP>/api/tokens/worker -H "Authorization: Bearer $JWT" -H "Content-Type: application/json" -d '{"cluster_id": "<id from /api/clusters>"}'

# 4. Register the worker with the returned one-liner:
# sudo NodeEngine -a <cluster_address> -p 8443 -s --token <token>
```

**Failure modes:**

| Symptom | Likely cause | Fix |
|---|---|---|
| `POST /api/tokens/worker` returns 404 | cluster_id not registered yet | Wait for cluster registration (check root logs for "Cluster ID received"), use the id from `GET /api/clusters` |
| `POST /api/tokens/worker` returns 502 | Root cannot deliver the token hash to the cluster over mTLS | Check root→cluster connectivity (STEP 7.1) and that the cluster gateway is up with valid certs |
| Bootstrap endpoint returns 401 | Token invalid, expired (TTL default 10 min), or already used (single-use) | Mint a fresh token |
| Worker bootstrap returns 400 (`CSR parsing failed` / `CSR has no common name`) | NodeEngine and cluster_manager from different versions — the worker bootstrap sends a CSR (`token`, `csr`) and gets no private key back | Run matching NodeEngine and cluster versions |
| TLS handshake fails on *any* route after `/api/certs/reset` | Peer still presents a cert signed by the old root CA — nginx rejects invalid client certs even in `optional` mode | Re-bootstrap the cluster/worker with a fresh token; restart `cluster_kong_external` after replacing the cluster's `ca.crt` |
| `cert_init` / `cluster_cert_bootstrap` exits 1: "public gateway certificate not found" | No BYO public cert provided (required — there is no fallback) | Drop `public/fullchain.pem`+`privkey.pem` into `<certs>/public/` and restart |
| Browser warns about untrusted cert | The BYO public cert is privately issued (not from a public CA) | Use a publicly trusted cert (e.g. Let's Encrypt), or install your CA into the client's trust store |
| cluster_manager can't reach root: certificate verify failed | `ROOT_GATEWAY_TRUST` mismatch — default `system` but the BYO public cert is privately issued (not in the OS trust store) | Set `ROOT_GATEWAY_TRUST` to the CA-bundle path for that cert, as a path inside the container (e.g. `/certs/…`); same logic for `CLUSTER_GATEWAY_TRUST` on the root |

### 18.5 Kong gateway state

```bash
# Verify the client-verification CA entity is loaded (UUID is fixed)
docker exec kong_external curl -s http://localhost:8001/ca_certificates/cafe0000-0000-4000-8000-000000000000 | head -5
# Check guarded routes carry the pre-function plugin
docker exec kong_external curl -s http://localhost:8001/routes | python3 -c "import json,sys; [print(r['name']) for r in json.load(sys.stdin)['data']]"
# Reload after replacing certificate files on disk
docker exec kong_external kong reload
```

### 18.6 Certificate revocation lists (CRLs)

Kong (root, `revoked.crl`) and mosquitto (cluster, `cluster_revoked.crl`) reject **every** client certificate when their CRL is expired or was signed by a CA key that has since been replaced — even if nothing is revoked. CRLs are signed for 30 days. `system_manager` and `cluster_manager` re-sign them from MongoDB at startup and every `CRL_REFRESH_INTERVAL_HOURS` (default 24; the first refresh, which also reloads Kong/mosquitto, runs one minute after startup). `POST /api/certs/reset` clears all revocations and re-signs the root CRL. mosquitto only accepts MQTT client certificates issued by the cluster intermediate CA (the CRL's issuer), so the cluster's own services use `cluster_mqtt.crt` rather than the root-issued `cluster.crt`; a client certificate from any other issuer fails with `unable to get certificate CRL`.

During a transition each CRL file holds **two** CRLs — one per trusted CA — because OpenSSL rejects a certificate whose issuer has no CRL: `revoked.crl` during a root CA rotation grace period (signed by the new and old root CA), `cluster_revoked.crl` while workers migrate to a new intermediate. `grep -c "BEGIN X509 CRL" <file>` shows 2 then, and 1 otherwise. The `openssl crl … -CAfile` check below only checks the first CRL in the file.

Entries are removed automatically once the revoked certificate has expired, so the lists stay small. When a worker renews, the cluster revokes its previous certificate (`reason: superseded`, `revoked_by: renewal`) as soon as the worker's heartbeat reports the new one.

```bash
# Must print "verify OK", and nextUpdate must be in the future
openssl crl -in root_orchestrator/config/certs/revoked.crl -CAfile root_orchestrator/config/certs/ca.crt -noout -nextupdate
openssl crl -in cluster_orchestrator/config/certs/cluster_revoked.crl -CAfile cluster_orchestrator/config/certs/cluster_ca.crt -noout -nextupdate

# List / clear / un-revoke — root: Admin JWT via the internal gateway (or external :443)
curl -s http://localhost:8000/api/certs/revoke -H "Authorization: Bearer $JWT"
curl -s -X DELETE http://localhost:8000/api/certs/revoke -H "Authorization: Bearer $JWT"
curl -s -X DELETE http://localhost:8000/api/certs/revoke/<serial_hex> -H "Authorization: Bearer $JWT"
# Cluster: internal gateway only, no login
curl -s -X DELETE http://localhost:8888/api/certs/revoke
```

**What it means:** a `verify failure` or past `nextUpdate` explains blanket `401 mTLS client certificate required` (root) or rejected MQTT TLS connections (cluster). Restart `system_manager` / `cluster_manager` to re-sign immediately.

### 18.7 Certificate lifecycle and renewal

Every certificate below renews itself; the "Manual" column is all an operator has to do. Checks run at startup and then daily unless noted.

| Certificate (file) | Issued by | Lifetime | Renewed automatically | Manual |
|---|---|---|---|---|
| Root CA (root `ca.crt`/`ca.key`) | self | 10 years | No — warning from 90 days before expiry | `POST /api/certs/rotate` before it expires (see below) |
| Root identity towards clusters (root `server.crt`) | root CA | 365 days | 30 days before expiry, and when a rotation ends | — |
| Cluster client cert (cluster `cluster.crt`) | root CA | 365 days | 30 days before expiry (in-band CSR to `/api/certs/cluster-renew`), and right after a root CA rotation | — |
| Cluster intermediate CA (cluster `cluster_ca.crt`/`.key`) | root CA | 5 years (never beyond the root CA) | After a root CA rotation — warning from 90 days before expiry otherwise | Replace it before it expires (see below) |
| Cluster MQTT client identity (`cluster_mqtt.crt`) | intermediate | 365 days | 30 days before expiry; restarts `cluster_service_manager` and `cluster_manager` | — |
| MQTT broker server cert (`mqtt_server.crt`) | intermediate | 365 days | 30 days before expiry, and when a worker migration ends; mosquitto reloads without dropping connections | — |
| Worker cert (`/etc/oakestra/certs/worker.crt`) | intermediate | 365 days | `WORKER_CERT_RENEW_DAYS` (30) before expiry, or when issued by a replaced intermediate — triggered by the worker heartbeat. NodeEngine and NetManager then reconnect to the broker with the new files, without restarting | — |
| CRLs (`revoked.crl`, `cluster_revoked.crl`) | root CA / intermediate | 30 days | Re-signed daily | — |
| Public gateway certs (`public/*`) | your CA (BYO) | yours | No | Renew them yourself (`PUT /api/certs/public` on the root) |

```bash
# Expiry and issuer of every certificate (run on the root/cluster host; worker files need sudo)
for f in root_orchestrator/config/certs/{ca,server}.crt cluster_orchestrator/config/certs/{cluster,cluster_ca,cluster_mqtt,mqtt_server}.crt; do
  [ -f "$f" ] && printf '%-50s %s | %s\n' "$f" "$(openssl x509 -in "$f" -noout -enddate | cut -d= -f2)" "$(openssl x509 -in "$f" -noout -issuer | cut -d= -f2-)"
done
sudo openssl x509 -in /etc/oakestra/certs/worker.crt -noout -enddate -issuer

# Is a rotation (root) or worker migration (cluster) in progress, and until when?
for f in root_orchestrator/config/certs/ca.old.expiry cluster_orchestrator/config/certs/cluster_ca.old.expiry; do
  [ -f "$f" ] && echo "in progress until $(cat "$f"): $f" || echo "none: $f"
done

# Renewal activity
docker logs system_manager 2>&1 | grep -E "root CA expires|Root server certificate|grace period ended"
docker logs cluster_manager 2>&1 | grep -E "Root CA was rotated|[Rr]enewal|intermediate CA expires|MQTT .*certificate|Worker migration|Asking worker|Revoked certificate"
# On a worker ($NODEENGINE_LOG from 9.1; NetManager's log path is static, see 9.3)
grep -E "renewal|renewed" "$NODEENGINE_LOG"
grep -E "reconnected with the renewed worker certificate" /var/log/oakestra/netmanager.log
```

**Rotating the root CA** (root host, internal gateway):
```bash
JWT=$(curl -s -X POST http://localhost:8000/api/auth/login -H "Content-Type: application/json" \
  -d '{"username":"Admin","password":"<password>","organization":""}' | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')
curl -s -X POST http://localhost:8000/api/certs/rotate -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" -d '{"grace_period_hours": 24}'
```
Everything after that is automatic:
1. The root trusts the old and new root CA until the deadline (`ca.old.expiry`) and gives the new CA a distinct name (`Oakestra Root CA <timestamp>`).
2. Each cluster sees the new CA in the response to its next `/api/information` update (every 15 s), renews its client cert and replaces its intermediate CA.
3. Workers are asked to renew through their heartbeat and move to the new intermediate; the broker keeps its old-intermediate cert meanwhile, so workers not yet renewed still connect.
4. After `INTERMEDIATE_GRACE_PERIOD_HOURS` the cluster drops the old intermediate, switches the broker cert and restarts its own MQTT clients.
5. After `grace_period_hours` the root drops the old CA and re-issues `server.crt`. `POST /api/certs/rotate-complete` ends this early.

Clusters offline for the whole root grace period, and workers offline for the whole migration, are locked out and need a new token (STEP 15). `POST /api/certs/reset` instead replaces the root CA **without** a grace period — every cluster and worker must re-register.

**Replacing a cluster's intermediate CA** (cluster host, internal gateway; workers migrate automatically as in steps 3–4):
```bash
curl -s -X POST http://localhost:8888/api/certs/renew -H "Content-Type: application/json" -d '{"renew_intermediate": true}'
```

**Failure modes:**

| Symptom | Likely cause | Fix |
|---|---|---|
| `Automatic cluster certificate renewal FAILED: mTLS client cert rejected` | The root no longer accepts the cluster's cert (expired, or a rotation grace period ended before the cluster reported) | Re-register the cluster with a new token (STEP 15) |
| Cluster renewal keeps failing with `Could not reach root` | Root or its gateway unreachable | Fix connectivity (STEP 7.1); the cluster retries daily until the cert expires |
| Worker log `renewal failed … status 401`, cluster answers `not issued by this cluster` | The worker's cert comes from an intermediate the cluster no longer keeps (it missed the migration) | Re-bootstrap the worker with a new token (STEP 15) |
| Worker log `renewal failed … status 403` (`CSR name must match`) | The worker's cert name differs from the CSR — the cert was replaced by hand | Re-bootstrap the worker |
| Worker log `NetManager could not reconnect with the renewed certificate` | After a renewal NodeEngine calls NetManager's local `POST /mqtt/reconnect`; NetManager was not running, had not been registered, or could not reach the broker | Check NetManager is up (STEP 9.3); `sudo systemctl restart netmanager` picks the new files up as well |
| Root → cluster calls (deployments, `POST /api/tokens/worker`) fail with 400 at the cluster gateway | The root's `server.crt` does not verify against the cluster's `ca.crt` — e.g. the cluster gateway has not reloaded after a renewal | `openssl verify -CAfile cluster_orchestrator/config/certs/ca.crt root_orchestrator/config/certs/server.crt`; then `docker exec cluster_kong_external kong reload` |
| `cluster_manager` and `cluster_service_manager` restart once a year | Expected: the cluster's MQTT client identity was renewed (`Restarting cluster_manager to load the new MQTT client certificate`) | — |
