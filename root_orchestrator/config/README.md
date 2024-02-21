# Alert/Logging Service
A service that collects alarms and notifications from the platform. This service can be used to signal things to the **app provider** or **infrastructure manager**.
## Overview

This memo briefly describe a proposal and possible architecture for integrating Loki and Promtail with Grafana for efficient log aggregation and visualization.

## \#Issue 225
### Requirements


A service that collects alarms and notifications from the platform. This service can be used to signal things to the app provider or infrastructure manager.
This service can be part of root and cluster orchestrator and:

- At root level, collects cluster's alarms and notifications + internal root notifications
- At cluster level, collects node alarms and notifications + internal cluster notifications

We need to define:

- **Alarms types/priorities** (e.g., Failure, SLA violation...)
- **Notification types** (e.g., deployment succeeded, new worker node)
- **Scope**s (e.g., cluster, worker, root)
- **Propagation strategies** (E.g., how to send the notification from a worker node?)

## [Proposal 1](https://github.com/oakestra/oakestra/issues/225#issuecomment-1945745102) \#225
Loki+Promtail in combination with Grafana
- Loki is a highly-available, multi-tenant log aggregation system inspired by Prometheus. It focuses on logs instead of metrics, collecting logs via push instead of pull.
- Promtail is an agent deployed on machines running applications to ship local logs to a Grafana Loki instance. It supports native log scraping from existing Docker containers.
- Grafana is already in use for cluster metrics. This proposal suggests setting up a new data source (Grafana logs) for logs visualization.

### Architecture

#### Root Orchestrator

1. **Custom Grafana Configuration:**
   - Introduce a custom configuration for the Grafana container with a pre-defined root Loki data source.

   Example:
   ```yaml
   datasources:
     - name: MyLoki
       type: loki
       url: http://loki:3100
       access: proxy
       jsonData:
         maxLines: 1000


# Checklist
Most of the consideration here reported apply also to **Cluster Orchestrator** components. 
## Discussion
The proposed solution involving Loki, Promtail and Grafana allows only to filter the container logs based on filtering at query level as adding more `labels` (*beside `container_name`, `logstream`, `jobs` and other few relevant parameter*) in [promptail.yml](./promtail.yml) results in degrading performance (*as [Grafana suggest](https://grafana.com/blog/2020/08/27/the-concise-guide-to-labels-in-loki/)*). 

Query-based filtering (*by LogQL*) allows *only* to filter based on the `labels` and, most relevant, specific conditions reported in the `line` parameter (*e.g. specifying, fixed a `container_name` or `logstream`, if a log `line` contains **warning**, **errno**, **HTTP 400**, etc*). Here an example from [./promtail.yml](promtail.yml) of some relevant and non relevant `labels` extraction (*full list on [Prometheus configuration](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#docker_sd_config)*):
```yaml
      - source_labels: ['__meta_docker_container_name']
        regex: '/(.*)'
        target_label: 'container_name'
      - source_labels: ['__meta_docker_container_log_stream']
        target_label: 'logstream'
      - source_labels: ['__meta_docker_container_label_logging_jobname']
        target_label: 'job'
      - source_labels: ['__meta_docker_port_private']
        target_label: 'container_port_private'
      - source_labels: ['__meta_docker_port_public']
        target_label: 'container_port_public'
      - source_labels: ['__meta_docker_port_public_ip']
        target_label: 'container_port_public_ip'
```

Regarding the **alarming** feature, the described context allows to define default alarm based on `labels` and manipulations of the most relevant attribute `line` (*usually specified in LogQL*) previously extracted from container logs. An example of [alarming rule](https://grafana.com/blog/2020/08/27/the-concise-guide-to-labels-in-loki/) can be found in [alerts/alert.yaml](./alerts/alert.yaml). 

#### Cluster Orchestrator
As required by the decoupling and avoid overload of information from cluster component to root, the same considerations apply for cluster orchestrator's components: the components to monitor has been tagged with a different label so the cluster-specific logging services (*mainly `cluster_loki`, `cluster_promtail`, beside `cluster_grfana`*) perform service discovery only on the tagged instances.

As stated in the general requirements, also the worker node propagation has been evaluated against Promtail [scrape_configs](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#scrape_config) that offers mainly *docker, docker_swarm, file, http, k8s, openstack, etc* service discovery to monitor log of several type of instances, along the widely-used and supported logging on `*.log` volumes.


## Implemented
Update the `docker-compose.yml` adding:
- `grafana/loki:2.9.2` container (*[Simple Scalable Deployment](https://grafana.com/docs/loki/latest/get-started/deployment-modes/), useful for memory, balancing and replica configu at [Grafana Loki Config Nuances](https://medium.com/lonto-digital-services-integrator/grafana-loki-configuration-nuances-2e9b94da4ac1)*)
- `grafana/promtail:2.9.2` container

Root components istances (`root_service_manager`, `system_manager`, `cloud_scheduler`, `resource_abstractor`) has been tagged with `logging=promtail` to allows Promtail to perform service discovery and monitor the logs only of those istances. 

### Filtering
Fine-grained information can be extracted from logs by **Promtail** [Pipeline stages](https://grafana.com/docs/loki/latest/send-data/promtail/stages/), allowing to define **parsing, transform, action and filtering** stages based on the log format. Regarding the root components, they emit mainly logs following the format:
- *Cloud scheduler*: `"%(asctime)s - - - %(name)s - %(levelname)s - %(message)s"`
- *System manager*: `"%(asctime)s - %(name)s - %(levelname)s - %(message)s"`

Non-default logs are emitted specifying `stdout`,`stderr` or using `printf` with direct error messages (*e.g. `logging.error("Calling network plugin " + request_addr + " Connection error.")`*).

Pipeline stages has been defined as follows (*source: [./promtail.yml](./promtail.yml)*):
```yaml
      - regex: #Regex cloud scheduler default log
          expression: '^(?P<timestamp>.*?) - - - (?P<logger_name>.*?) - (?P<log_level>.*?) - (?P<message>.*)$'
          labels:
            timestamp: timestamp
            logger_name: logger_name
            log_level: log_level
            message: message
      - regex: #Regex for system manager default log
          expression: '^(?P<timestamp>.*?) - (?P<logger_name>.*?) - (?P<log_level>.*?) - (?P<addr>.*?) - - (?P<date>.*?) (?P<time>.*?) (?P<message>.*)$'
          labels:
            timestamp: timestamp
            logger_name: logger_name
            log_level: log_level
            message: message
