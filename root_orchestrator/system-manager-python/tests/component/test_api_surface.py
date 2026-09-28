"""Component tests for the API surface: the OpenAPI document and a snapshot of every
Flask route's HTTP methods and whether it requires authentication.

Regenerate the snapshot after adding/removing a route or changing its auth requirement:

    UPDATE_SNAPSHOTS=1 .venv/bin/python -m pytest tests/component/test_api_surface.py -q

Then check the diff of tests/component/snapshots/api_surface.json. requires_auth comes
from calling each route with dummy path params and no Authorization header: a 401 or 403
counts as "needs a token". 422 deliberately does not: without a header flask_jwt_extended
never produces it, so a 422 here is a body validation error from a route that let the
request through.

These routes are meant to be open and must show requires_auth: false in the snapshot:
/api/auth/login, /api/user/ (password reset request/apply), /api/result/deploy,
/api/clusters/, /api/clusters/active, /api/information/<clusterid> and /frontend/uploader.
"""

import json
import os
import re
from pathlib import Path

import pytest
from bson import ObjectId

pytestmark = pytest.mark.component

SNAPSHOT_PATH = Path(__file__).parent / "snapshots" / "api_surface.json"

# Flask's static route and flask-swagger-ui's asset routes aren't part of our API.
SKIP_ENDPOINTS = {"static"}
SKIP_ENDPOINT_PREFIXES = ("swagger_ui.",)

# Without a usable body, an open route would crash on a KeyError instead of answering
# from its real logic. Keyed by the route template, not a concrete path.
DUMMY_BODIES = {
    ("POST", "/api/auth/login"): {"username": "Admin", "password": "Admin"},
    ("POST", "/api/user/"): {"username": "no-such-user", "domain": "example.com"},
    ("PUT", "/api/user/"): {"token": "bogus", "password": "x"},
    ("POST", "/api/result/deploy"): {"job_id": "x", "candidate_id": "y"},
    ("POST", "/api/information/<clusterid>"): {"jobs": []},
}
AUTH_FAILURE_CODES = {401, 403}


# A placeholder like <int:n> only matches a value its converter accepts; anything else
# 404s during URL matching and the route would wrongly look open.
DUMMY_BY_CONVERTER = {"int": "0", "float": "0.0"}
PLACEHOLDER = re.compile(r"<(?:(\w+)(?:\([^)]*\))?:)?\w+>")


def _concrete_path(rule):
    return PLACEHOLDER.sub(lambda m: DUMMY_BY_CONVERTER.get(m.group(1), str(ObjectId())), rule.rule)


def _is_skipped(rule):
    return rule.endpoint in SKIP_ENDPOINTS or rule.endpoint.startswith(SKIP_ENDPOINT_PREFIXES)


def _iter_probed_rules(app):
    for rule in sorted(app.url_map.iter_rules(), key=lambda r: (r.rule, str(r.methods))):
        if _is_skipped(rule):
            continue
        for method in sorted((rule.methods or set()) - {"HEAD", "OPTIONS"}):
            yield rule, method


def _probe(client, rule, method):
    """Call one route with no Authorization header and return its status code. The auth
    decorators reject the request before any route logic runs, so a route that crashes
    with a 500 here is one that does not require a token."""
    kwargs = {}
    if method in ("POST", "PUT", "DELETE", "PATCH"):
        kwargs["json"] = DUMMY_BODIES.get((method, rule.rule), {})
    return client.open(_concrete_path(rule), method=method, **kwargs).status_code


def _collect_surface(app, client):
    entries = [
        {
            "method": method,
            "rule": rule.rule,
            "requires_auth": _probe(client, rule, method) in AUTH_FAILURE_CODES,
        }
        for rule, method in _iter_probed_rules(app)
    ]
    entries.sort(key=lambda e: (e["rule"], e["method"]))
    return entries


def test_openapi_json_is_served(client):
    response = client.get("/docs/openapi.json")

    assert response.status_code == 200
    spec = response.get_json()
    assert spec["openapi"].startswith("3.")
    assert "/api/auth/login" in spec["paths"]
    assert len(spec["paths"]) > 10


def test_route_surface_matches_snapshot(app, client):
    surface = _collect_surface(app, client)

    if os.environ.get("UPDATE_SNAPSHOTS") == "1":
        SNAPSHOT_PATH.write_text(json.dumps(surface, indent=2) + "\n")
        pytest.skip("snapshot regenerated; rerun without UPDATE_SNAPSHOTS to verify")

    expected = json.loads(SNAPSHOT_PATH.read_text())
    assert surface == expected


def test_snapshot_marks_the_intentionally_open_routes_as_open():
    """Sanity check on the checked-in snapshot itself, so a bad regeneration (e.g. run
    against a broken harness where everything 401s) is caught even if nobody diffs the
    JSON by eye."""
    entries = {
        (e["method"], e["rule"]): e["requires_auth"] for e in json.loads(SNAPSHOT_PATH.read_text())
    }

    open_routes = [
        ("POST", "/api/auth/login"),
        ("POST", "/api/user/"),
        ("PUT", "/api/user/"),
        ("POST", "/api/result/deploy"),
        ("GET", "/api/clusters/"),
        ("GET", "/api/clusters/active"),
        ("POST", "/api/information/<clusterid>"),
        ("GET", "/frontend/uploader"),
    ]
    for method, rule in open_routes:
        assert entries[(method, rule)] is False, f"{method} {rule} should be open"

    protected_routes = [
        ("GET", "/api/organization/"),
        ("GET", "/api/users/"),
        ("GET", "/api/applications/"),
        ("GET", "/api/user/<username>"),
    ]
    for method, rule in protected_routes:
        assert entries[(method, rule)] is True, f"{method} {rule} should require auth"
