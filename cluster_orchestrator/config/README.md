# Alert & Monitoring Service

A detailed explanation of the shared logging and alerting configuration is available in [the Root Orchestrator configuration documentation](../../root_orchestrator/config/README.md).

Each Cluster runs its own `cluster_alloy`, `cluster_loki`, and `cluster_grafana`. Alloy collects stdout and stderr from every Compose service belonging to that Cluster and writes only to the Cluster-local Loki. It does not ship Cluster logs to the Root.

The Cluster Alloy diagnostic UI is available only from the Cluster host at `http://127.0.0.1:12346`.

The Cluster identity in Loki's `cluster_id` label is the configured `CLUSTER_NAME`. The Root-assigned database ID does not exist yet when Compose creates the services. Existing dashboards remain compatible through the `container_name`, `job`, and `logstream` aliases.

Python services use the versioned JSON contract documented by the shared [`oakestra_logging`](../../libraries/oakestra_logging/) package. Alloy collects these structured records together with raw Go and third-party output.

## Control-plane metrics

Each Cluster also runs a local `cluster_prometheus`, `cluster_node_exporter`, and `cluster_cadvisor`. Prometheus scrapes itself, both exporters, and the existing Cluster Manager metrics endpoint every 15 seconds. node_exporter reports host CPU, memory, filesystem, disk, and network counters. cAdvisor reports the same resource families per Oakestra container, filters out unrelated Docker containers, and maps the trusted Compose labels to `compose_service` and the configured `CLUSTER_NAME` in `cluster_id`. Host-wide series use `host_scope="cluster"`.

node_exporter shares the host network namespace so its network collector sees physical host interfaces rather than only a container `eth0`; it does not share the host PID namespace. Its listener is restricted to the private internal metrics-network gateway, and the process has read-only host mounts, no Linux capabilities, and `no-new-privileges`.

Metrics are retained for at most seven days or 1 GB. Cluster Grafana reaches Prometheus through the provisioned `Prometheus` datasource. cAdvisor's diagnostic UI and raw metrics are available only from the Cluster host at `http://127.0.0.1:8082` and `http://127.0.0.1:8082/metrics`; under the host-network override Prometheus is additionally bound only to `127.0.0.1:10009`. The Root does not scrape these metrics, so every standalone Cluster keeps and displays its own history.

The metrics stack requires rootful Linux Docker Engine 25 or newer on AMD64 or ARM64. Set `DOCKER_ROOT_DIR` for a non-default Docker data directory. The Docker socket and host filesystems are mounted read-only but remain sensitive; keep cAdvisor on loopback and do not expose node_exporter. Use `override-no-observe.yml` on unsupported hosts.

```bash
docker exec cluster_prometheus promtool query instant http://127.0.0.1:9090 up
docker exec cluster_prometheus promtool query instant http://127.0.0.1:9090 prometheus_tsdb_head_series
```

## Provisioned resources dashboard

Cluster Grafana automatically loads the version-controlled [`[Oakestra] Resources`](./dashboards/resources-dashboard.json) dashboard. Its host panels show CPU usage, memory used and available, root-filesystem usage, filesystem usage by mountpoint, and disk read/write throughput from node_exporter. They always describe this Cluster's physical host and are not affected by container filters.

The container panels show cAdvisor CPU and working-set memory trends plus top-N consumers. **Cluster**, **Source**, and **Component** filter only these panels; **Source** defaults to every Oakestra-managed container stored in the Cluster-local Prometheus. Container CPU is core percentage: `100%` means one fully occupied CPU core, so an aggregated service may exceed `100%` on a multicore host. Working set is cgroup usage minus inactive file cache; it can still include active cache and is not application heap or RSS. Links between Resources, Logs, and Log Statistics preserve compatible filters and the time range. Metrics remain Cluster-local. Quick ranges stop at seven days, matching time retention; the 1-GB size-retention limit can shorten available history, and WAL and active-head data also consume disk space.

