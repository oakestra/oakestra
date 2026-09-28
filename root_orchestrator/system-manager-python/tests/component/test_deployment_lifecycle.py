"""One deployment walked through the system manager from start to finish: login, gRPC
cluster registration, application creation, scale-up, the root scheduler's callback, a
cluster status report and application deletion. It catches breakage between the steps
that the per-endpoint tests can't see.
"""

import re

import pytest

from tests.component.conftest import cs2_message

pytestmark = pytest.mark.component

SLA_TWO_SERVICES = {
    "sla_version": "v2.0",
    "customerID": "Admin",
    "applications": [
        {
            "applicationID": "",
            "application_name": "lifecycleapp",
            "application_namespace": "test",
            "microservices": [
                {
                    "microserviceID": "",
                    "microservice_name": "frontend",
                    "microservice_namespace": "test",
                    "virtualization": "container",
                    "memory": 100,
                    "vcpus": 1,
                    "vgpus": 0,
                    "vtpus": 0,
                    "bandwidth_in": 0,
                    "bandwidth_out": 0,
                    "storage": 0,
                    "code": "docker.io/library/nginx",
                    "state": "",
                    "port": "80:80",
                    "cmd": ["ls"],
                    "added_files": [],
                    "constraints": [],
                    "connectivity": [],
                },
                {
                    "microserviceID": "",
                    "microservice_name": "backend",
                    "microservice_namespace": "test",
                    "virtualization": "container",
                    "memory": 100,
                    "vcpus": 1,
                    "vgpus": 0,
                    "vtpus": 0,
                    "bandwidth_in": 0,
                    "bandwidth_out": 0,
                    "storage": 0,
                    "code": "docker.io/library/nginx",
                    "state": "",
                    "port": "80:80",
                    "cmd": ["ls"],
                    "added_files": [],
                    "constraints": [],
                    "connectivity": [],
                },
            ],
        }
    ],
}


# ---------------------------------------------------------------------------- steps


def register_cluster_via_grpc(grpc_stub, cluster):
    response = grpc_stub.handle_init_final(cs2_message(cluster))
    cluster.id = response.id
    return cluster.id


def create_application(client, headers):
    response = client.post("/api/application/", json=SLA_TWO_SERVICES, headers=headers)
    assert response.status_code == 200, response.get_data(as_text=True)
    app = response.get_json()[0]
    assert len(app["microservices"]) == 2
    return app


def scale_up_service(client, headers, service_id):
    response = client.post(f"/api/service/{service_id}/instance", headers=headers)
    assert response.status_code == 200
    assert response.get_json() == {"message": "ok"}


def play_root_scheduler(client, outbound, cluster_id):
    """Stand in for the root_scheduler: read the job it was actually sent and post
    back the scheduling decision the way root_scheduler's callback does."""
    scheduler_calls = outbound.scheduler(method="POST", path="/api/calculate/deploy")
    assert len(scheduler_calls) == 1
    job_id = scheduler_calls[0].json["_id"]

    response = client.post(
        "/api/result/deploy", json={"job_id": job_id, "candidate_id": cluster_id}
    )
    assert response.status_code == 200
    assert response.get_data(as_text=True) == "ok"
    return job_id


def report_instance_running(client, cluster_id, job_id, instance_number):
    response = client.post(
        f"/api/information/{cluster_id}",
        json={
            "cpu_percent": "12",
            "jobs": [
                {
                    "_id": job_id,
                    "status": "RUNNING",
                    "instance_list": [
                        {
                            "instance_number": instance_number,
                            "status": "RUNNING",
                            "cpu_percent": "5",
                            "memory_percent": "5",
                            "publicip": "1.2.3.4",
                            "disk": "0",
                            "logs": "",
                            "worker_id": "worker-1",
                            "host_ip": "10.0.0.50",
                            "host_port": "31000",
                            "last_modified_timestamp": 0,
                        }
                    ],
                }
            ],
        },
    )
    assert response.status_code == 200
    assert response.get_data(as_text=True) == "ok"


def fetch_service(client, headers, job_id):
    response = client.get(f"/api/service/{job_id}", headers=headers)
    assert response.status_code == 200
    return response.get_json()


def delete_application(client, headers, app_id):
    response = client.delete(f"/api/application/{app_id}", headers=headers)
    assert response.status_code == 200
    assert response.get_json() == {"message": "Application Deleted"}


# ---------------------------------------------------------------------------- the test


def test_full_deployment_lifecycle(
    admin_headers, client, grpc_stub, make_cluster, resource_abstractor, outbound
):
    headers = admin_headers

    cluster = make_cluster(registered=False)
    cluster_id = register_cluster_via_grpc(grpc_stub, cluster)

    app = create_application(client, headers)
    frontend_id, backend_id = app["microservices"]
    assert set(resource_abstractor.jobs) == {frontend_id, backend_id}

    # Only one of the two services is scaled up; the other stays untouched.
    scale_up_service(client, headers, frontend_id)
    assert [c.json["_id"] for c in outbound.scheduler()] == [frontend_id]

    job_id = play_root_scheduler(client, outbound, cluster_id)
    assert job_id == frontend_id

    job = resource_abstractor.jobs[frontend_id]
    assert job["status"] == "CLUSTER_SCHEDULED"
    assert len(job["instance_list"]) == 1
    instance = job["instance_list"][0]
    assert instance["cluster_id"] == cluster_id
    instance_number = instance["instance_number"]

    deploy_calls = outbound.cluster(
        cluster, method="POST", path=f"/api/service/{frontend_id}/{instance_number}"
    )
    assert len(deploy_calls) == 1, "cluster manager never received the deploy command"

    report_instance_running(client, cluster_id, frontend_id, instance_number)

    service = fetch_service(client, headers, frontend_id)
    assert service["status"] == "RUNNING"
    assert len(service["instance_list"]) == 1
    reported_instance = service["instance_list"][0]
    assert reported_instance["status"] == "RUNNING"
    assert reported_instance["cluster_id"] == cluster_id
    assert reported_instance["publicip"] == "1.2.3.4"

    # backend was never scaled up, so it never reached the scheduler or the cluster.
    assert backend_id not in [c.json.get("_id") for c in outbound.cluster(cluster) if c.json]

    delete_application(client, headers, app["_id"])

    assert resource_abstractor.apps == {}
    assert resource_abstractor.jobs == {}

    undeploy_calls = outbound.cluster(
        cluster, method="DELETE", path=f"/api/service/{frontend_id}/{instance_number}"
    )
    assert len(undeploy_calls) == 1, (
        "cluster manager was never asked to undeploy the running instance"
    )

    # The instance number sent on instance undeploy is wrong today (strict xfail in
    # test_instances.py), so this test only checks that the call happens.
    instance_undeploy = re.compile(rf"^/api/net/{frontend_id}/-?\d+$")
    net_plugin_call_signatures = [
        (c.method, instance_undeploy.sub(f"/api/net/{frontend_id}/<n>", c.path))
        for c in outbound.net_plugin()
    ]
    assert net_plugin_call_signatures == [
        ("POST", "/api/net/cluster"),
        ("POST", "/api/net/service/deploy"),
        ("POST", "/api/net/service/deploy"),
        ("POST", "/api/net/instance/deploy"),
        ("DELETE", f"/api/net/{frontend_id}/<n>"),
        ("DELETE", f"/api/net/service/{frontend_id}"),
        ("DELETE", f"/api/net/service/{backend_id}"),
    ]
