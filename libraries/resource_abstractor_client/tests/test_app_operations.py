import json

import httpx
from resource_abstractor_client import app_operations


def test_get_apps_no_filter(recorder):
    recorder.queue(httpx.Response(200, json=[]))
    app_operations.get_apps()
    assert recorder.last.method == "GET"
    assert recorder.last.url.path == "/api/v1/applications"
    assert recorder.last.url.query == b""


def test_get_user_apps_sends_userid_on_the_wire(recorder):
    recorder.queue(httpx.Response(200, json=[]))
    app_operations.get_user_apps("u1")
    assert recorder.last.url.params["userId"] == "u1"
    assert "user_id" not in recorder.last.url.params


def test_get_app_by_name_and_namespace_returns_first_match(recorder):
    recorder.queue(httpx.Response(200, json=[{"_id": "a"}, {"_id": "b"}]))
    result = app_operations.get_app_by_name_and_namespace("n", "ns", "u1")
    assert result == {"_id": "a"}
    params = recorder.last.url.params
    assert params["application_name"] == "n"
    assert params["application_namespace"] == "ns"
    assert params["userId"] == "u1"


def test_get_app_by_name_and_namespace_empty_result_is_none(recorder):
    recorder.queue(httpx.Response(200, json=[]))
    assert app_operations.get_app_by_name_and_namespace("n", "ns", "u1") is None


def test_get_app_by_id_scopes_to_user(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "a"}))
    app_operations.get_app_by_id("a", "u1")
    assert recorder.last.url.path == "/api/v1/applications/a"
    assert recorder.last.url.params["userId"] == "u1"


def test_create_app_posts_with_userid_injected(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "new"}))
    data = {"application_name": "n", "custom_field": "x"}
    result = app_operations.create_app("u1", data)
    assert recorder.last.method == "POST"
    assert recorder.last.url.path == "/api/v1/applications"
    sent = json.loads(recorder.last.content)
    assert sent == {"application_name": "n", "custom_field": "x", "userId": "u1"}
    assert result == {"_id": "new"}


def test_create_app_mutates_callers_dict(recorder):
    """Preserved from the original client on purpose - changing this would be a
    caller-visible behaviour change, out of scope for this port."""
    recorder.queue(httpx.Response(200, json={"_id": "new"}))
    data = {"application_name": "n"}
    app_operations.create_app("u1", data)
    assert data["userId"] == "u1"


def test_update_app_patches(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "a"}))
    app_operations.update_app("a", "u1", {"application_desc": "d"})
    assert recorder.last.method == "PATCH"
    assert recorder.last.url.path == "/api/v1/applications/a"


def test_delete_app(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "a"}))
    app_operations.delete_app("a")
    assert recorder.last.method == "DELETE"
    assert recorder.last.url.path == "/api/v1/applications/a"


def test_get_apps_ignores_unsupported_kwarg(recorder, caplog):
    recorder.queue(httpx.Response(200, json=[]))
    with caplog.at_level("WARNING"):
        app_operations.get_apps(not_a_real_filter="x")
    assert "not_a_real_filter" not in str(recorder.last.url.params)
    assert "not_a_real_filter" in caplog.text
