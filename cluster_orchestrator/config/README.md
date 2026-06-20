# Alert & Monitoring Service

A detailed explanation of the shared logging and alerting configuration is
available in [the Root Orchestrator configuration documentation](../../root_orchestrator/config/README.md).

Each Cluster runs its own `cluster_alloy`, `cluster_loki`, and `cluster_grafana`. Alloy collects stdout and stderr from every Compose service belonging to that Cluster and writes only to the Cluster-local Loki. It does not ship Cluster logs to the Root.

The Cluster identity in Loki's `cluster_id` label is the configured
`CLUSTER_NAME`. The Root-assigned database ID does not exist yet when Compose creates the services. Existing dashboards remain compatible through the `container_name`, `job`, and `logstream` aliases.
