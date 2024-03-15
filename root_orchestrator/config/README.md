# Alert & Logging Configuration


## Monitoring Services
The proposed toolset for logs and alerting is based on:
- [Loki](https://grafana.com/docs/loki/latest/) is a highly-available, multi-tenant log aggregation system inspired by Prometheus. It focuses on logs instead of metrics, collecting logs via push instead of pull.
- [Promtail](https://grafana.com/docs/loki/latest/send-data/promtail/) is an agent deployed on machines running applications to ship local logs to a Grafana Loki instance. It supports native log scraping from existing Docker containers.
- [Grafana](https://grafana.com/docs/) is already in use for cluster metrics. Use Loki as data source for both logs and alerting.

The high-level composition of the service is here sketched:
![observe-arch](https://i.postimg.cc/vBZWQVLR/arch1.png)
*Promtail* perform service discovery based on labels: the root components are tagged with `logging=promtail`, while the cluster component are retrieved by the Promtail at cluster level by the label `logging=cluster_promtail`. 

At **root level**, each service is specified by:
- `loki:3100`
- `promtail`
- `grafana:3000`

At **cluster level**, each service is specified by:
- `cluster_loki:3101`
- `cluster_promtail`
- `cluster_grafana:3001`

Both two levels use different volumes for the configuration of the three services, respectively in [root_orchestrator/config/](../config/) and [cluster_orchestrator/config/](../../cluster_orchestrator/config/). Both `config` folders are structured as:
```bash
├── alerts
│   └── rules.yml           #Loki rules for alerting based on Promtail ingested logs
├── grafana-datasources.yml # Loki datasource setup
├── loki.yml                # Ingestion, storage config
├── promtail.yml            # Service discovery & static logs configuration
```
The configuration files can also be written at runtime but the volumes link allows a faster startup and configuration reload at runtime.


> ⚠️ 
> The *observability stack* can be ovverided to exclude the deployments of the three services at root/cluster deployment by:
 ```bash
 docker-compose -f docker-compose.yml -f override-no-observe.yml up --build
 ```
> ⚠️ 
> Both `loki` and `cluster_loki` logging output has been inhibited to avoid **output verbosity**. This configuration can be changed both at root/cluster level by modifying [docker-compose.yml](../docker-compose.yml) and removing:
```yaml
    logging:
      driver: none
```
### Monitoring granularity configuration
The [promtail.yml](./promtail.yml) configuration is analyzed in each section, starting from the scraper source:
```yaml
- job_name: root_logs_scraper
  docker_sd_configs: # Service discovery
  - host: unix:///var/run/docker.sock
    refresh_interval: 5s
    filters:
    - name: label
      values: ["logging=promtail"]
```
The default labels for service discovery target `docker_sd_config` has been identified as the following:
```yaml
  - source_labels: ['__meta_docker_container_id']
    target_label: 'container_id'
  - source_labels: ['__meta_docker_network_ip']
    target_label: 'container_ip'
  - source_labels: ['__meta_docker_container_name']
    regex: '/(.*)'
    target_label: 'container_name'
  - source_labels: ['__meta_docker_container_log_stream']
    target_label: 'logstream'
  - source_labels: ['__meta_docker_container_label_logging_jobname']
    target_label: 'job'

```
More base labels can be extracted based on the [docker_sd_configs](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#docker_sd_config) configuration. A careful evaluation of which labels are useful and which values can assume must be priorly done to avoid perfromance degradation at ingestion level.

#### Labels granularity
The specific logging format for each service allows also to exploit automatic filtering and extraction of labels by using [Prontail Pipeline stages](https://grafana.com/docs/loki/latest/send-data/promtail/stages/), here specified:
```yaml
  pipeline_stages:
  - json:
       expressions:
         level: level
         service: service
         filename: filename
  - regex: #Regex for `default` logging format
      expression: '(?P<level>[^\[\]]+?)(?P<timestamp>\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) (?P<service>[^:]+) (?P<file>[^:]+):(?P<line>\d+)\] (?P<message>.+)'
  - logfmt:
        mapping:
          level:
          service:
          file:
  - labels:
      level: level
      service: service
      file: line
```
If one of the format matches the ingested log lines, the specified list of labels are extracted. 

### Alarming
The [rules.yml](./alerts/rules.yml) contains the rules expression in [LogQL](https://grafana.com/docs/loki/latest/query/) based on the ***labels*** extracted by Promtail.
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
