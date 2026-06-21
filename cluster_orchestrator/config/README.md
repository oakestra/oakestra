# Alert & Monitoring Service

A detailed explanation of the shared logging and alerting configuration is available in [the Root Orchestrator configuration documentation](../../root_orchestrator/config/README.md).

Each Cluster runs its own `cluster_alloy`, `cluster_loki`, and `cluster_grafana`. Alloy collects stdout and stderr from every Compose service belonging to that Cluster and writes only to the Cluster-local Loki. It does not ship Cluster logs to the Root.

The Cluster Alloy diagnostic UI is available only from the Cluster host at `http://127.0.0.1:12346`.

The Cluster identity in Loki's `cluster_id` label is the configured `CLUSTER_NAME`. The Root-assigned database ID does not exist yet when Compose creates the services. Existing dashboards remain compatible through the `container_name`, `job`, and `logstream` aliases.

Python services use the versioned JSON contract documented by the shared [`oakestra_logging`](../../libraries/oakestra_logging/) package. Alloy collects these structured records together with raw Go and third-party output.

## Provisioned logs dashboard

Cluster Grafana automatically loads the version-controlled [`[Oakestra] Orchestrator Logs`](./dashboards/logs-dashboard.json) dashboard. Open `http://<cluster-address>:3001`, select it from **Dashboards**, and use the Cluster, Component, Level, regex search, and time-range controls to browse the Cluster-local Loki history. The **Extra LogQL pipeline** field accepts stages such as `|= "worker"` or `| json`; use the log panel's **Explore** action when a complete LogQL editor is needed. Component links provide shortcuts between the Cluster Manager, Scheduler, Resource Abstractor, and Service Manager.

The dashboard only displays logs stored in this Cluster's `cluster_loki`. It does not depend on Root Grafana and does not make Cluster logs available at the Root.