```
but despite the [Promtail Troubleshooting & Validation](https://grafana.com/docs/loki/latest/send-data/promtail/troubleshooting/#inspecting-pipeline-stages), it results in *unsuccessfull parsing stage* as seems that the non-uniformity of logs does not allows to filter out the matching logs. The different logs structure involves defining ad-hoc [regex (Go RE2)](https://grafana.com/docs/loki/latest/query/log_queries/#regular-expression) in combination with different pipeline stages (*e.g. `docker{}, cri{}, multiline` etc*).  An example of complex parsing is briefly described in [Grafana Loki Parsing Logs Blog](https://grafana.com/blog/2020/10/28/loki-2.0-released-transform-logs-as-youre-querying-them-and-set-up-alerts-within-loki/). Some possible solutions:
- Uniform logging using `json.dumps()`, allowing simple `json: {}` pipeline
- Define specific multistage pipelines, customizing the `regex` expressions to capture relevant info for each component
- Filter relevant info in Grafana dashboard at visualization level
### Alarming
Alarm has been evaluated by defining Loki [./rules.yml](./alerts/rules.yml): the alert definition and evaluation can be defined by LogQL queries, based on Promptail base `labels` or define complex query to extract info from `line` label. 


## Tested
Logs from root components can be filtered based on `line` text content, as shown:
![Monitoring](https://i.postimg.cc/vTLxxxSw/monitoring-root.jpg)

Filtering at visualization level, like
```regex
{job="containerlogs"} | regexp `^(?P<timestamp>.*?) - (?P<logger_name>.*?) - (?P<log_level>.*?) - (?P<ip>.*?) - - \[(?P<time>[^\]]*)\] (?P<message>.*)$` 
```
allows to obtain the following labels on a given example log:
![](https://i.postimg.cc/vmRLK956/gnome-shell-screenshot-ci1z6e.png)
Loki alerts defined in [rules.yml](./alerts/rules.yml) are correctly loaded by Grafana:
![Alerts](https://i.postimg.cc/1t1HN9nQ/gnome-shell-screenshot-axktf5.png)

The same considerations apply also to cluster components. Here an example of Loki-defined alarm firing on `cluster_manager` log fire condition:
![](https://i.postimg.cc/MZ9449Tf/gnome-shell-screenshot-919e9b.png)

## Proposal 2 - [SigNoz](https://signoz.io/docs/) \#TBA
SigNoz is an open-source OpenTelemetry-compliant observability tool that helps you monitor your applications and troubleshoot problems. It provides traces, metrics, and logs under a single pane of glass. It is available both as an open-source software and a cloud offering.

With SigNoz, you can:

- Visualise Traces, Metrics, and Logs in a single pane of glass
- Monitor application metrics like p99 latency, error rates for your services, external API calls, and individual endpoints.
- Find the root cause of the problem by going to the exact traces which are causing the problem and see detailed flamegraphs of individual request traces.
- Run aggregates on trace data to get business-relevant metrics
- Filter and query logs, build dashboards and alerts based on attributes in logs
- Monitor infrastructure metrics such as CPU utilization or memory usage
- Record exceptions automatically in Python, Java, Ruby, and Javascript
- Easy to set alerts with DIY query builder

Interesting benchmarking evaluations ([Ingestion benchmark results](https://signoz.io/blog/logs-performance-benchmark/?utm_source=github-readme&utm_medium=logs-benchmark))

### Pros
- OSS, OTel compliant
- Zero-configuration startup for logs/metrics of K8s clusters
- Automatic computation of 99p, 95p and multiple error metrics
- Large community, active Slack channel for support directly from mantainers

### Cons
- Frequent update break previous versions
- Not extensively documented as Grafana OSS-based stack
- Limited set of features regarding alarming/logging/metrics

***N.B.: alerting only on [metrics](https://signoz.io/docs/userguide/alerts-management/), not on pure logs***


## Proposal 3 - LGTM stack \#TBA
- Loki (Log Aggregation)
- Grafana Dashboard (Telemetry Visualization)
- Tempo (Trace Aggregation)
- Mimir (Metrics Aggregation)


### Resources
- [e2e LGTM stack](https://levelup.gitconnected.com/setting-up-an-end-to-end-monitoring-system-with-grafana-stack-lgtm-1c534ebdf17b)
- [Scale Observability with Mimir, Loki and Tempo - ObservabilityCON](https://grafana.com/go/observabilitycon/2022/lgtm-scale-observability-with-mimir-loki-and-tempo/)
## Explore resources
- [Grafana Loki parse nginx-like logs](https://grafana.com/blog/2021/08/09/new-in-loki-2.3-logql-pattern-parser-makes-it-easier-to-extract-data-from-unstructured-logs/)
- [Processing Log Lines - Grafana Loki Github](https://github.com/jafernandez73/grafana-loki/blob/master/docs/logentry/processing-log-lines.md)
- [LGMPP - Github](https://github.com/wick02/monitoring/tree/main)
