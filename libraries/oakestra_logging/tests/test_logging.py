import json
import logging
from importlib.resources import files

import jsonschema
import pytest
from oakestra_logging import configure_logging, get_logger


def records(capsys):
    return [json.loads(line) for line in capsys.readouterr().out.splitlines() if line]


def log_schema():
    return json.loads(
        files("oakestra_logging").joinpath("schema/log-event-v1.schema.json").read_text()
    )


def test_structured_record_matches_schema(capsys):
    configure_logging("test_service")
    get_logger("test.logger").info(
        "Worker registered",
        event_name="worker.registered",
        worker_id="worker-1",
    )

    [record] = records(capsys)
    jsonschema.Draft202012Validator(
        log_schema(), format_checker=jsonschema.FormatChecker()
    ).validate(record)
    assert record["schema_version"] == 1
    assert record["service"] == "test_service"
    assert record["event"] == "worker.registered"
    assert record["context"] == {"worker_id": "worker-1"}
    assert "source" not in record


def test_warning_contains_source_metadata(capsys):
    configure_logging("test_service")
    get_logger(__name__).warning("Retrying request")

    [record] = records(capsys)
    assert record["source"]["file"] == "test_logging.py"
    assert record["source"]["function"] == "test_warning_contains_source_metadata"
    assert isinstance(record["source"]["line"], int)
    jsonschema.Draft202012Validator(log_schema()).validate(record)


def test_schema_rejects_source_below_warning(capsys):
    configure_logging("test_service")
    get_logger(__name__).info("Normal operation")

    [record] = records(capsys)
    record["source"] = {"file": "service.py", "function": "run", "line": 12}
    with pytest.raises(jsonschema.ValidationError):
        jsonschema.Draft202012Validator(log_schema()).validate(record)


def test_schema_requires_source_at_warning_and_above(capsys):
    configure_logging("test_service")
    get_logger(__name__).error("Operation failed")

    [record] = records(capsys)
    record.pop("source")
    with pytest.raises(jsonschema.ValidationError):
        jsonschema.Draft202012Validator(log_schema()).validate(record)


@pytest.mark.parametrize(
    ("level", "expects_source"),
    [
        ("debug", False),
        ("info", False),
        ("warning", True),
        ("error", True),
        ("critical", True),
    ],
)
def test_every_supported_level_matches_schema(capsys, level, expects_source):
    configure_logging("test_service", "DEBUG")
    getattr(get_logger(__name__), level)("Level check")

    [record] = records(capsys)
    assert record["level"] == level
    assert ("source" in record) is expects_source
    jsonschema.Draft202012Validator(log_schema()).validate(record)


def test_exception_is_one_json_record(capsys):
    configure_logging("test_service")
    try:
        raise ValueError("broken")
    except ValueError:
        get_logger(__name__).exception("Operation failed")

    [record] = records(capsys)
    assert record["exception"]["type"] == "ValueError"
    assert record["exception"]["message"] == "broken"
    assert "Traceback (most recent call last)" in record["exception"]["stacktrace"]


def test_sensitive_context_is_recursively_redacted(capsys):
    configure_logging("test_service")
    get_logger(__name__).info(
        "Authenticating",
        request={
            "username": "ivo",
            "password": "secret",
            "password_hash": "hashed-secret",
            "oauth_token_value": "opaque-token",
            "nested": {"accessToken": "token-value"},
        },
    )

    [record] = records(capsys)
    assert record["context"]["request"] == {
        "username": "ivo",
        "password": "[REDACTED]",
        "password_hash": "[REDACTED]",
        "oauth_token_value": "[REDACTED]",
        "nested": {"accessToken": "[REDACTED]"},
    }


def test_every_sensitive_key_family_is_case_insensitively_redacted(capsys):
    configure_logging("test_service")
    sensitive_context = {
        "PASSWORD_HASH": "password-value",
        "clientSecret": "secret-value",
        "refresh_token_value": "token-value",
        "Authorization_Header": "authorization-value",
        "Set-Cookie": "cookie-value",
        "API_KEY_ID": "api-key-value",
        "private_key_path": "private-key-value",
        "serviceCredentials": "credentials-value",
    }
    get_logger(__name__).info("Sensitive values", details=sensitive_context)

    [record] = records(capsys)
    assert record["context"]["details"] == {
        key: "[REDACTED]" for key in sensitive_context
    }


def test_standard_library_logging_uses_same_contract(capsys):
    configure_logging("test_service")
    logging.getLogger("legacy.library").info(
        "Library message",
        extra={"operation": "lookup", "credentials": {"token": "secret"}},
    )

    [record] = records(capsys)
    assert record["logger"] == "legacy.library"
    assert record["message"] == "Library message"
    assert record["service"] == "test_service"
    assert record["context"]["operation"] == "lookup"
    assert record["context"]["credentials"] == "[REDACTED]"


def test_standard_library_exception_is_one_json_record(capsys):
    configure_logging("test_service")
    try:
        raise RuntimeError("stdlib failure")
    except RuntimeError:
        logging.getLogger("legacy.library").exception("Library operation failed")

    [record] = records(capsys)
    assert record["exception"]["type"] == "RuntimeError"
    assert record["exception"]["message"] == "stdlib failure"
    assert "Traceback (most recent call last)" in record["exception"]["stacktrace"]


def test_reconfiguration_does_not_duplicate_records(capsys):
    configure_logging("test_service")
    logger = get_logger(__name__)
    logger.info("Before reconfiguration")
    assert len(records(capsys)) == 1

    configure_logging("test_service")
    logger.info("After reconfiguration")

    assert len(records(capsys)) == 1


def test_debug_is_filtered_at_default_level(capsys):
    configure_logging("test_service")
    get_logger(__name__).debug("Hidden")

    assert records(capsys) == []


def test_log_level_is_read_from_environment(capsys, monkeypatch):
    monkeypatch.setenv("LOG_LEVEL", "DEBUG")
    configure_logging("test_service")
    get_logger(__name__).debug("Visible diagnostic")

    [record] = records(capsys)
    assert record["level"] == "debug"


def test_invalid_level_emits_warning_and_falls_back(capsys):
    configure_logging("test_service", "verbose")

    [record] = records(capsys)
    assert record["level"] == "warning"
    assert record["event"] == "logging.invalid_level"
    assert record["context"]["requested_level"] == "VERBOSE"


def test_unknown_context_values_are_serialized(capsys):
    class Identifier:
        def __str__(self):
            return "object-id"

    configure_logging("test_service")
    get_logger(__name__).info("Serialized value", identifier=Identifier())

    [record] = records(capsys)
    assert record["context"]["identifier"] == "object-id"


def test_failed_string_conversion_does_not_break_logging(capsys):
    class BrokenValue:
        def __str__(self):
            raise RuntimeError("cannot stringify")

    configure_logging("test_service")
    get_logger(__name__).info("Serialized fallback", value=BrokenValue())

    [record] = records(capsys)
    assert record["context"]["value"] == "<unserializable BrokenValue>"


def test_configuration_writes_no_log_files(capsys, tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    configure_logging("test_service")
    get_logger(__name__).info("Stdout only")

    captured = capsys.readouterr()
    assert len([line for line in captured.out.splitlines() if line]) == 1
    assert captured.err == ""
    assert list(tmp_path.iterdir()) == []


def test_service_name_is_required():
    with pytest.raises(ValueError):
        configure_logging("")
