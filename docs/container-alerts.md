# Container lifecycle alerts

Container lifecycle monitoring complements log alerts: it detects missing running containers and automatic Docker restarts even when an application emits no useful error log. It does not replace application health checks or prove that a running service is responsive.

## Collection and desired state

Each standalone Root or Cluster uses a local Docker-state exporter and its existing Prometheus and Grafana. A 1-DOC deployment uses one exporter for the shared Docker host. cAdvisor continues collecting resource usage; it does not provide the Docker restart counter required here.

The exporter reads Docker container state, including stopped containers, and the Docker restart-policy counter. Prometheus joins those observations with allowed Docker metadata and records metrics carrying `cluster_id`, `compose_service`, and `compose_project`. The project label prevents similarly named services in different Compose projects from being combined.

Observed state alone cannot detect a deleted or never-created container. The inventory generator reads the resolved Compose configuration and writes `oakestra_expected_container_replicas` into a node_exporter textfile. It includes enabled Oakestra-labelled services and their desired replica counts. Services disabled with zero replicas, inactive profiles, or the label `oakestra.monitoring.ignore: "true"` are excluded. The inventory contains service identities and replica counts, not environment variables or credentials.

The inventory remains on disk when a container disappears. Removing a container therefore does not automatically remove the expectation that it should exist. Regenerate the inventory whenever an intentional deployment change adds, removes, scales, or disables services.

## Deployment

The startup scripts generate the inventory before starting an observed deployment. Python 3 is required on the Docker host for this step; the generator uses only the standard library. Deployments using `override-no-observe.yml` do not require inventory generation.

When using Compose directly, generate the inventory from the same files, overrides, project name, environment, and profiles that you use for startup. Run this example from the repository root:

```bash
set -o pipefail
compose=(docker compose -f root_orchestrator/docker-compose.yml)
"${compose[@]}" config --format json |
  python3 scripts/utils/generateContainerInventory.py \
    --output root_orchestrator/config/container-inventory/containers.prom
"${compose[@]}" up -d
```

For standalone Cluster, substitute `cluster_orchestrator/docker-compose.yml` and the Cluster inventory directory. For 1-DOC, use `run-a-cluster/1-DOC.yaml` and `run-a-cluster/config/container-inventory/containers.prom`. The Root manifest under `run-a-cluster/root-orchestrator.yml` uses that same directory.

Add any `-f` overrides and `-p` project option to the `compose` array before both commands. For profiles, export `COMPOSE_PROFILES` so both Compose and the generator receive the same selection, or pass matching profiles explicitly to the generator with `--profiles`. A partial `docker compose up service_name` does not change the inventory: it still expects the full configured deployment.

Inventory replacement is atomic. Invalid configuration must fail generation rather than install a partial file. Do not run separate deployments against the same inventory directory. For planned maintenance, silence the relevant Grafana alerts; do not delete monitoring expectations just to hide an unplanned failure.

## Alert rules

| Rule | Condition | Pending duration |
|---|---|---|
| Expected container is not running | Desired replicas exceed observed running replicas | 1 minute |
| Container restarted automatically | The restart-policy counter increased within five minutes | None |
| Container monitoring unavailable | Docker-state or node_exporter collection is unavailable, or its data collection reports an error | 1 minute |
| Expected container inventory is missing | Monitoring is available but the inventory is absent or contains no expected services | 1 minute |

Missing-container and restart alerts carry the cluster, Compose service, and Compose project. Monitoring-input alerts describe the local monitoring system; they must not invent a cluster identity for a shared physical host.

The missing-container rule covers stopped, restarting, paused, dead, never-created, and deleted containers whenever fewer replicas are running than expected. A brief transition shorter than the pending duration may not notify. Extra running replicas do not count as a failure.

The restart rule detects Docker automatic restart-policy events, not an exact crash count. Repeated increases can indicate a crash loop. A manual `docker restart`, container recreation, or failure before the first observed counter sample is not reliably counted by this signal. A container left stopped is still detected through the inventory. Counter resets must not be interpreted as new restarts.

Prometheus scrapes and evaluates recording rules every 15 seconds; Grafana evaluates alerts every 30 seconds. Notifications also have grouping delays, so the pending duration is not a promise of exact end-to-end delivery latency. Alerts keep firing for one minute after their condition clears, with a 30-second notification group wait, five-minute group interval, and four-hour repeat interval.

