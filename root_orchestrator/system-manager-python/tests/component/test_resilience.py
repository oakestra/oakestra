"""What the public flows do when resource-abstractor, a cluster manager or the jwt
generator is down. The happy paths are tested in the per-endpoint files.
"""

import pytest

pytestmark = pytest.mark.component


# ---------------------------------------------------------------------------- resource abstractor down


def test_login_succeeds_when_resource_abstractor_down(client, resource_abstractor):
    # Users live in Mongo, not the resource abstractor, so login must be unaffected.
    resource_abstractor.go_down()

    response = client.post("/api/auth/login", json={"username": "Admin", "password": "Admin"})

    assert response.status_code == 200
    assert "token" in response.get_json()


def test_get_user_applications_returns_500_when_resource_abstractor_down(
    client, admin_headers, resource_abstractor, admin_user_id
):
    resource_abstractor.go_down()

    response = client.get(f"/api/applications/{admin_user_id}", headers=admin_headers)

    assert response.status_code == 500


def test_scale_up_instance_is_a_noop_when_resource_abstractor_down(
    client, admin_headers, resource_abstractor, outbound
):
    # The job exists, so the outage is the only reason nothing happens.
    # request_scale_up_instance can't look the job up and just returns, and the route
    # answers 200 regardless.
    job = resource_abstractor.add_service()
    resource_abstractor.go_down()

    response = client.post(f"/api/service/{job['_id']}/instance", headers=admin_headers)

    assert response.status_code == 200
    assert response.get_json() == {"message": "ok"}
    assert outbound.scheduler() == []
    assert resource_abstractor.jobs[job["_id"]].get("status") is None


def test_scheduler_callback_is_a_noop_when_resource_abstractor_down(
    client, resource_abstractor, outbound, make_cluster
):
    # Job and cluster exist, so again only the outage can explain the no-op.
    cluster = make_cluster()
    job = resource_abstractor.add_job(applicationID="app-1", job_name="svc")
    resource_abstractor.go_down()

    response = client.post(
        "/api/result/deploy", json={"job_id": job["_id"], "candidate_id": cluster.id}
    )

    assert response.status_code == 200
    assert response.get_data(as_text=True) == "ok"
    assert outbound.net_plugin(method="POST", path="/api/net/instance/deploy") == []
    assert resource_abstractor.jobs[job["_id"]]["instance_list"] == []


def test_cluster_information_report_returns_400_when_resource_abstractor_down(
    client, resource_abstractor
):
    resource_abstractor.go_down()

    response = client.post(
        "/api/information/000000000000000000000000",
        json={"cpu_percent": "1", "jobs": []},
    )

    assert response.status_code == 400


# ---------------------------------------------------------------------------- cluster manager down


def test_scheduler_callback_persists_instance_when_cluster_manager_unreachable(
    client, make_cluster, resource_abstractor, outbound
):
    # cluster_request_to_deploy (ext_requests/cluster_requests.py) swallows every
    # exception, so the job's status and instance are persisted whether or not the
    # cluster manager could be reached.
    cluster = make_cluster()
    job = resource_abstractor.add_job(
        job_name="unreachable-cluster-job",
        applicationID="app-1",
        microservice_name="svc",
        next_instance_progressive_number=0,
        instance_list=[],
    )
    cluster.go_down()

    response = client.post(
        "/api/result/deploy", json={"job_id": job["_id"], "candidate_id": cluster.id}
    )

    assert response.status_code == 200
    assert response.get_data(as_text=True) == "ok"

    persisted = resource_abstractor.jobs[job["_id"]]
    assert persisted["status"] == "CLUSTER_SCHEDULED"
    assert len(persisted["instance_list"]) == 1
    assert persisted["instance_list"][0]["cluster_id"] == cluster.id

    # The deploy is still attempted.
    attempted = outbound.cluster(cluster, method="POST", path=f"/api/service/{job['_id']}/0")
    assert len(attempted) == 1


# ---------------------------------------------------------------------------- jwt generator down


def test_login_fails_with_500_and_no_token_when_jwt_generator_down(client, jwt_generator_down):
    response = client.post("/api/auth/login", json={"username": "Admin", "password": "Admin"})

    assert response.status_code == 500
    assert "token" not in (response.get_json(silent=True) or {})
