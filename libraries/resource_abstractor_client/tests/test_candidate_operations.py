import json

import httpx
from resource_abstractor_client import candidate_operations


def test_get_candidates_no_filter(recorder):
    recorder.queue(httpx.Response(200, json=[]))
    candidate_operations.get_candidates()
    assert recorder.last.url.path == "/api/v1/resources"
    assert recorder.last.url.query == b""


def test_active_bool_is_sent_lowercase(recorder):
    """requests used to send active=True (capitalized) on the wire; both abstractors'
    boolean parsers accept it case-insensitively, but httpx's own encoding already
    produces the lowercase form - assert on it so a client-library swap can't silently
    change this without a test failing."""
    recorder.queue(httpx.Response(200, json=[]))
    candidate_operations.get_candidates(active=True)
    assert recorder.last.url.params["active"] == "true"


def test_resources_as_comma_string_passes_through_unchanged(recorder):
    """The one real call site (clusters_blueprints.py) already builds this string
    itself; both abstractors read it as a single raw value."""
    recorder.queue(httpx.Response(200, json=[]))
    candidate_operations.get_candidates(resources="last_modified_timestamp,active")
    assert recorder.last.url.params["resources"] == "last_modified_timestamp,active"


def test_resources_as_list_is_comma_joined_not_repeated(recorder):
    """resources is style: form, explode: false in the spec - a single comma-joined
    value. Confirmed against a live go_resource_abstractor: the server's query binder
    (oapi-codegen's runtime.BindQueryParameterWithOptions) enforces this literally -
    ?resources=a,b succeeds, but the repeated form (?resources=a&resources=b, which is
    httpx's own default encoding of a Python list) is a 400 Bad Request. get_candidates
    joins a list itself before it reaches the generated request builder."""
    recorder.queue(httpx.Response(200, json=[]))
    candidate_operations.get_candidates(resources=["gpu_temp", "gpu_drivers"])
    assert recorder.last.url.params["resources"] == "gpu_temp,gpu_drivers"
    query = str(recorder.last.url)
    assert "resources=gpu_temp&" not in query


def test_get_candidate_by_id(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "c1"}))
    candidate_operations.get_candidate_by_id("c1")
    assert recorder.last.url.path == "/api/v1/resources/c1"


def test_get_candidate_by_name_sends_candidate_name(recorder):
    """Fixed from the original client, which sent cluster_name= - a param the
    server has never accepted. Dead code before this port (zero call sites), so
    this is a behaviour change with no blast radius."""
    recorder.queue(httpx.Response(200, json=[{"_id": "c1", "candidate_name": "mine"}]))
    result = candidate_operations.get_candidate_by_name("mine")
    assert recorder.last.url.params["candidate_name"] == "mine"
    assert "cluster_name" not in recorder.last.url.params
    assert result == {"_id": "c1", "candidate_name": "mine"}


def test_get_candidate_by_name_empty_result_is_none(recorder):
    recorder.queue(httpx.Response(200, json=[]))
    assert candidate_operations.get_candidate_by_name("nobody") is None


def test_get_candidate_by_ip(recorder):
    recorder.queue(httpx.Response(200, json=[{"_id": "c1", "ip": "1.2.3.4"}]))
    result = candidate_operations.get_candidate_by_ip("1.2.3.4")
    assert recorder.last.url.params["ip"] == "1.2.3.4"
    assert result == {"_id": "c1", "ip": "1.2.3.4"}


def test_update_candidate_information_patches(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "c1", "cpu_percent": 5}))
    candidate_operations.update_candidate_information("c1", {"cpu_percent": 5})
    assert recorder.last.method == "PATCH"
    assert recorder.last.url.path == "/api/v1/resources/c1"
    assert json.loads(recorder.last.content) == {"cpu_percent": 5}


def test_update_candidate_information_empty_body_sends_empty_object(recorder):
    recorder.queue(httpx.Response(200, json={"_id": "c1"}))
    candidate_operations.update_candidate_information("c1", {})
    assert recorder.last.content == b"{}"


def test_create_candidate_is_a_put_upsert(recorder):
    """Named create, but the resource abstractor treats POST /api/v1/resources as a
    real create and PUT as an upsert-by-candidate_name - this has always issued PUT,
    a wrinkle worth a separate cleanup but out of scope here."""
    recorder.queue(httpx.Response(200, json={"_id": "c1"}))
    candidate_operations.create_candidate({"candidate_name": "c1"})
    assert recorder.last.method == "PUT"
    assert recorder.last.url.path == "/api/v1/resources"


def test_undeclared_field_survives_round_trip_through_the_model(recorder):
    """Resource is additionalProperties: true - a field the spec doesn't know about
    (like a GPU vendor's own metric) must still reach the wire untouched."""
    recorder.queue(httpx.Response(200, json={"_id": "c1"}))
    candidate_operations.update_candidate_information("c1", {"gpu_temp": 42})
    assert json.loads(recorder.last.content) == {"gpu_temp": 42}
