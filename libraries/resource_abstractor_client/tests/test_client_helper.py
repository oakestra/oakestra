import httpx
import pytest
from resource_abstractor_client import client_helper
from resource_abstractor_client.openapi.api.jobs import get_job


def test_404_returns_none(recorder):
    recorder.queue(httpx.Response(404, json={"message": "Not Found"}))
    result = client_helper.make_request(get_job._get_kwargs("missing"))
    assert result is None


def test_5xx_returns_none(recorder):
    recorder.queue(httpx.Response(500, json={"message": "Internal Server Error"}))
    result = client_helper.make_request(get_job._get_kwargs("job1"))
    assert result is None


def test_connection_failure_returns_none(monkeypatch):
    def boom(request):
        raise httpx.ConnectError("connection refused", request=request)

    fake_client = client_helper.Client(
        base_url="http://unreachable:11011",
        httpx_args={"transport": httpx.MockTransport(boom)},
    )
    monkeypatch.setattr(client_helper, "RESOURCE_ABSTRACTOR_ADDR", "http://unreachable:11011")
    monkeypatch.setitem(client_helper._clients, "http://unreachable:11011", fake_client)

    result = client_helper.make_request(get_job._get_kwargs("job1"))
    assert result is None


def test_empty_body_204_returns_none(recorder):
    recorder.queue(httpx.Response(204))
    result = client_helper.make_request(get_job._get_kwargs("job1"))
    assert result is None


def test_2xx_with_body_decodes_json(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "abc", "job_name": "n"}))
    result = client_helper.make_request(get_job._get_kwargs("job1"))
    assert result == {"_id": "abc", "job_name": "n"}


def test_list_response_stays_a_list(recorder):
    recorder.queue(httpx.Response(200, json=[{"_id": "a"}, {"_id": "b"}]))
    result = client_helper.make_request(get_job._get_kwargs("job1"))
    assert result == [{"_id": "a"}, {"_id": "b"}]


def test_undeclared_fields_survive_round_trip(recorder):
    """The abstractor is a schemaless Mongo pass-through - fields the spec doesn't
    declare (microserviceID, next_instance_progressive_number, ...) must come back
    exactly as sent, since make_request never parses into a typed model."""
    doc = {"_id": "abc", "microserviceID": "abc", "next_instance_progressive_number": 3}
    recorder.queue(httpx.Response(200, json=doc))
    result = client_helper.make_request(get_job._get_kwargs("job1"))
    assert result == doc


class TestTranslateKwargs:
    def test_translates_wire_name_to_python_name(self):
        result = client_helper.translate_kwargs({"userId": "u1"}, {"userId": "user_id"})
        assert result == {"user_id": "u1"}

    def test_drops_unsupported_keys_with_a_warning(self, caplog):
        wire_to_python = {"job_name": "job_name"}
        with caplog.at_level("WARNING"):
            result = client_helper.translate_kwargs(
                {"job_name": "n", "instance_list": {"$elemMatch": {}}}, wire_to_python
            )
        assert result == {"job_name": "n"}
        assert "instance_list" in caplog.text

    def test_empty_kwargs_translate_to_empty(self):
        assert client_helper.translate_kwargs({}, {"job_name": "job_name"}) == {}


@pytest.mark.parametrize("addr", ["http://a:1", "http://b:2"])
def test_client_is_cached_per_address(monkeypatch, addr):
    monkeypatch.setattr(client_helper, "RESOURCE_ABSTRACTOR_ADDR", addr)
    monkeypatch.setattr(client_helper, "_clients", {})
    first = client_helper._client()
    second = client_helper._client()
    assert first is second