The rules reuse the existing `OAKESTRA_ALERT_CONTACT_POINT` selection and webhook/email provisioning. Configure a real destination using the deployment's contact-point instructions. The placeholder webhook is not a working notification receiver. Local monitoring cannot notify if its own Grafana, Prometheus, Docker host, or notification network is completely unavailable; independent external monitoring is needed for that failure mode.

## Security, resource cost, and scope

The exporter image is pinned to version 0.7.0 and its multi-platform manifest digest. It runs without privileged mode or Linux capabilities, with a read-only root filesystem, no-new-privileges, and a read-only Docker socket mount. It has no published host port, including in host-network deployments. Its limits are 64 MiB memory, no additional swap allowance, 0.25 CPU, and 128 processes/threads.

A read-only socket mount does **not** enforce read-only Docker API operations. Any process with access to that socket must be trusted with privileged Docker daemon access. The isolation settings reduce exposure but do not turn the socket into a restricted API. Do not expose the exporter publicly.

The upstream exporter enumerates the host's containers and also requests resource statistics internally. Dropping those resource samples in Prometheus avoids duplicate storage but does not eliminate those Docker API calls. The retained raw lifecycle series can include names of unrelated host containers. Scoped recording rules and alerts restrict actionable service data through the permitted collector metadata and generated inventory. They are not a security boundary for someone who already has access to the raw Prometheus datasource.

The 64-MiB limit and CPU cap protect the host, but sufficiently large or slow Docker installations may cause exporter failures or scrape timeouts. The monitoring-unavailable alert reports that loss of coverage instead of treating it as proof that all services stopped. Any Docker inspection or statistics failure reported by the exporter, including one involving an unrelated host container, can temporarily suspend lifecycle detection. Measure overhead on the target host before increasing limits. Existing application code, orchestration behavior, cAdvisor resource collection, Prometheus retention, and log-alert semantics are not changed by lifecycle detection.

## Access from a remote workstation

When using VS Code Remote SSH, run Docker and inventory commands in the remote terminal on the Linux host. A Windows browser's `127.0.0.1` is not the Linux host's loopback address. Forward the existing Grafana port through VS Code's Ports view or an SSH tunnel and open the displayed local address. Do not publish the Docker-state exporter to make remote testing easier; Grafana Explore and Alerting already expose the relevant results through the configured Prometheus datasource.

## Verification

First check Prometheus Targets: `docker-state` and `node-exporter` must be up. Inspect these expressions in Grafana Explore using the Prometheus datasource:

```promql
oakestra:container_monitoring_ready
oakestra_expected_container_replicas
oakestra:container_missing_replicas
oakestra:container_restarts_5m
```

The readiness series should be 1. The inventory should match your enabled services and replicas. A healthy, stable deployment should have zero missing replicas and zero recent automatic restarts. An empty inventory is not a successful health check. During a monitoring outage, per-service alert instances may enter recovery because their queries are deliberately suppressed; that is not proof that the containers recovered. Check the separate monitoring-unavailable rule before interpreting a resolution.

In Grafana Alerting, verify that all four container rules are provisioned and have no evaluation errors. For an end-to-end test, use a disposable test deployment and its own inventory, Prometheus, Grafana, and local webhook receiver. Check running → stopped → Pending → Firing, recovery, deletion while the expectation remains, automatic restart-counter increases, and collector failure. Never stop production managers merely to test alerting.

The offline checks can be run without changing a deployment:

```bash
python3 -m unittest discover -s scripts/utils/tests -v
docker run --rm --entrypoint promtool \
  -v "$PWD/root_orchestrator/prometheus:/etc/prometheus:ro" \
  --workdir /etc/prometheus/tests \
  prom/prometheus:v3.13.2-distroless \
  test rules container-lifecycle.test.yml
```

Implementation references: [Docker Engine API](https://docs.docker.com/reference/api/engine/version/v1.46/), [pinned Docker exporter source](https://github.com/davidborzek/docker-exporter/tree/v0.7.0), and [node_exporter textfile collector](https://github.com/prometheus/node_exporter#textfile-collector).
