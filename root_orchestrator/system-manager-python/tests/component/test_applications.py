"""Component tests for /api/application(s) - blueprints/applications_blueprints.py,
services/application_management.py and SLA validation.
"""

import json
from pathlib import Path

import pytest

from tests.component.conftest import APP_NO_SERVICES

pytestmark = pytest.mark.component

SLA_DIR = Path(__file__).resolve().parents[1] / "service_level_agreements"
CORRECT_SLAS = sorted(SLA_DIR.glob("sla_correct_*.json"))
FLAWED_SLAS = sorted(SLA_DIR.glob("sla_flawed_*.json"))

# sla_flawed_1 and sla_flawed_3 both omit "sla_version", which crashes the parser
# before it ever gets to validate the rest of the document (see the xfail test below).
FLAWED_SLAS_WITHOUT_SLA_VERSION = {"sla_flawed_1", "sla_flawed_3"}
FLAWED_SLAS_REJECTED_BY_SCHEMA = [
    p for p in FLAWED_SLAS if p.stem not in FLAWED_SLAS_WITHOUT_SLA_VERSION
]
FLAWED_SLAS_MISSING_SLA_VERSION = [
    p for p in FLAWED_SLAS if p.stem in FLAWED_SLAS_WITHOUT_SLA_VERSION
]

APP_WITH_ONE_SERVICE = {
    "sla_version": "v2.0",
    "customerID": "x",
    "applications": [
        {
            "application_name": "AppTwo",
            "application_namespace": "test",
            "application_desc": "d",
            "microservices": [
                {
                    "microservice_name": "svcA",
                    "microservice_namespace": "test",
                    "virtualization": "container",
                    "code": "docker.io/library/nginx",
                    "port": "80:80",
                }
            ],
        }
    ],
}


def _load(path):
    return json.loads(path.read_text())


# ---------------------------------------------------------------------------- create


@pytest.mark.parametrize("sla_path", CORRECT_SLAS, ids=[p.stem for p in CORRECT_SLAS])
def test_create_application_with_correct_sla_registers_app_and_jobs(
    client, admin_headers, resource_abstractor, sla_path
):
    sla = _load(sla_path)
    expected_microservices = sla["applications"][0]["microservices"]

    response = client.post("/api/application/", json=sla, headers=admin_headers)

    assert response.status_code == 200, response.get_data(as_text=True)
    body = response.get_json()
    assert len(body) == 1
    assert body[0]["userId"] == "Admin"
    assert body[0]["application_name"] == sla["applications"][0]["application_name"]
    assert len(resource_abstractor.apps) == 1
    assert len(resource_abstractor.jobs) == len(expected_microservices)


@pytest.mark.parametrize(
    "sla_path", FLAWED_SLAS_REJECTED_BY_SCHEMA, ids=[p.stem for p in FLAWED_SLAS_REJECTED_BY_SCHEMA]
)
def test_create_application_with_flawed_sla_rejected_without_side_effects(
    client, admin_headers, resource_abstractor, outbound, sla_path
):
    sla = _load(sla_path)

    response = client.post("/api/application/", json=sla, headers=admin_headers)

    assert response.status_code == 422
    assert resource_abstractor.apps == {}
    assert resource_abstractor.jobs == {}
    assert outbound.net_plugin() == []


@pytest.mark.xfail(
    strict=True,
    reason=(
        "bug: sla/versioned_sla_parser.py:19 indexes json_data['sla_version'] before "
        "validating the document, so an SLA missing that key raises an unhandled "
        "KeyError instead of the 422 every other malformed SLA gets"
    ),
)
@pytest.mark.parametrize(
    "sla_path",
    FLAWED_SLAS_MISSING_SLA_VERSION,
    ids=[p.stem for p in FLAWED_SLAS_MISSING_SLA_VERSION],
)
def test_create_application_missing_sla_version_rejected_cleanly(
    client, admin_headers, resource_abstractor, sla_path
):
    sla = _load(sla_path)

    response = client.post("/api/application/", json=sla, headers=admin_headers)

    assert response.status_code == 422
    assert resource_abstractor.apps == {}


def test_create_application_duplicate_name_and_namespace_conflicts(
    client, admin_headers, resource_abstractor
):
    first = client.post("/api/application/", json=APP_NO_SERVICES, headers=admin_headers)
    assert first.status_code == 200, first.get_data(as_text=True)

    second = client.post("/api/application/", json=APP_NO_SERVICES, headers=admin_headers)

    assert second.status_code == 409
    assert len(resource_abstractor.apps) == 1


def test_create_application_resource_abstractor_down_leaves_nothing_behind(
    client, admin_headers, resource_abstractor, outbound
):
    resource_abstractor.go_down()

    response = client.post("/api/application/", json=APP_NO_SERVICES, headers=admin_headers)

    assert response.status_code == 500
    assert resource_abstractor.apps == {}
    assert outbound.net_plugin() == []


def test_create_application_service_creation_fails_leaves_empty_app(
    client, admin_headers, resource_abstractor, net_plugin_fails
):
    """create_services_of_app reports the failure in its own response, but register_app
    discards that response and always returns get_user_apps(), so a failed service
    deploy is invisible to the POST /api/application/ caller."""
    response = client.post("/api/application/", json=APP_WITH_ONE_SERVICE, headers=admin_headers)

    assert response.status_code == 200, response.get_data(as_text=True)
    assert len(resource_abstractor.apps) == 1
    app = next(iter(resource_abstractor.apps.values()))
    assert app["microservices"] == []
    assert resource_abstractor.jobs == {}


# ---------------------------------------------------------------------------- read