## Provisioned logs dashboard

Cluster Grafana automatically loads the version-controlled [`[Oakestra] Orchestrator Logs`](./dashboards/logs-dashboard.json) dashboard. Open `http://<cluster-address>:3001` and select it from **Dashboards**. **Source** separates Oakestra services, observability, and data stores; Cluster, Component, Level, Full-line search, and the time picker narrow the Cluster-local history. Keep each query window at 30 days or less: Grafana's generic picker can display longer ranges, but Loki 2.9 rejects a single query longer than its `30d1h` default; inspect older retained history by moving an absolute window of at most 30 days backward. Level matches Alloy's normalized label exactly, with Critical separate from Error and unsupported formats under Unparsed. **Display** defaults to a Compact component/message/event/context summary and can switch to the exact Raw record; formatting happens after filtering and does not modify Loki data. Python fields are accepted only from expected structured services with the schema-v1 identity fields and matching service-to-container identity, while MongoDB, logfmt, Redis, Nginx, scheduler, and legacy parsers are scoped to their known formats. Payload fields, logger names, and message words therefore cannot cause false severity matches. The application `service` field stays in JSON for query-time filtering; `compose_service` is the trusted indexed component. Use **Advanced field filter (LogQL)** with `| json` to query `event`, `logger`, or nested `context` fields without indexing them. Its optional `| logfmt` parser is only for a selected local observability component that emits logfmt. Shared-ID links retain the time range and search the same Oakestra ID in another component; they are correlation shortcuts rather than distributed traces. Use the panel's **Explore** action for the full LogQL editor.

The dashboard only displays logs stored in this Cluster's `cluster_loki`. It does not depend on Root Grafana and does not make Cluster logs available at the Root.

## Provisioned log statistics dashboard

Cluster Grafana also loads the version-controlled [`[Oakestra] Log Statistics`](./dashboards/log-statistics-dashboard.json) dashboard. It shows selected-range totals, average throughput, component and Cluster trends, top-N components by average warning or error/critical lines per second, and a distribution of Alloy's normalized `debug`, `info`, `warning`, `error`, and `critical` labels. **Unparsed** counts retained records without a recognized level rather than inferring severity from message text. Cluster, Source, and Component filters are shared with the Logs dashboard, and links between both dashboards preserve those filters and the time range. The Source selector defaults to Oakestra services and can include local observability services, data stores, or all collected containers. The data remains Cluster-local, and each query range must stay at 30 days or less because Loki 2.9 rejects a single query longer than `30d1h`.

## Provisioned log alerting

Cluster Grafana provisions the same Grafana-managed error and stacktrace rule against `cluster_loki`. Every alert instance is grouped by the configured `CLUSTER_NAME` in `cluster_id` and by `compose_service`, waits one minute before firing, and retains the firing state for one minute after the two-minute log window clears. The rule covers structured error levels, uppercase error tokens, Oakestra compact error records, Python tracebacks, and Go panic/stack markers.

Both **Oakestra Alert Webhook** and **Oakestra Alert Email** are provisioned. Select one with `OAKESTRA_ALERT_CONTACT_POINT`, configure either `OAKESTRA_ALERT_WEBHOOK_URL` or the `OAKESTRA_ALERT_EMAIL_TO` plus `OAKESTRA_ALERT_SMTP_*` variables, and recreate `cluster_grafana`. Inspect or test both destinations under **Alerting → Notification configuration → Contact points**. A controlled test can emit `ERROR manual-alert-test` through `/proc/1/fd/2` in `cluster_manager`; the alert must carry that Cluster's name and `compose_service=cluster_manager`, transition from **Pending** to **Firing**, and later recover to **Normal**. See the [Root configuration documentation](../../root_orchestrator/config/README.md#provisioned-log-alerting) for the complete environment examples, pattern, notification grouping, and test procedure.
