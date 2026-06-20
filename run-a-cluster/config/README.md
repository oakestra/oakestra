# Alert & Logging Configuration


## Monitoring Services
The proposed toolset for logs and alerting is based on:
- [Loki](https://grafana.com/docs/loki/latest/) is a highly-available, multi-tenant log aggregation system inspired by Prometheus. It focuses on logs instead of metrics, collecting logs via push instead of pull.
- [Grafana Alloy](https://grafana.com/docs/alloy/latest/) is the collection agent. It discovers Docker containers, reads stdout/stderr, processes log lines, and forwards them to the local Loki instance.
- [Grafana](https://grafana.com/docs/) is already in use for cluster metrics. Use Loki as data source for both logs and alerting.

The high-level composition of the services is here sketched:

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
```
The configuration files can also be written at runtime but the volumes link allows a faster startup and configuration reload at runtime.


> ⚠️ 
> The *observability stack* can be ovverided to exclude the deployments of the three services at root/cluster deployment by:
 ```bash
 docker-compose -f docker-compose.yml -f override-no-observe.yml up --build
 ```
> ⚠️
> Alloy positions and Loki data use named volumes. Regular container recreation preserves them; `docker compose down -v` deliberately removes the stored state and log history.

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

The assigned Oakestra database ID is not available when Compose creates the
containers, so `cluster_id` intentionally uses the configured Cluster name.
The compatibility labels `container_name`, `job`, and `logstream` remain
available for the existing dashboards and alert rules.

`loki.source.docker` reads both stdout and stderr through the Docker API and
forwards them to the processing pipeline:

```alloy
loki.source.docker "oakestra" {
  host             = "unix:///var/run/docker.sock"
  targets          = discovery.relabel.oakestra.output
  relabel_rules    = discovery.relabel.docker_stream.rules
  refresh_interval = "5s"
  forward_to       = [loki.process.oakestra.receiver]
}
```

More Docker metadata is available, but labels such as full container IDs,
container IPs, and source-line numbers are deliberately not indexed to avoid
unnecessary label churn and cardinality.

#### Labels granularity
The existing JSON and Oakestra default-format extraction is preserved with
Alloy's `loki.process` stages:

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

If a supported format matches, `level` and `service` are extracted as
labels. Raw lines are still collected when neither parser matches.

The [rules.yml](./alerts/rules.yml) contains LogQL rules based on the labels extracted by Alloy. Both discovery and processing labels can be used for alert conditions.

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
  - Currently only logging features for `root_scheduler`, `system_manager`, `cluster_manager`
  - Test if the defined format is suitable for each relevant information extraction
    - *e.g. Incoming/Outgoing HTTP req, internal cluster updates, deployment descriptor, etc* 
- Where a logging module is not set up, integrate it and review all the service log lines
  - Remove print, redirect stdout/stderr, aggregate/disaggregate information log where needed
- Test OpenTelemetry toolset for format-agnostic observability, relieving Oakestra of the weight of the grafana stack:
  - [Automatic Instrumentation](https://opentelemetry.io/docs/languages/python/automatic/example/), may allow to traces with zero-code 
  - [Collector/Exporter](https://opentelemetry-python.readthedocs.io/en/latest/exporter/otlp/otlp.html): the idea is have OpenTelemetry Collector as log exporter + Alloy for ingestion. Interesting is [automatic instrumentation](https://opentelemetry.io/docs/languages/python/automatic/example/) of Python services.

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