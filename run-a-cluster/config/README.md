# Alert & Logging Configuration


## Monitoring Services
The proposed toolset for logs and alerting is based on:
- [Loki](https://grafana.com/docs/loki/latest/) is a highly-available, multi-tenant log aggregation system inspired by Prometheus. It focuses on logs instead of metrics, collecting logs via push instead of pull.
- [Promtail](https://grafana.com/docs/loki/latest/send-data/promtail/) is an agent deployed on machines running applications to ship local logs to a Grafana Loki instance. It supports native log scraping from existing Docker containers.
- [Grafana](https://grafana.com/docs/) is already in use for cluster metrics. Use Loki as data source for both logs and alerting.

The high-level composition of the services is here sketched:
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
The specified [logging format](#logging) (*later specified*) allows also to exploit automatic filtering and extraction of labels by using [Prontail Pipeline stages](https://grafana.com/docs/loki/latest/send-data/promtail/stages/), here specified:
```yaml
  pipeline_stages:
  - json: # Stage for `json` log format
       expressions:
         level: level
         service: service
         container_id: container_id
         filename: filename
         context: context
  - regex: #Stage for `default` log format
      expression: '(?P<level>[^\[\]]+?)(?P<timestamp>\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) (?P<service>[^:]+) (?P<file>[^:]+):(?P<line>\d+)\] (?P<message>.+)'
  - labels:
      level:
      service:
      file:
  - logfmt: #Stage for `log` log format
      mapping:
        level:
        service:
        container_id:
        file:
```
If one of the format matches the ingested log lines, the specified list of labels are extracted. 
The [rules.yml](./alerts/rules.yml) contains the rules expression in [LogQL](https://grafana.com/docs/loki/latest/query/) based on the ***labels*** extracted by Promtail.
Both the **service discovery** and **pipeline stages** labels can also be used in [rules.yml](./alerts/rules.yml) to detect specific conditions on extracted labels.

## Logging 
This section briefly provides an overview of logging approach implemented at the moment of this PR. Most of the format and guidelines here specified are assumed be valid for all services both at root and cluster level.

The logging is perfomed by a wrapper of `logging` library that implement the specification here described.  
The logging wrapper support three format (*at the moment*):
- `default`
- `json`
- `logfmt`


The format configuration can be explictly setted in the logging module for each service (*e.g [sm_logging.py](../system-manager-python/sm_logging.py) for `system_manager`*); by default, the `default` format is used. For the purpose of modularity, a `CustomLogger` is used in (*optional*) combination with three `CustomFormatter` to be able to support seamlessly the conversion from a format to another. 


The `default` logging format have the following structure:
```bash
	[Lmmdd hh:mm:ss.uuuuuu svc file:line] msg key1=value1 key2=value2 ...
```
where the fields are defined as follows:
```bash
	L                   A single character, representing the log level (eg 'I' for INFO)
	mm                  The month (zero padded; ie May is '05')
	dd                  The day (zero padded)
	hh:mm:ss.uuuuuu     Time in hours, minutes and fractional seconds
	svc                 The service name (e.g. `system_manager`)
	file                The file name
	line                The line number
	msg                 The user-supplied message
    extra               Contextual parameters that can be passed by using `extra` dict
```
The folowwing format is encoded in the `standard` format. As a `logging` wrapper, it support the [logging levels](https://docs.python.org/3/library/logging.html#levels) `INFO`, `DEBUG`, `WARNING`, `ERROR`, `CRITICAL` and `NOTSET`.  
The following call:
```python
ctx = {"url": url, "headers": headers, "data": data}
logger.info("HTTP POST request sent", extra={'context': ctx})
```
That generate the following log line:
```bash
[I2024-02-28 10:43:27 system_manager wsgi.py.py:639] HTTP POST request sent url=https://192.168.1.5/api/node/register headers={'Content-Type': 'application/json'} data={...}
```
The `extra` optinal dictionary in combination with the `context` dictionary allows to log additional contextual information and, if the configured according to [labels granularity](#labels-granularity) allows filtering and/or alerting on those fine-grained parameters.

According to the specification, the `json` format output:
```json
{
  "level": "INFO",
  "timestamp": "2024-02-28T09:43:27.000Z",
  "service": "system_manager",
  "filename": "wsgi.py",
  "line_no": 639,
  "message": "HTTP POST request sent",
  "context": {
    ...
  }
}

```
While for `logfmt` format:
```log
level=INFO ts=1651254607.000176 service=system_manager file=wsgi.py file_no=639 message='HTTP POST request sent' url=https://192.168.1.5/api/node/register headers='{"Content-Type": "application/json"}' data='{"key1": "value1", "key2": 123}'
```

## Next step \#note
- Refactor log lines of each service
  - Currently only logging features for `cloud_scheduler`, `system_manager`, `cluster_manager`
  - Test if the defined format is suitable for each relevant information extraction
    - *e.g. Incoming/Outgoing HTTP req, internal cluster updates, deployment descriptor, etc* 
- Where a logging module is not set up, integrate it and review all the service log lines
  - Remove print, redirect stdout/stderr, aggregate/disaggregate information log where needed
- Test OpenTelemetry toolset for format-agnostic observability, relieving Oakestra of the weight of the grafana stack:
  - [Automatic Instrumentation](https://opentelemetry.io/docs/languages/python/automatic/example/), may allow to traces with zero-code 
  - [Collector/Exporter](https://opentelemetry-python.readthedocs.io/en/latest/exporter/otlp/otlp.html): the idea is have OpenTelemetry Collector as log exporter + Promtail for ingestion. Interesting is [automatic instrumentation](https://opentelemetry.io/docs/languages/python/automatic/example/) of Python services.

## Explore resources
- [e2e LGTM stack](https://levelup.gitconnected.com/setting-up-an-end-to-end-monitoring-system-with-grafana-stack-lgtm-1c534ebdf17b)
- [Scale Observability with Mimir, Loki and Tempo - ObservabilityCON](https://grafana.com/go/observabilitycon/2022/lgtm-scale-observability-with-mimir-loki-and-tempo/)
- [Grafana Loki parse nginx-like logs](https://grafana.com/blog/2021/08/09/new-in-loki-2.3-logql-pattern-parser-makes-it-easier-to-extract-data-from-unstructured-logs/)
- [Processing Log Lines - Grafana Loki Github](https://github.com/jafernandez73/grafana-loki/blob/master/docs/logentry/processing-log-lines.md)
- [LGMPP - Github](https://github.com/wick02/monitoring/tree/main)
- [Play with Mimir](https://grafana.com/tutorials/play-with-grafana-mimir/) 
- [Loki getting started](https://github.com/grafana/loki/tree/main/examples/getting-started)
- [New in Grafana Loki 2.4](https://www.youtube.com/watch?v=M8nYWBpbwWg)
- [Ward Bekker's gist](https://gist.github.com/wardbekker/6abde118f530a725e60acb5adb04508a)
- [Getting started with Grafana Mimir](https://www.youtube.com/watch?v=pTkeucnnoJg)
- [High Level solution for node monitoring with Loki and OTel - Reddit](https://www.reddit.com/r/kubernetes/comments/16y26t4/comment/kf3amq1/?utm_source=share&utm_medium=web3x&utm_name=web3xcss&utm_term=1&utm_content=share_button) \#relevant
- [Python Instrumentation with OTel](https://opentelemetry.io/docs/languages/python/automatic/example/) \#relevant - Support Flask, FastAPI and other Python frameworks
- [Traces with Tempo and Otel](https://tracetest.io/blog/building-an-observability-stack-with-docker#home) \#relevant
- [Python logs with OTel and LOki](https://signoz.io/blog/sending-and-filtering-python-logs-with-opentelemetry/)
- [OTel metrics python](https://intellitect.com/blog/opentelemetry-metrics-python/)
- [MLTP Grafana Example](https://github.com/grafana/intro-to-mltp) \#relevant
- [Grafana Instrumenting for tracing](https://grafana.com/docs/tempo/latest/getting-started/instrumentation/?pg=oss-tempo&plcmt=resources)
- [OTel at Grafana](https://grafana.com/docs/opentelemetry/?pg=oss-tempo&plcmt=resources)
- [Automatic Grafana Dashboards](https://stackoverflow.com/questions/63518460/grafana-import-dashboard-as-part-of-docker-compose)
- [Ready dashboards](https://levelup.gitconnected.com/initialize-grafana-inside-the-docker-container-with-a-ready-dashboard-a90eb76f75a4)