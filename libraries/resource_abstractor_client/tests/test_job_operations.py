import json

import httpx
from oakestra_utils.types.statuses import DeploymentStatus
from resource_abstractor_client import job_operations


def test_get_jobs_no_filter(recorder):
    recorder.queue(httpx.Response(200, json=[]))
    job_operations.get_jobs()
    assert recorder.last.url.path == "/api/v1/jobs"
    assert recorder.last.url.query == b""


def test_get_jobs_of_application_sends_applicationid(recorder):
    recorder.queue(httpx.Response(200, json=[]))
    job_operations.get_jobs_of_application("app1")
    assert recorder.last.url.params["applicationID"] == "app1"


def test_get_jobs_drops_mongo_query_dicts(recorder, caplog):
    """job_management.get_stale_jobs and get_jobs_with_failed_instances pass Mongo
    filter documents that the resource abstractor has never had a query parameter
    for - they urlencoded to junk before and were silently ignored server-side.
    Behaviour is identical (every job still comes back); the only change is a
    warning instead of silence."""
    recorder.queue(httpx.Response(200, json=[{"_id": "j1"}]))
    with caplog.at_level("WARNING"):
        result = job_operations.get_jobs(
            **{"instance_list": {"$elemMatch": {"last_modified_timestamp": {"$lt": 1}}}}
        )
    assert recorder.last.url.query == b""
    assert result == [{"_id": "j1"}]
    assert "instance_list" in caplog.text


def test_get_jobs_with_or_query_dict_dropped(recorder, caplog):
    recorder.queue(httpx.Response(200, json=[{"_id": "j1"}, {"_id": "j2"}]))
    with caplog.at_level("WARNING"):
        result = job_operations.get_jobs(**{"$or": [{"instance_list.status": "FAILED"}]})
    assert recorder.last.url.query == b""
    assert result == [{"_id": "j1"}, {"_id": "j2"}]


def test_get_job_by_id(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "j1"}))
    job_operations.get_job_by_id("j1")
    assert recorder.last.url.path == "/api/v1/jobs/j1"
    assert recorder.last.url.query == b""


def test_get_job_by_id_with_instance_number_filter(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "j1"}))
    job_operations.get_job_by_id("j1", filter={"instance_number": "2"})
    assert recorder.last.url.params["instance_number"] == "2"


def test_get_job_by_id_404_is_none(recorder):
    recorder.queue(httpx.Response(404, json={"message": "Not Found"}))
    assert job_operations.get_job_by_id("missing") is None


def test_get_job_instance(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "j1", "instance_list": []}))
    job_operations.get_job_instance("j1", 3)
    assert recorder.last.url.path == "/api/v1/jobs/j1/3"


def test_get_job_instance_ignores_filter_argument(recorder):
    """getJobInstance has no query parameters in the spec; filter is accepted only to
    keep the function's signature unchanged for existing callers."""
    recorder.queue(httpx.Response(200, json={"_id": "j1"}))
    job_operations.get_job_instance("j1", 3, filter={"unused": "value"})
    assert recorder.last.url.query == b""


def test_append_job_instance(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "j1"}))
    instance_data = {"instance_list": [{"instance_number": 3, "status": "NODE_SCHEDULED"}]}
    job_operations.append_job_instance("j1", 3, instance_data)
    assert recorder.last.method == "PUT"
    assert recorder.last.url.path == "/api/v1/jobs/j1/3"
    assert json.loads(recorder.last.content) == instance_data


def test_create_job_is_a_put_upsert(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "j1"}))
    job_operations.create_job({"job_name": "svc1"})
    assert recorder.last.method == "PUT"
    assert recorder.last.url.path == "/api/v1/jobs"


def test_update_job_patches_whole_document(recorder):
    """cluster-manager's job_management.py PATCHes the entire job document back,
    including fields the spec doesn't declare - must round-trip untouched."""
    recorder.queue(httpx.Response(200, json={"_id": "j1"}))
    data = {
        "status": "RUNNING",
        "next_instance_progressive_number": 4,
        "instance_list": [{"instance_number": 1}],
    }
    job_operations.update_job("j1", data)
    assert recorder.last.method == "PATCH"
    assert json.loads(recorder.last.content) == data


def test_update_job_status_without_detail(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "j1"}))
    job_operations.update_job_status("j1", DeploymentStatus.RUNNING)
    assert json.loads(recorder.last.content) == {"status": DeploymentStatus.RUNNING.value}


def test_update_job_status_with_detail(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "j1"}))
    job_operations.update_job_status("j1", DeploymentStatus.FAILED, "OOM killed")
    assert json.loads(recorder.last.content) == {
        "status": DeploymentStatus.FAILED.value,
        "status_detail": "OOM killed",
    }


def test_update_job_instance(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "j1"}))
    job_operations.update_job_instance("j1", 2, {"status": "RUNNING", "cpu_percent": 12.5})
    assert recorder.last.method == "PATCH"
    assert recorder.last.url.path == "/api/v1/jobs/j1/2"


def test_delete_job_instance(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "j1", "instance_list": []}))
    job_operations.delete_job_instance("j1", 2)
    assert recorder.last.method == "DELETE"
    assert recorder.last.url.path == "/api/v1/jobs/j1/2"


def test_delete_job(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "j1"}))
    job_operations.delete_job("j1")
    assert recorder.last.method == "DELETE"
    assert recorder.last.url.path == "/api/v1/jobs/j1"


def test_delete_job_204_returns_none(recorder):
    recorder.queue(httpx.Response(204))
    assert job_operations.delete_job("j1") is None
