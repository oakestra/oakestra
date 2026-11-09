"""Cluster resource/job reporting and the cluster listing endpoints: clusters_blueprints."""

import copy

import pytest

pytestmark = pytest.mark.component


def test_report_updates_candidate_fields(client, resource_abstractor, make_cluster):
    cluster = make_cluster()

    response = client.post(f"/api/information/{cluster.id}", json={"cpu_percent": "12", "jobs": []})

    assert response.status_code == 200
    candidate = resource_abstractor.candidates[cluster.id]
    # ResourceSchema declares cpu_percent a Float, so the string report is coerced.
    assert candidate["cpu_percent"] == 12.0


def test_report_ignores_ip_in_body(client, resource_abstractor, make_cluster):
    cluster = make_cluster()

    response = client.post(f"/api/information/{cluster.id}", json={"ip": "9.9.9.9", "jobs": []})

    assert response.status_code == 200
    assert resource_abstractor.candidates[cluster.id]["ip"] == cluster.ip


def test_report_updates_job_status_and_instance_fields(client, resource_abstractor, make_cluster):
    cluster = make_cluster()
    job = resource_abstractor.add_job(
        applicationID="app1",
        job_name="svc",
        status="CLUSTER_SCHEDULED",
        instance_list=[
            {"instance_number": 0, "cluster_id": cluster.id, "status": "CLUSTER_SCHEDULED"}
        ],
    )
    body = {
        "jobs": [
            {
                "_id": job["_id"],
                "status": "RUNNING",
                "status_detail": "container up",
                "instance_list": [
                    {
                        "instance_number": 0,
                        "status": "RUNNING",
                        "cpu_percent": "5",
                        "memory_percent": "10",
                        "logs": "started ok",
                    }
                ],
            }
        ]
    }

    response = client.post(f"/api/information/{cluster.id}", json=body)

    assert response.status_code == 200
    stored = resource_abstractor.jobs[job["_id"]]
    assert stored["status"] == "RUNNING"
    assert stored["status_detail"] == "container up"
    instance = stored["instance_list"][0]
    assert instance["status"] == "RUNNING"
    assert instance["cpu_percent"] == "5"
    assert instance["memory_percent"] == "10"
    assert instance["logs"] == "started ok"


def test_report_of_unknown_job_asks_cluster_to_delete_it(
    client, resource_abstractor, outbound, make_cluster
):
    cluster = make_cluster()
    body = {"jobs": [{"_id": "000000000000000000000000", "status": "RUNNING", "instance_list": []}]}

    response = client.post(f"/api/information/{cluster.id}", json=body)

    assert response.status_code == 200
    calls = outbound.cluster(cluster, method="DELETE")
    assert [c.path for c in calls] == ["/api/service/000000000000000000000000/-1"]


def test_report_for_malformed_cluster_id_returns_400(client):
    response = client.post("/api/information/not-a-valid-objectid", json={"jobs": []})

    assert response.status_code == 400


@pytest.mark.xfail(
    strict=True,
    reason='bug: blueprints/clusters_blueprints.py:103 does `del data["jobs"]` unconditionally, '
    "so a report without a jobs key crashes with KeyError instead of a clean 4xx and never "
    "reaches candidate_operations.update_candidate_information",
)
def test_report_missing_jobs_key_rejected_cleanly(client, resource_abstractor, make_cluster):
    cluster = make_cluster()
    before = copy.deepcopy(resource_abstractor.candidates[cluster.id])

    response = client.post(f"/api/information/{cluster.id}", json={"cpu_percent": "1"})

    assert 400 <= response.status_code < 500
    assert resource_abstractor.candidates[cluster.id] == before


def test_list_clusters_maps_candidate_attributes(client, resource_abstractor, make_cluster):
    make_cluster(memory=2048, vcpus=4, vgpus=1)

    response = client.get("/api/clusters/")

    assert response.status_code == 200
    [cluster] = response.get_json()
    assert cluster["cluster_name"] == cluster["candidate_name"]
    assert cluster["cluster_location"] == cluster["candidate_location"]
    # No cpu_percent was ever reported for this candidate, so the mapping falls back to 0.
    assert cluster["aggregated_cpu_percent"] == 0
    assert cluster["memory_in_mb"] == 2048
    assert cluster["total_cpu_cores"] == 4
    assert cluster["total_gpu_cores"] == 1


def test_list_clusters_active_excludes_stale_candidates(client, resource_abstractor, make_cluster):
    fresh = make_cluster(name="fresh")
    stale = make_cluster(name="stale")
    resource_abstractor.candidates[stale.id]["last_modified_timestamp"] = 0

    active_response = client.get("/api/clusters/active")
    all_response = client.get("/api/clusters/")

    assert [c["cluster_name"] for c in active_response.get_json()] == [fresh.name]
    active_by_name = {c["cluster_name"]: c["active"] for c in all_response.get_json()}
    assert active_by_name == {"fresh": True, "stale": False}


@pytest.mark.parametrize(
    "endpoint", ["/api/clusters/", "/api/clusters/active"], ids=["all", "active"]
)
@pytest.mark.xfail(
    strict=True,
    reason="bug: blueprints/clusters_blueprints.py maps over get_candidates() before its "
    "`if clusters is None` check, so a resource-abstractor outage raises TypeError. The "
    "abort(500, ...) behind that check also passes the message positionally, so "
    "flask-smorest would drop it even if the check were reached",
)
def test_list_clusters_when_resource_abstractor_down_returns_clean_error(
    client, resource_abstractor, make_cluster, endpoint
):
    make_cluster()
    resource_abstractor.go_down()

    response = client.get(endpoint)

    assert response.status_code == 500
    assert response.get_json().get("message") == "Getting clusters failed"
