# Alert & Logging Configuration


## Monitoring Services
The proposed toolset for logs and alerting is based on:
- [Loki](https://grafana.com/docs/loki/latest/) is a highly-available, multi-tenant log aggregation system inspired by Prometheus. It focuses on logs instead of metrics, collecting logs via push instead of pull.
- [Grafana Alloy](https://grafana.com/docs/alloy/latest/) is the collection agent. It discovers Docker containers, reads stdout/stderr, processes log lines, and forwards them to the local Loki instance.
- [Grafana](https://grafana.com/docs/) is already in use for cluster metrics. Use Loki as data source for both logs and alerting.

The high-level composition of the service is here sketched:

> **Migration notice:** This historical diagram still labels the collector as
> Promtail. Grafana deprecated Promtail and ended support on March 2, 2026. In
> this repository, the Promtail component shown below has been replaced by
> Grafana Alloy; the rest of the high-level log flow remains the same.

![observe-arch](https://i.postimg.cc/vBZWQVLR/arch1.png)
*Grafana Alloy* performs Docker service discovery. Root and Cluster Compose services carry explicit collector-scope labels, preventing co-located collectors from ingesting each other's containers. The original diagram remains conceptually valid, with Alloy replacing the Promtail collector.

At **root level**, each service is specified by:
- `loki:3100`
- `alloy`
- `grafana:3000`

At **cluster level**, each service is specified by:
- `cluster_loki:3101`
- `cluster_alloy`
- `cluster_grafana:3001`

Both two levels use different volumes for the configuration of the three services, respectively in [root_orchestrator/config/](../config/) and [cluster_orchestrator/config/](../../cluster_orchestrator/config/). Both `config` folders are structured as:
```bash
├── alerts
│   ├── grafana-rules.yml         # Grafana-managed log alert rule
│   └── grafana-contact-point.yml # Configurable webhook and email contact points
├── grafana-datasources.yml # Loki datasource setup
├── loki.yml                # Ingestion, storage config
├── config.alloy            # Alloy Docker discovery, processing, and Loki output
└── dashboards
    └── logs-dashboard.json # Provisioned orchestrator logs dashboard
```
The configuration files can also be written at runtime but the volumes link allows a faster startup and configuration reload at runtime.


> ⚠️ 
> The *observability stack* can be ovverided to exclude the deployments of the three services at root/cluster deployment by:
 ```bash
 docker-compose -f docker-compose.yml -f override-no-observe.yml up --build
 ```
> ⚠️
> Alloy positions and Loki data use named volumes. Regular container recreation preserves them; `docker compose down -v` deliberately removes the stored state and log history.

Alloy's diagnostic UI is available only from the orchestrator host at `http://127.0.0.1:12345`. It shows the component graph, health, and discovered targets. The loopback binding avoids exposing diagnostic and profiling endpoints to the deployment network.

## Provisioned logs dashboard

Grafana automatically loads the version-controlled [`[Oakestra] Orchestrator Logs`](./dashboards/logs-dashboard.json) dashboard from the mounted `config/dashboards` directory. Open Root Grafana on port `3000` and select the dashboard from **Dashboards**.

The dashboard provides:

- a **Source** selector that keeps Oakestra services, observability services, and data stores separate, with Oakestra services as the default;
- **Cluster** and **Component** selectors populated from Loki's `cluster_id` and `compose_service` labels;
- a **Level** selector backed by Alloy's normalized `level` label, with separate Debug, Info, Warning, Error, Critical, and Unparsed choices;
- a **Display** selector whose default Compact view renders schema-v1 JSON as a consistent component, message, event, context, and exception summary, while Raw restores the exact stored line;
- a case-insensitive **Full-line search (regex)** field for free text and shared IDs;
- an **Advanced field filter (LogQL)** field for exact JSON fields such as `event`, `logger`, and `context.job_id`;
- Grafana's dashboard time picker for scrolling back through retained logs. Keep each query window at 30 days or less: Grafana's generic picker can display longer ranges, but Loki 2.9 rejects a single query longer than its `30d1h` default. Older retained history remains accessible by moving an absolute window of at most 30 days backward;
- shared-ID links for searching the same 24-character Oakestra ID in managers, schedulers, resource abstractors, and service managers.

The selector matches the indexed `level` label exactly. Python fields are trusted only for expected `oakestra_logging` services when a record declares schema v1, contains the required identity fields, and its `service` matches the emitting Compose service; therefore an arbitrary JSON payload, `logger="gunicorn.error"`, or the word `ERROR` in an Info message cannot create an Error match. Other parsers are scoped to their actual emitters: MongoDB's `s`, observability logfmt, Redis markers, Nginx headers, scheduler prefixes, and strict legacy Oakestra headers. Unsupported raw records remain available under **Unparsed** instead of being guessed. Full-line search may still highlight matching text, but highlighting is not severity classification. Previously stored records are not relabelled retroactively.

High-cardinality structured fields are parsed at query time instead of indexed. For example, use `| json schema="schema_version", event_name="event" | schema="1" | event_name="cluster.registration.completed"` in **Advanced field filter (LogQL)**. The same pattern works for `logger` and nested context fields. Compact formatting happens after these filters, so searches still inspect the original record; it changes only the returned line presentation and not Loki's stored data. `| logfmt` in this box is an optional query-time parser for a selected observability component that emits logfmt, not the schema-v1 Python format. It does not create the dashboard Level label: Alloy has already normalized that label during ingestion, using a separate scoped `stage.logfmt` for known logfmt emitters. For unrestricted LogQL editing, open the log panel menu and choose **Explore**.

Root Grafana queries only the Root-local Loki. A standalone Cluster provides the same dashboard through its own Grafana and Loki instead of sending logs to the Root.

### Monitoring granularity configuration
The native Alloy configuration is stored in [config.alloy](./config.alloy).
`discovery.docker` asks the Docker daemon for containers carrying the
collector-scope label assigned by Compose:

```alloy
discovery.docker "oakestra" {
  host             = "unix:///var/run/docker.sock"
  refresh_interval = "5s"

  filter {
    name   = "label"
    values = ["oakestra.logging.collector=" + sys.env("ALLOY_COLLECTOR_ID")]
  }
}
```

`discovery.relabel` derives the labels required for log filtering:

- `container`: readable Docker container name;
- `compose_service`: stable Docker Compose service key;
- `cluster_id`: `root` at Root level and the configured `CLUSTER_NAME` at
  Cluster level.

The assigned Oakestra database ID is not available when Compose creates the containers, so `cluster_id` intentionally uses the configured Cluster name. The compatibility labels `container_name`, `job`, and `logstream` remain available for the existing dashboards and alert rules.

`loki.source.docker` reads both stdout and stderr through the Docker API and forwards them to the processing pipeline:

```alloy
loki.source.docker "oakestra" {
  host             = "unix:///var/run/docker.sock"
  targets          = discovery.relabel.oakestra.output
  relabel_rules    = discovery.relabel.docker_stream.rules
  refresh_interval = "5s"
  forward_to       = [loki.process.oakestra.receiver]
}
```

More Docker metadata is available, but labels such as full container IDs, container IPs, and source-line numbers are deliberately not indexed to avoid unnecessary label churn and cardinality.

#### Severity and field granularity

The authoritative processing pipeline is [`config.alloy`](./config.alloy). Structured Python parsing is limited to the Compose services that use `oakestra_logging` and checks the contract's identity fields before accepting `level`. The application `service` field remains inside the JSON for query-time inspection; the trusted indexed component identity is Docker's `compose_service`, avoiding user-controlled label cardinality. Separate `stage.match` blocks scope MongoDB, logfmt, Redis, Nginx, scheduler, and legacy parsing to their corresponding known Compose services. Supported aliases such as `WARN`, `E`, MongoDB `D1`–`D5`, Nginx `alert`, `FATAL`, and `panic` map into the fixed lowercase set `debug`, `info`, `warning`, `error`, and `critical`. Unknown values are deliberately left without a level label.

Only low-cardinality fields needed by normal dashboard queries are indexed. Message text, logger names, events, IDs, and context values remain inside the record and are parsed with `| json` when needed. This prevents user-controlled or highly variable data from creating Loki streams while retaining full searchability. Unsupported raw lines are still collected and appear under **Unparsed**.

Python orchestrator containers emit versioned JSON through the shared
[`oakestra_logging`](../../libraries/oakestra_logging/) package. Its schema, level conventions,
redaction behavior, and local validation commands are documented in the
[library README](../../libraries/oakestra_logging/README.md). Collector labels such as container
identity remain separate from fields generated by the application.

## Provisioned log alerting

Grafana loads [grafana-rules.yml](./alerts/grafana-rules.yml) and [grafana-contact-point.yml](./alerts/grafana-contact-point.yml) from `/etc/grafana/provisioning/alerting`. The rule is Grafana-managed and queries the local Loki datasource every 30 seconds. It counts matching lines over two minutes and creates one alert instance per `cluster_id` and `compose_service`, so every notification identifies the affected deployment and component.

The LogQL pattern recognizes structured `level=error`, `level=critical`, and JSON equivalents; uppercase `ERROR`, `FATAL`, `CRITICAL`, and `EXCEPTION`; Oakestra compact `[E...]` and `[F...]` records; Python `Traceback` and `File "...", line ...` markers; and Go `panic:`, `fatal error:`, `runtime error:`, `goroutine ...`, and `.go:<line> +0x...` stack markers. Grafana, Loki, and Alloy streams are excluded to prevent an alert-delivery failure from recursively producing more alerts.

An instance remains pending for one minute before firing. Notifications are grouped by `alertname`, `cluster_id`, and `compose_service`, wait 30 seconds for related alerts, and repeat after four hours while the problem remains active. When matching lines leave the two-minute window, the alert remains firing for one additional minute before resolving. `severity=error`, `category=logs`, and `team=infrastructure` are attached to every instance.

The contact-point file provisions both **Oakestra Alert Webhook** and **Oakestra Alert Email**. `OAKESTRA_ALERT_CONTACT_POINT` selects which one receives notifications from the rule and defaults to the webhook. The webhook reads `OAKESTRA_ALERT_WEBHOOK_URL`; its Compose default is an intentionally inactive local placeholder, so alerts still evaluate but delivery reports connection refused until a real receiver is configured:

```bash
export OAKESTRA_ALERT_CONTACT_POINT="Oakestra Alert Webhook"
export OAKESTRA_ALERT_WEBHOOK_URL=https://alerts.example.com/oakestra
docker compose -f root_orchestrator/docker-compose.yml up -d --force-recreate grafana
```

Email delivery requires both the recipient and Grafana's SMTP transport. Configure the SMTP server before recreating Grafana; keep real credentials out of tracked files:

```bash
export OAKESTRA_ALERT_CONTACT_POINT="Oakestra Alert Email"
export OAKESTRA_ALERT_EMAIL_TO=infra@example.com
export OAKESTRA_ALERT_SMTP_ENABLED=true
export OAKESTRA_ALERT_SMTP_HOST=smtp.example.com:587
export OAKESTRA_ALERT_SMTP_USER=infra@example.com
export OAKESTRA_ALERT_SMTP_PASSWORD='<smtp-password>'
export OAKESTRA_ALERT_SMTP_FROM_ADDRESS=infra@example.com
export OAKESTRA_ALERT_SMTP_FROM_NAME="Oakestra Alerts"
export OAKESTRA_ALERT_SMTP_SKIP_VERIFY=false
docker compose -f root_orchestrator/docker-compose.yml up -d --force-recreate grafana
```

Open **Alerting → Alert rules** to inspect **Orchestrator error or stacktrace detected** and **Alerting → Notification configuration → Contact points** to inspect or test either destination. Both resources are provisioned from version-controlled files and are read-only in the UI; change their structure in the repository and their environment-backed destinations at deployment time. The **Test** action sends a predefined notification without waiting for the alert rule. An end-to-end rule test uses the controlled error below and sends to the contact point selected by `OAKESTRA_ALERT_CONTACT_POINT`.

For a controlled local test, write a unique error line to a running component's main stderr, confirm the alert reaches **Pending** and then **Firing**, and wait for it to return to **Normal** after the query window and keep-firing period expire:

```bash
docker exec system_manager sh -c 'printf "ERROR issue-533-alert-test\n" > /proc/1/fd/2'
docker logs system_manager --tail 5
```

The alert instance must carry `cluster_id=root` and `compose_service=system_manager`. It should become **Pending** at the next evaluation, **Firing** after the one-minute `for` duration, and later return to **Normal**. On a standalone Cluster, run the equivalent command against `cluster_manager`; its local Grafana must show the configured Cluster name and `compose_service=cluster_manager`.
