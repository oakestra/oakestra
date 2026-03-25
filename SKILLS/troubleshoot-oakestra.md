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
docker exec cluster_manager env 2>/dev/null | grep -E "SYSTEM_MANAGER_URL|CLUSTER_NAME|CLUSTER_LOCATION|MQTT|CLUSTER_MONGO|CLUSTER_SCHEDULER|RESOURCE_ABSTRACTOR" || echo "cluster_manager not running"

docker exec cluster_scheduler env 2>/dev/null | grep -E "MANAGER_URL|RESOURCE_ABSTRACTOR|REDIS_ADDR" || echo "cluster_scheduler not running"

docker exec cluster_service_manager env 2>/dev/null | grep -E "ROOT_SERVICE_MANAGER_URL|MQTT|CLUSTER_MONGO|SYSTEM_MANAGER" || echo "cluster_service_manager not running"
```

**Key validations:**
- `SYSTEM_MANAGER_URL` in cluster containers must be the **root machine's IP or hostname**, not `localhost` or `127.0.0.1` (unless 1-DOC)
- `CLUSTER_NAME` and `CLUSTER_LOCATION` must be non-empty in cluster_manager
- `REDIS_ADDR` must match `redis://:rootRedis@root_redis:6379` (root) or `redis://:clusterRedis@cluster_redis:6479` (cluster)
- `CLUSTER_LOCATION` format: `latitude,longitude,radius` (e.g., `48.1,11.6,1000`)

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
- Jobs stuck in `CLUSTER_SCHEDULED` or `NODE_SCHEDULED` for a long time → worker not acknowledging deployment
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
# Check what IP was used to start containers
docker exec cluster_manager env 2>/dev/null | grep SYSTEM_MANAGER_URL
docker inspect oakestra-frontend-container --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null | grep API_ADDRESS

# Verify the IP is reachable
ip addr show | grep "inet " | grep -v "127.0.0.1"
```

A common issue is using `localhost` or `127.0.0.1` as SYSTEM_MANAGER_URL when cluster and root are on different machines. The actual machine IP must be used.

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

Check `LIB_BRANCH` env var — it controls which branch of the `libraries` package is used during build:
```bash
echo $LIB_BRANCH  # Should match the Oakestra version being deployed
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
docker logs cluster_manager 2>&1 | grep -iE "register|system_manager|refused|timeout" | tail -20
```

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
