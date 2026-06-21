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
│   └── rules.yml           #Loki rules for alerting based on Alloy-ingested logs
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

- **Cluster** and **Component** selectors populated from Loki's `cluster_id` and `compose_service` labels;
- a **Level** selector that searches both full severity names and Oakestra's compact forms such as `[I...]` and `[E...]`;
- a case-insensitive **Search (regex)** field;
- an **Extra LogQL pipeline** field for stages such as `|= "worker"`, `| json`, or `| logfmt`;
- Grafana's dashboard time picker, with presets from 5 minutes to 30 days, for scrolling back through retained logs;
- links for jumping between the managers, schedulers, resource abstractors, service managers, and observability components.

The severity selector searches the log line instead of requiring a `level` label because not every orchestrator component currently emits the same structured format. Raw lines remain searchable. For unrestricted LogQL editing, open the log panel menu and choose **Explore**; the current query and time range are carried into Grafana Explore.

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

#### Labels granularity
The existing JSON and Oakestra default-format extraction is preserved with Alloy's `loki.process` stages:

```alloy
loki.process "oakestra" {
  stage.json {
    expressions = {
      level    = "level",
      service  = "service",
      filename = "filename",
    }
  }

  stage.regex {
    expression = "(?P<level>[^\\[\\]]+?)(?P<timestamp>\\d{4}-\\d{2}-\\d{2} \\d{2}:\\d{2}:\\d{2}) (?P<service>[^:]+) (?P<file>[^:]+):(?P<line>\\d+)\\] (?P<message>.+)"
  }

  stage.labels {
    values = {
      level   = "",
      service = "",
    }
  }
}
```

If a supported format matches, `level` and `service` are extracted as labels. Raw lines are still collected when neither parser matches.

Python orchestrator containers emit versioned JSON through the shared
[`oakestra_logging`](../../libraries/oakestra_logging/) package. Its schema, level conventions,
redaction behavior, and local validation commands are documented in the
[library README](../../libraries/oakestra_logging/README.md). Collector labels such as container
identity remain separate from fields generated by the application.

### Alarming
The [rules.yml](./alerts/rules.yml) contains the rules expression in [LogQL](https://grafana.com/docs/loki/latest/query/) based on the ***labels*** extracted by Alloy.
Both the **service discovery** and **pipeline stages** labels can also be used in [rules.yml](./alerts/rules.yml) to detect specific conditions on extracted labels.

Here the `rules.yaml` configuration defining two generic alarm based on *base labels*:
```yaml
    rules: 
      - alert: SystemManagerErrorAlert #Service-specifix rule
        expr: |
          (count_over_time({container_name="system_manager"} | level = `E` [1m]) > 1)
        for: 1m
        labels:
            severity: error
            team: infrastructure
            category: logs
        annotations:
            title: "System Manager Error Log Alert"
            description: "Service reported an error in the last 1 minute"
            impact: "impact"
            action: "action"
      - alert: StderrHighLoadAlert # Output target generic rules
        expr: |
          count_over_time({logstream="stderr"} [1m]) > 10
        for: 10s
        labels:
            severity: warning
            team: infrastructure
            category: logs
        annotations:
            title: "Standard Error High Load Alert"
            description: "Service reported high load on stderr in the last 1 minute"
            impact: "impact"
            action: "action"
```
