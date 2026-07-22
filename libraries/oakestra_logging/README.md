# Oakestra logging

`oakestra_logging` is the shared runtime logging configuration for Oakestra's Python services. It emits one JSON object per line to stdout so Docker can capture the record and Promtail or Grafana Alloy can forward it to Loki without parsing a service-specific text format. The package owns the configuration and schema; application modules only choose a level, a short message, a stable event name, and useful structured context.

The design follows Structlog's official [getting-started guide](https://www.structlog.org/en/stable/getting-started.html): log entries are event dictionaries, keyword arguments add structured context, processors enrich and transform each entry, and the final renderer produces JSON. Oakestra also follows Structlog's [logging best practices](https://www.structlog.org/en/stable/logging-best-practices.html) by writing structured records to stdout and favoring a small number of meaningful records over noisy step-by-step output. The implementation goes beyond the basic tutorial because Oakestra needs a fixed cross-service contract and compatibility with existing libraries. It therefore uses Structlog's documented [standard-library integration](https://www.structlog.org/en/stable/standard-library.html), including `ProcessorFormatter`, so records from Flask and other `logging.getLogger()` users pass through the same formatter.

Do not call `structlog.configure()` directly in an Oakestra service. Use this package so every Python component keeps the same output, redaction, level filtering, and exception behavior.

## Configure a service

Call `configure_logging()` once in the service entry point, before the application starts handling work. The deployment supplies the runtime role through `OAKESTRA_SERVICE_NAME`; the code fallback keeps direct local execution usable.

```python
import os

from oakestra_logging import configure_logging, get_logger

configure_logging(os.getenv("OAKESTRA_SERVICE_NAME", "system_manager"))
logger = get_logger(__name__)
```

Other modules must only obtain a module-named logger:

```python
from oakestra_logging import get_logger

logger = get_logger(__name__)
```

The System Manager, Cluster Manager, and JWT Generator also select the package's Gunicorn logger class in their image command. This routes Gunicorn startup and optional access records through the same JSON handler. Flask development-server banners, subprocess output, and dependencies that write directly to stdout or stderr cannot be converted safely by the logging package and are deliberately left as raw lines.

The output threshold is controlled by `LOG_LEVEL` and defaults to `INFO`. Supported configuration values are `DEBUG`, `INFO`, `WARN`, `WARNING`, `ERROR`, `FATAL`, and `CRITICAL`; output always uses the normalized lowercase levels `debug`, `info`, `warning`, `error`, and `critical`. An invalid value produces a structured warning and falls back to `INFO`. Calling `configure_logging()` repeatedly replaces the root handler rather than adding another one, but services should still configure it only in their entry point.

## Version 1 log structure

The authoritative contract is `oakestra_logging/schema/log-event-v1.schema.json`. Every standardized record is a single JSON object followed by a newline.

| Field | Required | Meaning |
|---|---:|---|
| `schema_version` | yes | Numeric contract version, currently `1` |
| `timestamp` | yes | UTC RFC 3339 timestamp added by the formatter |
| `level` | yes | `debug`, `info`, `warning`, `error`, or `critical` |
| `service` | yes | Runtime role supplied to `configure_logging()`, normally through `OAKESTRA_SERVICE_NAME` |
| `logger` | yes | Python module or standard-library logger name |
| `message` | yes | Short human-readable description of what happened |
| `event` | no | Stable dotted identifier supplied as `event_name` |
| `context` | no | Structured application values supplied as keyword arguments or bound context |
| `source` | conditional | File, function, and line; included only at `warning`, `error`, and `critical` |
| `exception` | no | Exception type, message, and escaped stack trace in the same JSON record |

A normal service-owned record looks like this:

```json
{
  "context": {
    "cluster_id": "abc123",
    "job_count": 2
  },
  "event": "cluster.registration.completed",
  "level": "info",
  "logger": "services.cluster_management",
  "message": "Cluster registration completed",
  "schema_version": 1,
  "service": "system_manager",
  "timestamp": "2026-07-22T10:00:00Z"
}
```

The processor chain merges bound context, adds the normalized level and logger name, copies safe standard-library `extra` values, adds the timestamp, schema version, and service role, normalizes exception information, adds source location for warning and higher levels, maps `event_name` to `event`, moves application values under `context`, recursively redacts sensitive mapping keys, and serializes compact JSON to stdout.

## How to write a good log

### Describe one meaningful event

Use a short static message that a person can understand, and put changing values in keyword arguments. This keeps messages groupable, makes fields queryable, and reduces the chance of leaking data through string interpolation.

```python
logger.info(
    "Cluster registration completed",
    event_name="cluster.registration.completed",
    cluster_id=cluster_id,
    cluster_name=cluster_name,
)
```

Do not construct the whole record as text:

```python
# Bad: changing data is hidden in the message and may expose the complete object.
logger.info(f"Cluster {cluster_id} registered with payload {cluster_payload}")
```

Prefer one record for a meaningful outcome. A start record is useful when an operation may be slow, asynchronous, or fail later; otherwise, a single completion or failure record is usually enough. High-frequency internal steps belong at `debug`, not `info`.

### Choose a stable event name

Use `event_name` when the event is operationally important or likely to be queried, counted, linked to a dashboard, or used by an alert. Event names are lowercase dotted identifiers and must describe the event rather than contain dynamic data. Follow the existing `domain.object.outcome` style, for example `cluster.registration.completed`, `scheduler.deploy.request_failed`, `addon.container.retry_scheduled`, or `mail.send_failed`.

Keep an event name stable when wording changes. Do not put IDs, cluster names, exception text, or status values inside it.

```python
# Good
logger.warning(
    "Cluster endpoint is temporarily unreachable",
    event_name="cluster.registration.endpoint_unreachable",
    cluster_address=cluster_address,
    retry_in_seconds=retry_delay,
)

# Bad
logger.warning(
    "Cluster endpoint is temporarily unreachable",
    event_name=f"cluster.{cluster_id}.unreachable",
)
```

Generic dependency records do not need an artificial event name. The standard-library bridge can still give them the required `level`, `service`, `logger`, `message`, `schema_version`, and `timestamp` fields.

### Add useful, bounded context

Use lowercase `snake_case` keys. Prefer identifiers, counts, enum-like states, durations, field names, and target components that help answer what failed, where, and during which operation. Examples include `cluster_id`, `worker_id`, `service_id`, `job_id`, `operation`, `status`, `retry_count`, `duration_ms`, `field_count`, and `error_type`.

Do not attach complete request bodies, SLAs, MQTT payloads, database documents, user objects, resource snapshots, HTTP responses, environment dictionaries, or arbitrary objects. Record the minimum diagnostic summary instead: an identifier, payload size, selected field names, item count, or exception type. Even though application context is parsed at query time rather than promoted to permanent Loki labels, large and unbounded values still increase storage, cost, and exposure.

When several records in one local operation need the same values, bind them once to a local logger. Bound loggers are immutable, so keep the returned logger in the local scope.

```python
operation_logger = logger.bind(
    cluster_id=cluster_id,
    operation="cluster_registration",
)
operation_logger.debug(
    "Validating cluster registration",
    event_name="cluster.registration.validating",
)
operation_logger.info(
    "Cluster registration completed",
    event_name="cluster.registration.completed",
)
```

Do not bind request-specific values to a global module logger. Oakestra's processor chain supports context variables, but request middleware must clear and bind them correctly before they are adopted broadly, especially because the services use Eventlet and mixed execution contexts.

### Use levels consistently

| Level | Use it for | Examples |
|---|---|---|
| `debug` | High-frequency or detailed diagnostics normally hidden in production | Request preparation, candidate counts, retry internals |
| `info` | Normal lifecycle events and successful state changes | Service started, cluster registered, job inserted |
| `warning` | Unexpected but recoverable conditions requiring attention | Invalid input, retry scheduled, optional dependency unavailable |
| `error` | An operation failed, but the process can continue serving other work | Deployment rejected by a dependency, database update failed |
| `critical` | The service cannot initialize or safely continue | Required configuration missing, unrecoverable registration failure |

Do not use `error` merely because an HTTP client returned a normal user-facing 4xx result. Do not use `info` for tight polling loops or full payloads. `warning`, `error`, and `critical` automatically include source file, function, and line metadata; lower levels intentionally omit it to reduce record size.

### Log exceptions once

Inside an exception handler, use `logger.exception()` when the traceback is useful. The package stores the exception type, message, and stack trace in one `exception` object and emits one JSON line.

```python
try:
    deploy_service(service_id)
except DeploymentError:
    logger.exception(
        "Service deployment failed",
        event_name="service.deploy.failed",
        service_id=service_id,
        operation="deploy_service",
    )
    raise
```

Do not call `traceback.print_exc()`, format a traceback manually, or log the same exception again at every layer. Log it once at the boundary that handles, translates, retries, or terminates because of it. For an expected recoverable failure where a traceback adds no value, use `warning` or `error` with a safe summary such as `error_type=type(error).__name__`.

Exception messages are supplied by the thrown object and cannot be reliably redacted. Never construct exceptions containing credentials or complete sensitive payloads.

### Protect secrets and personal data

The redaction processor recursively replaces values whose mapping keys represent passwords, `passwd`, secrets, tokens, authorization headers, cookies, API keys, private keys, or credentials. Matching is case-insensitive and covers variants such as `password_hash`, `oauth_token_value`, and `Authorization_Header`.

```python
logger.info(
    "Authentication attempted",
    event_name="authentication.request.received",
    request={"username": username, "password": supplied_password},
)
```

The structured `password` value above becomes `[REDACTED]`, but redaction is a final safety net, not permission to log sensitive data. It cannot discover secrets embedded in `message`, an exception message, an opaque string, or a value stored under an innocent-looking key. Prefer not to include the sensitive field at all. Avoid personal data unless it is necessary, permitted, and minimized.

### Keep application fields separate from deployment labels

The application record describes what the Python service did. The collector describes where the record came from. Promtail or Alloy attaches deployment metadata such as `cluster_id`, `compose_service`, `container`, and `logstream`; these are intentionally not part of the application JSON schema. A business object may still appear under `context.cluster_id`, but that means the cluster referenced by the operation, while the collector's `cluster_id` label identifies the orchestrator that emitted the record.

Do not manually copy Docker labels into every log call. Do not turn request IDs, worker IDs, service IDs, user IDs, or other high-cardinality application values into Loki labels; retain them as JSON context and parse them at query time.

### Use the standard-library bridge only when needed

New Oakestra service code should use `get_logger(__name__)`. Existing internal modules and third-party libraries may continue using standard Python logging; `ProcessorFormatter` routes those records through the same contract.

```python
import logging

legacy_logger = logging.getLogger(__name__)
legacy_logger.info(
    "Lookup completed",
    extra={"operation": "lookup", "result_count": result_count},
)
```

Values supplied through standard-library `extra` are moved under `context`. Do not install additional handlers, `basicConfig()`, rotating files, or custom JSON formatters inside a service because they can bypass redaction or duplicate records.

## Stdout and retention

The package does not use Python's `RotatingFileHandler`. Older System Manager and Cluster Manager code wrote a small `sm.log` file and rotated it inside the container; that duplicated the stdout record and was lost with the container filesystem. A containerized service should write one record to stdout and let the platform own transport and storage.

Docker Compose still limits the host's `json-file` logs with `max-size: 1m` and `max-file: 1`, which protects the host disk but retains no previous Docker log generation. This is separate from Loki retention: Docker's local log history can roll over while the collector has already forwarded the records to Loki. Configure Loki retention deliberately in the observability deployment when long-term history is needed.

## How Grafana should identify errors

Grafana alert rules should treat the JSON `level` field as the authoritative severity for Oakestra-owned Python records. `stderr` only identifies a Docker stream; it is not an error level, because tools such as Gunicorn can write normal informational messages there.

For a deployment using the current Promtail labels, an initial structured-error expression is:

```logql
sum by (container_name, service) (
  count_over_time({job="containerlogs"} | json | schema_version=1 | level=~"error|critical" [5m])
)
```

The exact stream selector changes with the deployment and collector. After the Alloy logging work, the alert can group by `cluster_id` and `compose_service` instead. A temporary fallback for lines outside the schema should explicitly exclude structured records and match only well-known failure markers; it must not double-count JSON errors:

```logql
count_over_time(
  {job="containerlogs"}
  |! "\"schema_version\":1"
  |~ "(?i)(\\berror\\b|traceback|panic:|fatal|stack.?trace)" [5m]
)
```

The fallback is a compatibility measure for process-owned or non-Python output, not a substitute for a structured contract. It should become smaller as Go and other components gain their own standard format.

## Review checklist

Before submitting a new or changed log call, confirm that:

- The message is short, static, and understandable without containing the entire payload.
- The level matches the operational severity rather than the developer's frustration.
- An important event has a stable lowercase dotted `event_name`.
- Dynamic values use meaningful `snake_case` context keys.
- Context contains only the minimum identifiers, counts, states, and timings needed to diagnose the event.
- No credential, token, authorization value, cookie, private key, personal data, or full request/database/MQTT object is logged.
- An exception is emitted once with `logger.exception()` only when its traceback is useful.
- A loop, polling path, or recurring health check does not create excessive `info` traffic.
- The code uses `get_logger()` and does not configure another handler or renderer.

## Installation in service images

Oakestra's Python service images use separate Docker build contexts, so the repository-level library directory is not copied into those images automatically. Each consuming service declares the package using pip's named VCS direct-reference syntax:

```text
oakestra-logging @ git+https://github.com/oakestra/oakestra.git@${LIB_BRANCH}#subdirectory=libraries/oakestra_logging
```

`LIB_BRANCH` selects the same Oakestra branch or tag as the service build. This follows the existing mechanism used by Oakestra's other internal Python libraries without introducing a new package registry, environment manager, or Docker/Compose build pattern as part of this logging change. The selected branch or tag must exist in the Oakestra repository before a normal image build can install it.

## Local verification

```bash
python -m venv .venv
. .venv/bin/activate
pip install -e 'libraries/oakestra_logging[test]'
pytest libraries/oakestra_logging/tests
```

When reviewing a live service, filter Docker output to schema-v1 records:

```bash
docker logs --tail 200 system_manager 2>&1 |
  jq -R 'fromjson? | select(.schema_version == 1)'
```

In Loki, parse application fields at query time. The stream selector depends on the collector labels in the deployment; for example:

```logql
{container_name="system_manager"} | json | schema_version=1 | level="error"
```
