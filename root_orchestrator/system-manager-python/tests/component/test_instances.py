"""Scale up/down of service instances: deployment_blueprints, instance_management,
scheduler_requests and cluster_requests."""

import pytest
from oakestra_utils.types.statuses import PositiveSchedulingStatus

pytestmark = pytest.mark.component


# ---------------------------------------------------------------------------- scale up


def test_scale_up_sets_requested_status_and_calls_scheduler_once(
    client, admin_headers, resource_abstractor, outbound
):
    job = resource_abstractor.add_service()

    response = client.post(f"/api/service/{job['_id']}/instance", headers=admin_headers)

    assert response.status_code == 200
    stored = resource_abstractor.jobs[job["_id"]]
    assert stored["status"] == PositiveSchedulingStatus.REQUESTED.value
    assert stored["status_detail"] == "Waiting for scheduling decision"
    calls = outbound.scheduler(method="POST", path="/api/calculate/deploy")
    assert len(calls) == 1
    assert calls[0].json["_id"] == job["_id"]


def test_scale_up_for_other_users_service_is_noop(
    client, resource_abstractor, outbound, create_user
):
    other_headers = create_user("bob")
    job = resource_abstractor.add_service(owner="Admin")

    response = client.post(f"/api/service/{job['_id']}/instance", headers=other_headers)

    # The blueprint does not check whether the scale-up actually happened, it always
    # answers "ok" to the caller.
    assert response.status_code == 200
    assert outbound.scheduler() == []
    assert resource_abstractor.jobs[job["_id"]].get("status") is None


def test_scale_up_for_unknown_service_is_noop(client, admin_headers, outbound):
    response = client.post("/api/service/000000000000000000000000/instance", headers=admin_headers)

    assert response.status_code == 200
    assert outbound.scheduler() == []


def test_scale_up_for_service_not_in_application_is_noop(
    client, admin_headers, resource_abstractor, outbound
):
    job = resource_abstractor.add_service(in_microservices=False)

    response = client.post(f"/api/service/{job['_id']}/instance", headers=admin_headers)

    assert response.status_code == 200
    assert outbound.scheduler() == []
    assert resource_abstractor.jobs[job["_id"]].get("status") is None


def test_scale_up_when_scheduler_down_still_answers_ok(
    client, admin_headers, resource_abstractor, scheduler_down
):
    job = resource_abstractor.add_service()

    response = client.post(f"/api/service/{job['_id']}/instance", headers=admin_headers)

    # The scheduler call is fired from a background thread and its errors are swallowed,
    # so the request-scale-up call itself never sees the failure.
    assert response.status_code == 200
    stored = resource_abstractor.jobs[job["_id"]]
    assert stored["status"] == PositiveSchedulingStatus.REQUESTED.value


# ---------------------------------------------------------------------------- scale down


def test_scale_down_deletes_only_the_requested_instance(
    client, admin_headers, resource_abstractor, outbound, make_cluster
):
    cluster_a = make_cluster()
    cluster_b = make_cluster()
    instances = [
        {"instance_number": 0, "cluster_id": cluster_a.id, "status": "CLUSTER_SCHEDULED"},
        {"instance_number": 1, "cluster_id": cluster_b.id, "status": "CLUSTER_SCHEDULED"},
    ]
    job = resource_abstractor.add_service(
        instance_list=instances,
        next_instance_progressive_number=2,
        status="CLUSTER_SCHEDULED",
    )

    response = client.delete(f"/api/service/{job['_id']}/instance/0", headers=admin_headers)

    assert response.status_code == 200
    a_calls = outbound.cluster(cluster_a, method="DELETE")
    assert [c.path for c in a_calls] == [f"/api/service/{job['_id']}/0"]
    assert outbound.cluster(cluster_b, method="DELETE") == []
    net_calls = outbound.net_plugin(method="DELETE")
    assert [c.path for c in net_calls] == [f"/api/net/{job['_id']}/0"]
    remaining = resource_abstractor.jobs[job["_id"]]["instance_list"]
    assert [i["instance_number"] for i in remaining] == [1]


# ---------------------------------------------------------------------------- known bugs


def _seed_three_instances(resource_abstractor, cluster):
    instances = [
        {"instance_number": i, "cluster_id": cluster.id, "status": "CLUSTER_SCHEDULED"}
        for i in range(3)
    ]
    return resource_abstractor.add_service(
        instance_list=instances,
        next_instance_progressive_number=3,
        status="CLUSTER_SCHEDULED",
    )


@pytest.mark.xfail(
    strict=True,
    reason="bug: services/instance_management.py:106 mutates instances while iterating over "
    "it, so 'delete all' (which_one=-1) skips the middle instance instead of removing all 3",
)
def test_scale_down_all_removes_every_instance(
    client, admin_headers, resource_abstractor, make_cluster
):
    cluster = make_cluster()
    job = _seed_three_instances(resource_abstractor, cluster)

    response = client.delete(f"/api/service/{job['_id']}/instance/-1", headers=admin_headers)

    assert response.status_code == 200
    remaining = resource_abstractor.jobs[job["_id"]]["instance_list"]
    assert remaining == []


@pytest.mark.xfail(
    strict=True,
    reason="bug: services/instance_management.py:104 passes which_one (always -1 for 'delete "
    "all') to net_inform_instance_undeploy instead of the instance's own instance_number",
)
def test_scale_down_all_informs_net_plugin_of_the_instance_number(
    client, admin_headers, resource_abstractor, outbound, make_cluster
):
    # One instance only, so the mutate-while-iterating bug above can't skip anything
    # and the which_one mixup is the only thing that can fail this.
    cluster = make_cluster()
    instances = [{"instance_number": 0, "cluster_id": cluster.id, "status": "CLUSTER_SCHEDULED"}]
    job = resource_abstractor.add_service(
        instance_list=instances,
        next_instance_progressive_number=1,
        status="CLUSTER_SCHEDULED",
    )

    response = client.delete(f"/api/service/{job['_id']}/instance/-1", headers=admin_headers)

    assert response.status_code == 200
    net_calls = outbound.net_plugin(method="DELETE")
    assert [c.path for c in net_calls] == [f"/api/net/{job['_id']}/0"]
