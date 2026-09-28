"""Component tests for /api/service(s) - blueprints/services_blueprints.py and
services/service_management.py.
"""

import pytest

from tests.component.conftest import APP_NO_SERVICES

pytestmark = pytest.mark.component


def service_sla(app_id, name="svcB", virtualization="container"):
    return {
        "sla_version": "v2.0",
        "customerID": "x",
        "applications": [
            {
                "applicationID": app_id,
                "application_name": "AppOne",
                "application_namespace": "test",
                "microservices": [
                    {
                        "microservice_name": name,
                        "microservice_namespace": "test",
                        "virtualization": virtualization,
                        "code": "docker.io/library/redis",
                        "port": "6379:6379",
                    }
                ],
            }
        ],
    }


@pytest.fixture
def app_id(client, admin_headers):
    created = client.post("/api/application/", json=APP_NO_SERVICES, headers=admin_headers)
    assert created.status_code == 200, created.get_data(as_text=True)
    return created.get_json()[0]["_id"]


# ---------------------------------------------------------------------------- create


def test_add_service_creates_job_and_updates_app(
    client, admin_headers, resource_abstractor, outbound, app_id
):
    response = client.post("/api/service/", json=service_sla(app_id), headers=admin_headers)

    assert response.status_code == 200, response.get_data(as_text=True)
    body = response.get_json()
    job_id = body["job_id"]
    assert body["deployed_services"] == [{"service_name": "svcB", "status": 200}]
    assert body["failed_services"] == []

    job = resource_abstractor.jobs[job_id]
    assert job["virtualization"] == "docker"  # "container" is mapped to the docker driver name
    assert job["image"] == "docker.io/library/redis"  # image comes from "code"
    assert job["next_instance_progressive_number"] == 0
    assert job["instance_list"] == []
    assert job["applicationID"] == app_id
    assert job["microserviceID"] == job_id

    assert resource_abstractor.apps[app_id]["microservices"] == [job_id]

    deploy_calls = outbound.net_plugin(method="POST", path="/api/net/service/deploy")
    assert len(deploy_calls) == 1
    assert deploy_calls[0].json["_id"] == job_id
    assert deploy_calls[0].json["deployment_descriptor"]["applicationID"] == app_id


def test_add_service_net_plugin_failure_rolls_back(
    client, admin_headers, resource_abstractor, net_plugin_fails, app_id
):
    response = client.post("/api/service/", json=service_sla(app_id), headers=admin_headers)

    assert response.status_code == 200, response.get_data(as_text=True)
    body = response.get_json()
    assert body["deployed_services"] == []
    assert len(body["failed_services"]) == 1
    assert body["failed_services"][0]["service_name"] == "svcB"
    assert body["failed_services"][0]["status"] == 500

    assert resource_abstractor.jobs == {}
    assert resource_abstractor.apps[app_id]["microservices"] == []


@pytest.mark.xfail(
    strict=True,
    reason=(
        "bug: blueprints/services_blueprints.py ServiceControllerPost.post() calls "
        "abort(status, result) inside a bare `except Exception`, which catches the "
        "HTTPException and re-aborts with 400 'SLA not formatted correctly', so the 404 "
        "(unknown app) and 422 (invalid SLA) from create_services_of_app never reach "
        "the client"
    ),
)
@pytest.mark.parametrize(
    ("make_sla", "expected_status"),
    [
        (lambda app_id: service_sla("000000000000000000000000"), 404),
        (lambda app_id: service_sla(app_id, name="bad.name"), 422),
    ],
    ids=["unknown_application", "invalid_service_name"],
)
def test_add_service_rejection_keeps_its_status_and_has_no_side_effects(
    client, admin_headers, resource_abstractor, outbound, app_id, make_sla, expected_status
):
    response = client.post("/api/service/", json=make_sla(app_id), headers=admin_headers)

    assert resource_abstractor.jobs == {}
    assert outbound.net_plugin(path="/api/net/service/deploy") == []
    assert response.status_code == expected_status


# ---------------------------------------------------------------------------- read


def test_get_service_returns_job_document(client, admin_headers, resource_abstractor, app_id):
    created = client.post("/api/service/", json=service_sla(app_id), headers=admin_headers)
    job_id = created.get_json()["job_id"]

    response = client.get(f"/api/service/{job_id}", headers=admin_headers)

    assert response.status_code == 200
    assert response.get_json()["_id"] == job_id


