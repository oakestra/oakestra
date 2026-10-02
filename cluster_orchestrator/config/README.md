# Alert & Monitoring Service

A detailed explanation of the shared logging and alerting configuration is available in [the Root Orchestrator configuration documentation](../../root_orchestrator/config/README.md).

Each Cluster runs its own `cluster_alloy`, `cluster_loki`, and `cluster_grafana`. Alloy collects stdout and stderr from Compose services belonging to that Cluster and writes only to the Cluster-local Loki. It does not ship Cluster logs to the Root.

`cluster_grafana`, `cluster_loki`, and `cluster_alloy` opt out of collection with
`oakestra.logging.enabled: "false"`. Their logs remain available through
`docker compose logs cluster_grafana cluster_loki cluster_alloy`. Override the
label to `"true"` and recreate the service when centralized diagnostics are
needed, as described in the Root configuration documentation.

The Cluster Alloy diagnostic UI is available only from the Cluster host at `http://127.0.0.1:12346`.

The Cluster identity in Loki's `cluster_id` label is the configured `CLUSTER_NAME`. The Root-assigned database ID does not exist yet when Compose creates the services. Existing dashboards remain compatible through the `container_name`, `job`, and `logstream` aliases.
