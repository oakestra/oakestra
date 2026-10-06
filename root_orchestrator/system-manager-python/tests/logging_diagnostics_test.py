import json
import logging
from pathlib import Path

import pytest
import requests
import structlog
from ext_requests import cluster_requests
from oakestra_logging import configure_logging
from services.service_management import create_services_of_app
from sla.versioned_sla_parser import SLAFormatError, parse_sla_json


@pytest.fixture(autouse=True)
def restore_logging_configuration():
    root = logging.getLogger()
    handlers = root.handlers[:]
    level = root.level
    config = structlog.get_config()
    configured = structlog.is_configured()
    yield
    root.handlers[:] = handlers
    root.setLevel(level)
    if configured:
        structlog.configure(**config)
    else:
        structlog.reset_defaults()


@pytest.mark.parametrize(
    ("function_name", "arguments", "endpoint", "job_id"),
    [
        ("cluster_request_status", ("cluster-1",), "/status", None),
        ("cluster_request_to_deploy", ("cluster-1", "job-1", 2), "/api/service/job-1/2", "job-1"),
        ("cluster_request_to_delete_job", ("job-1", 2), "/api/service/job-1/2", "job-1"),
        (
            "cluster_request_to_delete_job_by_ip",
            ("job-1", 2, "cluster-1"),
            "/api/service/job-1/2",
            "job-1",
        ),
        (
            "cluster_request_to_replicate_up",
            ({"_id": "cluster-1", "ip": "127.0.0.1", "port": 10100}, {"_id": "job-1"}, 2),
            "/api/replicate/",
            "job-1",
        ),
        (
            "cluster_request_to_replicate_down",
            ({"_id": "cluster-1", "ip": "127.0.0.1", "port": 10100}, {"_id": "job-1"}, 2),
            "/api/replicate/",
            "job-1",
        ),
        (
            "cluster_request_to_move_within_cluster",
            ({"_id": "cluster-1", "ip": "127.0.0.1", "port": 10100}, "job-1", "node-a", "node-b"),
            "/api/move/",
            "job-1",
        ),
    ],
)
def test_cluster_failure_records_identify_target(
    monkeypatch, capsys, function_name, arguments, endpoint, job_id
):
    configure_logging("system_manager")
    candidate = {"_id": "cluster-1", "ip": "127.0.0.1", "port": 10100}
    monkeypatch.setattr(
        cluster_requests.candidate_operations, "get_candidate_by_id", lambda _id: candidate
    )
    monkeypatch.setattr(
        cluster_requests.job_operations,
        "get_job_instance",
        lambda *_args: {"_id": "job-1", "payload": "private-payload"},
    )
    monkeypatch.setattr(cluster_requests, "find_cluster_of_job", lambda *_args: candidate)

    def fail_request(url, **kwargs):
        assert url == "http://127.0.0.1:10100" + endpoint
        raise requests.exceptions.ConnectionError("Connection refused")

    for method in ("get", "post", "delete"):
        monkeypatch.setattr(cluster_requests.requests, method, fail_request)

    getattr(cluster_requests, function_name)(*arguments)

    output = capsys.readouterr().out
    record = json.loads(output.splitlines()[-1])
    assert record["level"] == "error"
    assert record["context"]["cluster_id"] == "cluster-1"
    assert record["context"]["cluster_addr"] == "http://127.0.0.1:10100" + endpoint
    assert record["exception"]["message"] == "Connection refused"
    if job_id:
        assert record["context"]["job_id"] == job_id
    assert "private-payload" not in output


@pytest.mark.parametrize("missing", ["cluster", "job"])
def test_missing_deployment_records_keep_bound_identifiers(monkeypatch, capsys, missing):
    configure_logging("system_manager")
    monkeypatch.setattr(
        cluster_requests.candidate_operations,
        "get_candidate_by_id",
        lambda _id: None if missing == "cluster" else {"_id": "cluster-1"},
    )
    monkeypatch.setattr(cluster_requests.job_operations, "get_job_instance", lambda *_args: None)

    cluster_requests.cluster_request_to_deploy("cluster-1", "job-1", 2)

    record = json.loads(capsys.readouterr().out)
    assert record["event"] == missing + ".not_found"
    assert record["context"]["cluster_id"] == "cluster-1"
    assert record["context"]["job_id"] == "job-1"
    assert record["context"]["instance_number"] == 2


def test_sla_failure_logs_schema_reason_without_submitted_values(capsys):
    configure_logging("system_manager")
    sample = Path(__file__).parent / "service_level_agreements" / "sla_correct_1.json"
    sla = json.loads(sample.read_text())
    sla["applications"][0]["application_name"] = "private-payload"

    with pytest.raises(SLAFormatError) as raised:
        parse_sla_json(sla)

    output = capsys.readouterr().out
    record = json.loads(output)
    result = record["context"]["validation_result"]
    assert record["event"] == "sla.validation.failed"
    assert result["constraint"] == "pattern"
    assert "application_name" in result["schema_path"]
    assert result["expected"] == "^[a-zA-Z0-9]{1,32}$"
    assert "private-payload" not in output
    assert "private-payload" in str(raised.value.args[0])


def test_service_creation_logs_validation_failure_once(capsys):
    configure_logging("system_manager")
    sample = Path(__file__).parent / "service_level_agreements" / "sla_correct_1.json"
    sla = json.loads(sample.read_text())
    sla["applications"][0]["application_name"] = "private-payload"

    response, status = create_services_of_app("test-user", sla)

    output = capsys.readouterr().out
    assert status == 422
    assert isinstance(response["message"], SLAFormatError)
    assert len(output.splitlines()) == 1
    assert json.loads(output)["event"] == "sla.validation.failed"
    assert "private-payload" not in output


@pytest.mark.parametrize("applications", [None, 42, "invalid", {}])
def test_invalid_applications_reaches_sla_validation(capsys, applications):
    configure_logging("system_manager", level="DEBUG")
    sample = Path(__file__).parent / "service_level_agreements" / "sla_correct_1.json"
    sla = json.loads(sample.read_text())
    sla["applications"] = applications

    response, status = create_services_of_app("test-user", sla)

    assert status == 422
    assert isinstance(response["message"], SLAFormatError)
    records = [json.loads(line) for line in capsys.readouterr().out.splitlines()]
    started = next(record for record in records if record.get("event") == "services.create.started")
    assert started["context"]["application_count"] == 0
    assert sum(record.get("event") == "sla.validation.failed" for record in records) == 1
