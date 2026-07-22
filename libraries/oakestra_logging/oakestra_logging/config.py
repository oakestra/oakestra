"""Configure the common Oakestra JSON logging contract."""

from __future__ import annotations

import json
import logging
import os
import re
import sys
import traceback
from collections.abc import Mapping
from typing import Any

import structlog
from structlog.typing import EventDict, Processor, WrappedLogger

SCHEMA_VERSION = 1
DEFAULT_LOG_LEVEL = "INFO"
REDACTED = "[REDACTED]"

_service_name = "unknown"
_configured_level = logging.INFO

_SENSITIVE_KEYS = {
    "accesstoken",
    "apikey",
    "authorization",
    "cookie",
    "credentials",
    "password",
    "passwd",
    "privatekey",
    "refreshtoken",
    "secret",
    "setcookie",
    "token",
}

_LEVEL_NUMBERS = {
    "debug": logging.DEBUG,
    "info": logging.INFO,
    "warning": logging.WARNING,
    "error": logging.ERROR,
    "critical": logging.CRITICAL,
}

_CONFIG_LEVELS = {
    "DEBUG": logging.DEBUG,
    "INFO": logging.INFO,
    "WARN": logging.WARNING,
    "WARNING": logging.WARNING,
    "ERROR": logging.ERROR,
    "FATAL": logging.CRITICAL,
    "CRITICAL": logging.CRITICAL,
}


def _safe_string(value: object) -> str:
    try:
        return str(value)
    except Exception:
        return f"<unserializable {type(value).__name__}>"


def _normalise_key(key: object) -> str:
    return re.sub(r"[^a-z0-9]", "", _safe_string(key).lower())


def _is_sensitive_key(key: object) -> bool:
    normalised = _normalise_key(key)
    return any(sensitive in normalised for sensitive in _SENSITIVE_KEYS)


def _redact(value: Any, *, depth: int = 0) -> Any:
    """Recursively redact sensitive mapping values while preserving JSON-friendly structure."""
    if depth >= 20:
        return "[MAX_DEPTH]"
    if isinstance(value, Mapping):
        return {
            _safe_string(key): (
                REDACTED if _is_sensitive_key(key) else _redact(item, depth=depth + 1)
            )
            for key, item in value.items()
        }
    if isinstance(value, (list, tuple, set, frozenset)):
        return [_redact(item, depth=depth + 1) for item in value]
    return value


def _redact_processor(
    _logger: WrappedLogger, _method_name: str, event_dict: EventDict
) -> EventDict:
    return _redact(event_dict)


def _add_contract_fields(
    _logger: WrappedLogger, _method_name: str, event_dict: EventDict
) -> EventDict:
    event_dict["schema_version"] = SCHEMA_VERSION
    event_dict["service"] = _service_name
    return event_dict


def _normalise_exception(
    _logger: WrappedLogger, _method_name: str, event_dict: EventDict
) -> EventDict:
    exc_info = event_dict.pop("exc_info", None)
    if not exc_info:
        return event_dict

    if exc_info is True:
        exc_info = sys.exc_info()
    elif isinstance(exc_info, BaseException):
        exc_info = (type(exc_info), exc_info, exc_info.__traceback__)

    if not isinstance(exc_info, tuple) or len(exc_info) != 3 or exc_info[1] is None:
        event_dict["exception"] = {
            "type": "UnknownException",
            "message": _safe_string(exc_info),
            "stacktrace": "",
        }
        return event_dict

    exception_type, exception, exception_traceback = exc_info
    event_dict["exception"] = {
        "type": getattr(exception_type, "__name__", _safe_string(exception_type)),
        "message": _safe_string(exception),
        "stacktrace": "".join(
            traceback.format_exception(exception_type, exception, exception_traceback)
        ).rstrip(),
    }
    return event_dict


def _nest_source(_logger: WrappedLogger, _method_name: str, event_dict: EventDict) -> EventDict:
    source = {
        "file": event_dict.pop("filename", None),
        "function": event_dict.pop("func_name", None),
        "line": event_dict.pop("lineno", None),
    }
    if _LEVEL_NUMBERS.get(str(event_dict.get("level", "")).lower(), 0) >= logging.WARNING:
        if all(value is not None for value in source.values()):
            event_dict["source"] = source
    return event_dict