def test_get_application_returns_own_app(client, admin_headers, resource_abstractor):
    created = client.post("/api/application/", json=APP_NO_SERVICES, headers=admin_headers)
    app_id = created.get_json()[0]["_id"]

    response = client.get(f"/api/application/{app_id}", headers=admin_headers)

    assert response.status_code == 200
    assert response.get_json()["application_name"] == "AppOne"


def test_get_application_of_another_user_not_found(client, create_user):
    alice = create_user("alice")
    bob = create_user("bob")
    created = client.post("/api/application/", json=APP_NO_SERVICES, headers=alice)
    app_id = created.get_json()[0]["_id"]

    response = client.get(f"/api/application/{app_id}", headers=bob)

    assert response.status_code == 404


# ---------------------------------------------------------------------------- update


def test_update_application_changes_name_and_namespace(client, admin_headers, resource_abstractor):
    created = client.post("/api/application/", json=APP_NO_SERVICES, headers=admin_headers)
    app_id = created.get_json()[0]["_id"]

    response = client.put(
        f"/api/application/{app_id}",
        json={"application_name": "AppOneRenamed", "application_namespace": "test"},
        headers=admin_headers,
    )

    assert response.status_code == 200
    assert resource_abstractor.apps[app_id]["application_name"] == "AppOneRenamed"


@pytest.mark.xfail(
    strict=True,
    reason=(
        "bug: services/application_management.py:70-78 update_app() always writes "
        "fields.get('microservices') into the PATCH payload, so a PUT that only renames "
        "an app sets its microservices list to None"
    ),
)
def test_update_application_without_microservices_field_keeps_existing_services(
    client, admin_headers, resource_abstractor
):
    created = client.post("/api/application/", json=APP_WITH_ONE_SERVICE, headers=admin_headers)
    app_id = created.get_json()[0]["_id"]
    original_services = list(resource_abstractor.apps[app_id]["microservices"])
    assert original_services

    response = client.put(
        f"/api/application/{app_id}",
        json={"application_name": "AppTwoRenamed", "application_namespace": "test"},
        headers=admin_headers,
    )

    assert response.status_code == 200
    assert resource_abstractor.apps[app_id]["microservices"] == original_services


@pytest.mark.xfail(
    strict=True,
    reason=(
        "bug: blueprints/applications_blueprints.py ApplicationController.put() never "
        "checks that the caller owns appid before calling update_app(), so any "
        "authenticated user can overwrite another user's application and, because the "
        "client sets userId to the caller, take ownership of it"
    ),
)
def test_update_application_of_another_user_forbidden(client, create_user, resource_abstractor):
    alice = create_user("alice")
    bob = create_user("bob")
    created = client.post("/api/application/", json=APP_NO_SERVICES, headers=alice)
    app_id = created.get_json()[0]["_id"]

    response = client.put(
        f"/api/application/{app_id}",
        json={"application_name": "Hijacked", "application_namespace": "test"},
        headers=bob,
    )

    assert response.status_code in (401, 403, 404)
    assert resource_abstractor.apps[app_id]["application_name"] == "AppOne"
    assert resource_abstractor.apps[app_id]["userId"] == "alice"


# ---------------------------------------------------------------------------- delete


def test_delete_application_cascades_to_services_and_net_plugin(
    client, admin_headers, resource_abstractor, outbound
):
    created = client.post("/api/application/", json=APP_WITH_ONE_SERVICE, headers=admin_headers)
    app_id = created.get_json()[0]["_id"]
    job_id = resource_abstractor.apps[app_id]["microservices"][0]

    response = client.delete(f"/api/application/{app_id}", headers=admin_headers)

    assert response.status_code == 200
    assert app_id not in resource_abstractor.apps
    assert job_id not in resource_abstractor.jobs
    undeploy_calls = outbound.net_plugin(method="DELETE", path=f"/api/net/service/{job_id}")
    assert len(undeploy_calls) == 1


def test_delete_application_of_another_user_leaves_it_untouched(
    client, create_user, resource_abstractor
):
    alice = create_user("alice")
    bob = create_user("bob")
    created = client.post("/api/application/", json=APP_NO_SERVICES, headers=alice)
    app_id = created.get_json()[0]["_id"]

    response = client.delete(f"/api/application/{app_id}", headers=bob)

    assert response.status_code == 501
    assert app_id in resource_abstractor.apps


# ---------------------------------------------------------------------------- listing


def test_get_applications_of_user_by_id_ownership_check(
    client, admin_headers, create_user, mongo, admin_user_id
):
    alice = create_user("alice")
    client.post("/api/application/", json=APP_NO_SERVICES, headers=alice)
    alice_id = str(mongo.users.find_one({"name": "alice"})["_id"])

    own = client.get(f"/api/applications/{alice_id}", headers=alice)
    other = client.get(f"/api/applications/{admin_user_id}", headers=alice)

    assert own.status_code == 200
    assert len(own.get_json()) == 1
    assert other.status_code == 401


def test_get_all_applications_requires_admin_role(client, admin_headers, create_user):
    alice = create_user("alice")
    client.post("/api/application/", json=APP_NO_SERVICES, headers=alice)
    other = {**APP_NO_SERVICES}
    other["applications"] = [{**APP_NO_SERVICES["applications"][0], "application_name": "AppOther"}]
    client.post("/api/application/", json=other, headers=admin_headers)

    forbidden = client.get("/api/applications/", headers=alice)
    allowed = client.get("/api/applications/", headers=admin_headers)

    assert forbidden.status_code == 403
    assert allowed.status_code == 200
    assert len(allowed.get_json()) == 2
