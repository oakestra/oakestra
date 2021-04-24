# Cloud-components

By design, the centralized Root Orchestrator contains

- a System Manager for user interaction with the system
- a Database (mongodb) to store data about the participating clusters
- a Scheduler which calculates task-to-cluster affinity
- some Monitoring solution (first prototype shows read-only Grafana Dashboards which collect metrics from Prometheus instances on each cluster orchestrator)

During implementation, some components could be changed/renamed/replaced/merged.

## Usage

- Use the docker-compose.yml with `docker-compose up` to start the cloud components.