def _shape_event(_logger: WrappedLogger, _method_name: str, event_dict: EventDict) -> EventDict:
    message = event_dict.pop("event", "")
    stable_event = event_dict.pop("event_name", None)

    event_dict["message"] = message if isinstance(message, str) else _safe_string(message)
    if stable_event:
        event_dict["event"] = _safe_string(stable_event)

    reserved = {
        "_from_structlog",
        "_record",
        "event",
        "exception",
        "level",
        "logger",
        "message",
        "schema_version",
        "service",
        "source",
        "timestamp",
    }
    supplied_context = event_dict.pop("context", {})
    context = (
        dict(supplied_context)
        if isinstance(supplied_context, Mapping)
        else {"value": supplied_context}
    )
    for key in list(event_dict):
        if key not in reserved and not key.startswith("_"):
            context[key] = event_dict.pop(key)
    if context:
        event_dict["context"] = context
    return event_dict


def _json_default(value: object) -> str:
    return _safe_string(value)


def _render_json(_logger: WrappedLogger, _method_name: str, event_dict: EventDict) -> str:
    event_dict.pop("_record", None)
    event_dict.pop("_from_structlog", None)
    return json.dumps(
        event_dict,
        default=_json_default,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )


def _resolve_level(level: str | None) -> tuple[int, str | None]:
    requested = level if level is not None else os.getenv("LOG_LEVEL", DEFAULT_LOG_LEVEL)
    level_name = str(requested).upper()
    resolved = _CONFIG_LEVELS.get(level_name)
    if resolved is not None:
        return resolved, None
    return logging.INFO, level_name


def _shared_processors() -> list[Processor]:
    return [
        structlog.contextvars.merge_contextvars,
        structlog.stdlib.add_log_level,
        structlog.stdlib.add_logger_name,
        structlog.stdlib.ExtraAdder(),
        structlog.processors.TimeStamper(fmt="iso", utc=True, key="timestamp"),
        _add_contract_fields,
        _normalise_exception,
        structlog.processors.CallsiteParameterAdder(
            {
                structlog.processors.CallsiteParameter.FILENAME,
                structlog.processors.CallsiteParameter.FUNC_NAME,
                structlog.processors.CallsiteParameter.LINENO,
            },
            additional_ignores=["oakestra_logging"],
        ),
        _nest_source,
        _shape_event,
        _redact_processor,
    ]


def configure_logging(service: str, level: str | None = None) -> None:
    """Configure one stdout JSON handler for a Python service.

    Calling this function repeatedly replaces the root handler rather than duplicating output.
    """
    global _configured_level, _service_name

    if not service or not service.strip():
        raise ValueError("service must be a non-empty string")

    _service_name = service.strip()
    _configured_level, invalid_level = _resolve_level(level)
    processors = _shared_processors()

    formatter = structlog.stdlib.ProcessorFormatter(
        processor=_render_json,
        foreign_pre_chain=processors,
    )
    handler = logging.StreamHandler(sys.stdout)
    handler.setLevel(_configured_level)
    handler.setFormatter(formatter)

    root_logger = logging.getLogger()
    root_logger.handlers.clear()
    root_logger.addHandler(handler)
    root_logger.setLevel(_configured_level)

    structlog.configure(
        processors=[
            structlog.stdlib.filter_by_level,
            *processors,
            structlog.stdlib.ProcessorFormatter.wrap_for_formatter,
        ],
        wrapper_class=structlog.stdlib.BoundLogger,
        context_class=dict,
        logger_factory=structlog.stdlib.LoggerFactory(),
        cache_logger_on_first_use=True,
    )

    if invalid_level:
        get_logger(__name__).warning(
            "Invalid log level; using INFO",
            event_name="logging.invalid_level",
            requested_level=invalid_level,
            fallback_level=DEFAULT_LOG_LEVEL,
        )


def configure_gunicorn_loggers() -> None:
    """Route Gunicorn's standard-library loggers through the root JSON handler.

    Gunicorn owns its startup messages and normally installs text formatters on
    ``gunicorn.error`` and ``gunicorn.access``.  The custom Gunicorn logger
    calls this after its normal setup so those records propagate to the same
    handler configured by :func:`configure_logging`.
    """
    root_level = logging.getLogger().level
    for logger_name in ("gunicorn.error", "gunicorn.access"):
        gunicorn_logger = logging.getLogger(logger_name)
        gunicorn_logger.handlers.clear()
        gunicorn_logger.setLevel(root_level)
        gunicorn_logger.propagate = True


def get_logger(name: str | None = None) -> structlog.stdlib.BoundLogger:
    """Return a structured logger using the shared configuration."""
    return structlog.get_logger(name)
