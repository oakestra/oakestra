# Oakestra logging

`oakestra_logging` provides the shared, build-time logging configuration for Oakestra's
Python services. It emits one JSON object per line to stdout, allowing Docker, Grafana Alloy,
and Loki to transport logs without parsing service-specific text formats.

```python
from oakestra_logging import configure_logging, get_logger

configure_logging("system_manager")
logger = get_logger(__name__)

logger.info(
    "Cluster registered",
    event_name="cluster.registered",
    cluster_id=cluster_id,
)
```

The output level is controlled with `LOG_LEVEL` and defaults to `INFO`. The JSON shape is
defined by `oakestra_logging/schema/log-event-v1.schema.json`. Application-specific values are
placed under `context`; `event_name` becomes the optional stable `event` field. Docker metadata
such as `cluster_id`, `compose_service`, and `container` is attached by Alloy and is intentionally
not part of this application schema.

Sensitive mapping keys are recursively redacted. Call sites must still avoid embedding secrets,
personal data, or complete large payloads directly in the human-readable message.

## Version 1 contract

| Field | Required | Meaning |
|---|---:|---|
| `schema_version` | yes | Contract version, currently `1` |
| `timestamp` | yes | UTC RFC 3339 timestamp |
| `level` | yes | `debug`, `info`, `warning`, `error`, or `critical` |
| `service` | yes | Runtime role from `OAKESTRA_SERVICE_NAME` |
| `logger` | yes | Python module or standard-library logger name |
| `message` | yes | Human-readable summary without embedded payloads or secrets |
| `event` | no | Stable dotted identifier supplied as `event_name` |
| `context` | no | Structured application values supplied as keyword arguments |
| `source` | no | File, function, and line; emitted only at `warning` and above |
| `exception` | no | Exception type, message, and escaped stack trace in the same JSON record |

Use `debug` for verbose diagnostics, `info` for lifecycle and normal state changes, `warning` for
recoverable failures, `error` for failed operations, and `critical` when the service cannot
continue. Use `logger.exception()` inside an exception handler instead of emitting the exception
and traceback as separate records.

The configuration also routes standard-library loggers through the same formatter. This keeps
Flask and reusable internal libraries compatible while service-owned code uses `get_logger()`.
Calling `configure_logging()` repeatedly replaces the root handler, preventing duplicate records.
An invalid `LOG_LEVEL` produces a structured warning and falls back to `INFO`.

## Installation in service images

Oakestra's Python service images use separate Docker build contexts, so the repository-level
library directory is not copied into those images automatically. Compose and image workflows
provide `libraries/` as a BuildKit named context, and each consuming Dockerfile installs the
package from that local context:

```text
COPY --from=libraries . /libraries
RUN pip install --no-cache-dir /libraries/oakestra_logging
```

This guarantees that a source build uses the logging package from the same checkout as the service
without requiring a pushed branch or fetching application dependencies from GitHub.

The recursive redaction processor replaces values whose keys represent passwords, secrets,
tokens, authorization headers, cookies, API keys, private keys, or credentials. Redaction cannot
identify a secret interpolated into `message`, an exception message, or an opaque string, so call
sites must log identifiers, counts, field names, and payload sizes rather than complete request,
SLA, MQTT, database, user, or resource objects.

## Local verification

```bash
python -m venv .venv
. .venv/bin/activate
pip install -e 'libraries/oakestra_logging[test]'
pytest libraries/oakestra_logging/tests
```
