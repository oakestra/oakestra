"""The scheduler's deploy callback: scheduling_blueprints and instance_scale_up_scheduled_handler."""

import pytest
from oakestra_utils.types.statuses import NegativeSchedulingStatus, PositiveSchedulingStatus

pytestmark = pytest.mark.component


def test_deploy_result_success_appends_instance_and_notifies_downstream(
    client, resource_abstractor, outbound, make_cluster
):
    cluster = make_cluster()
    job = resource_abstractor.add_job(applicationID="app1", job_name="svc")

    response = client.post(
        "/api/result/deploy", json={"job_id": job["_id"], "candidate_id": cluster.id}
    )

    assert response.status_code == 200
    stored = resource_abstractor.jobs[job["_id"]]
    assert stored["instance_list"] == [
        {
            "instance_number": 0,
            "cluster_id": cluster.id,
            "cluster_location": "48.1,11.5,1000",  # default candidate_location from make_cluster
            "status": PositiveSchedulingStatus.CLUSTER_SCHEDULED.value,
        }
    ]
    assert stored["next_instance_progressive_number"] == 1
    assert stored["status"] == PositiveSchedulingStatus.CLUSTER_SCHEDULED.value

    net_calls = outbound.net_plugin(method="POST", path="/api/net/instance/deploy")
    assert len(net_calls) == 1
    assert net_calls[0].json == {
        "instance_number": 0,
        "cluster_id": cluster.id,
        "_id": job["_id"],
    }
    cluster_calls = outbound.cluster(cluster, method="POST")
    assert [c.path for c in cluster_calls] == [f"/api/service/{job['_id']}/0"]


def test_two_consecutive_deploy_results_produce_instance_0_then_1(
    client, resource_abstractor, outbound, make_cluster
):
    cluster = make_cluster()
    job = resource_abstractor.add_job(applicationID="app1", job_name="svc")

    for _ in range(2):
        response = client.post(
            "/api/result/deploy", json={"job_id": job["_id"], "candidate_id": cluster.id}
        )
        assert response.status_code == 200

    stored = resource_abstractor.jobs[job["_id"]]
    assert [i["instance_number"] for i in stored["instance_list"]] == [0, 1]
    assert stored["next_instance_progressive_number"] == 2
    cluster_calls = outbound.cluster(cluster, method="POST")
    assert sorted(c.path for c in cluster_calls) == [
        f"/api/service/{job['_id']}/0",
        f"/api/service/{job['_id']}/1",
    ]


@pytest.mark.parametrize(
    ("advertised_ip", "reached_ip"),
    [
        ("10.0.0.9", "10.0.0.9"),
        # Clusters behind dual-stack sockets advertise v4 addresses in mapped form.
        ("::ffff:10.0.0.9", "10.0.0.9"),
        ("fd00::1", "fd00::1"),
    ],
    ids=["ipv4", "ipv4-mapped-ipv6", "ipv6"],
)
def test_deploy_result_reaches_cluster_at_its_advertised_address(
    client, resource_abstractor, outbound, make_cluster, advertised_ip, reached_ip
):
    cluster = make_cluster(registered=False, ip=reached_ip)
    candidate = resource_abstractor.add_candidate(
        candidate_name=cluster.name,
        candidate_location="1,1,1",
        ip=advertised_ip,
        port=str(cluster.port),
    )
    job = resource_abstractor.add_job(applicationID="app1", job_name="svc")

    response = client.post(
        "/api/result/deploy", json={"job_id": job["_id"], "candidate_id": candidate["_id"]}
    )

    assert response.status_code == 200
    calls = outbound.cluster(cluster, method="POST", path=f"/api/service/{job['_id']}/0")
    assert len(calls) == 1


@pytest.mark.parametrize("status", list(NegativeSchedulingStatus), ids=lambda s: s.name)
def test_deploy_result_failure_status_updates_job_without_scheduling(
    client, resource_abstractor, outbound, status
):
    job = resource_abstractor.add_job(applicationID="app1", job_name="svc")

    response = client.post(
        "/api/result/deploy", json={"job_id": job["_id"], "status": status.value}
    )

    assert response.status_code == 200
    stored = resource_abstractor.jobs[job["_id"]]
    assert stored["status"] == status.value
    assert stored["instance_list"] == []
    assert outbound.net_plugin() == []


@pytest.mark.xfail(
    strict=True,
    reason="bug: blueprints/scheduling_blueprints.py calls convert_to_status(status) for the "
    "failure-status branch with nothing catching the ValueError it raises for a name it "
    "doesn't recognise, so a malformed client status crashes with 500 instead of a clean 4xx",
)
def test_deploy_result_unknown_status_string_rejected_cleanly(client, resource_abstractor):
    job = resource_abstractor.add_job(applicationID="app1", job_name="svc", status="REQUESTED")

    response = client.post(
        "/api/result/deploy", json={"job_id": job["_id"], "status": "TOTALLY_UNKNOWN"}
    )

    assert 400 <= response.status_code < 500
    assert resource_abstractor.jobs[job["_id"]]["status"] == "REQUESTED"


def test_deploy_result_net_plugin_failure_leaves_instance_persisted_but_cluster_uncontacted(
    client, resource_abstractor, outbound, make_cluster, net_plugin_fails
):
    cluster = make_cluster()
    job = resource_abstractor.add_job(applicationID="app1", job_name="svc")

    # instance_scale_up_scheduled_handler saves the new instance before calling
    # net_inform_instance_deploy, which (unlike the undeploy call) doesn't catch
    # raise_for_status. So a net plugin failure leaves an instance in the job that the
    # cluster was never told to run.
    # Not an xfail: we haven't decided whether the fix is a rollback or a retry, so
    # there's no correct behaviour to assert yet. This pins what happens today.
    response = client.post(
        "/api/result/deploy", json={"job_id": job["_id"], "candidate_id": cluster.id}
    )

    assert response.status_code == 500
    stored = resource_abstractor.jobs[job["_id"]]
    assert stored["status"] == PositiveSchedulingStatus.CLUSTER_SCHEDULED.value
    assert [i["instance_number"] for i in stored["instance_list"]] == [0]
    assert outbound.cluster(cluster, method="POST") == []