def test_get_service_of_another_user_not_found(client, create_user):
    alice = create_user("alice")
    bob = create_user("bob")
    created_app = client.post("/api/application/", json=APP_NO_SERVICES, headers=alice)
    app_id_alice = created_app.get_json()[0]["_id"]
    created_service = client.post("/api/service/", json=service_sla(app_id_alice), headers=alice)
    job_id = created_service.get_json()["job_id"]

    response = client.get(f"/api/service/{job_id}", headers=bob)

    assert response.status_code == 404


def test_get_services_of_application(client, admin_headers, app_id):
    client.post("/api/service/", json=service_sla(app_id, name="svcB"), headers=admin_headers)
    client.post("/api/service/", json=service_sla(app_id, name="svcC"), headers=admin_headers)

    response = client.get(f"/api/services/{app_id}", headers=admin_headers)

    assert response.status_code == 200
    names = {job["microservice_name"] for job in response.get_json()}
    assert names == {"svcB", "svcC"}


def test_get_services_of_another_users_application_not_found(
    client, admin_headers, create_user, app_id
):
    client.post("/api/service/", json=service_sla(app_id), headers=admin_headers)
    alice = create_user("alice")

    response = client.get(f"/api/services/{app_id}", headers=alice)

    assert response.status_code == 404


def test_get_all_services_requires_admin_role(client, admin_headers, create_user):
    alice = create_user("alice")
    created = client.post("/api/application/", json=APP_NO_SERVICES, headers=alice)
    app_id_alice = created.get_json()[0]["_id"]
    client.post("/api/service/", json=service_sla(app_id_alice), headers=alice)

    forbidden = client.get("/api/services/", headers=alice)
    allowed = client.get("/api/services/", headers=admin_headers)

    assert forbidden.status_code == 403
    assert allowed.status_code == 200
    assert len(allowed.get_json()) == 1


# ---------------------------------------------------------------------------- update


def test_update_service_not_implemented(client, admin_headers, app_id):
    created = client.post("/api/service/", json=service_sla(app_id), headers=admin_headers)
    job_id = created.get_json()["job_id"]
    body = {"applications": [{"microservices": [{"microservice_name": "renamed"}]}]}

    response = client.put(f"/api/service/{job_id}", json=body, headers=admin_headers)

    assert response.status_code == 501


# ---------------------------------------------------------------------------- delete


def test_delete_service_removes_job_updates_app_and_informs_net_plugin(
    client, admin_headers, resource_abstractor, outbound, app_id
):
    created = client.post("/api/service/", json=service_sla(app_id), headers=admin_headers)
    job_id = created.get_json()["job_id"]

    response = client.delete(f"/api/service/{job_id}", headers=admin_headers)

    assert response.status_code == 200
    assert job_id not in resource_abstractor.jobs
    assert resource_abstractor.apps[app_id]["microservices"] == []
    undeploy_calls = outbound.net_plugin(method="DELETE", path=f"/api/net/service/{job_id}")
    assert len(undeploy_calls) == 1


def test_delete_service_with_running_instance_also_deletes_it_on_the_cluster(
    client, admin_headers, resource_abstractor, outbound, make_cluster
):
    cluster = make_cluster()
    app = resource_abstractor.add_app(
        userId="Admin", application_name="AppThree", application_namespace="test"
    )
    job = resource_abstractor.add_job(
        applicationID=app["_id"],
        job_name="AppThree.test.svcD.test",
        status="RUNNING",
        next_instance_progressive_number=1,
        instance_list=[
            {
                "instance_number": 0,
                "cluster_id": cluster.id,
                "cluster_location": "loc",
                "status": "CLUSTER_SCHEDULED",
            }
        ],
    )
    resource_abstractor.apps[app["_id"]]["microservices"] = [job["_id"]]

    response = client.delete(f"/api/service/{job['_id']}", headers=admin_headers)

    assert response.status_code == 200
    assert job["_id"] not in resource_abstractor.jobs
    cluster_calls = outbound.cluster(cluster, method="DELETE", path=f"/api/service/{job['_id']}/0")
    assert len(cluster_calls) == 1


def test_delete_service_of_another_user_leaves_it_untouched(
    client, create_user, resource_abstractor
):
    alice = create_user("alice")
    bob = create_user("bob")
    created_app = client.post("/api/application/", json=APP_NO_SERVICES, headers=alice)
    app_id_alice = created_app.get_json()[0]["_id"]
    created_service = client.post("/api/service/", json=service_sla(app_id_alice), headers=alice)
    job_id = created_service.get_json()["job_id"]

    response = client.delete(f"/api/service/{job_id}", headers=bob)

    assert response.status_code == 500
    assert job_id in resource_abstractor.jobs
    assert resource_abstractor.apps[app_id_alice]["microservices"] == [job_id]
