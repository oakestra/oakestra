import os

import requests

SERVICE_MANAGER_ADDR = (
    "http://"
    + os.environ.get("CLUSTER_SERVICE_MANAGER_ADDR")
    + ":"
    + os.environ.get("CLUSTER_SERVICE_MANAGER_PORT")
)


def network_notify_deployment(job_id, job):
    print("Sending network deployment notification to the network component")
    job["_id"] = str(job["_id"])
    try:
        requests.post(
            SERVICE_MANAGER_ADDR + "/api/net/deployment",
            json={"job_name": job["job_name"]},
        )
    except requests.exceptions.RequestException:
        print("Calling Service Manager /api/net/deployment not successful.")


def network_notify_migration(job_id, job):
    pass


def network_notify_undeployment(job_id, job):
    pass


def network_notify_gateway_deploy(gateway_node_data):
    print("Sending gateway registration information to the network component")
    try:
        resp = requests.post(
            SERVICE_MANAGER_ADDR + "/api/net/gateway/deploy", json=gateway_node_data
        )
        return "ok", resp.status_code
    except requests.exceptions.RequestException as e:
        print(e)
        print("Calling Service Manager /api/net/gateway/deploy not successul")
        return "", 500


def network_notify_gateway_service_update(gateway_id, service_info):
    print("Sending gateway service expose information to the network component")
    print("updating gateway for service-manager:", service_info)
    try:
        requests.post(
            SERVICE_MANAGER_ADDR + "/api/net/gateway/{}/service".format(gateway_id),
            json=service_info,
        )
    except requests.exceptions.RequestException:
        print(
            "Calling Service Manager /api/net/gateway/{}/service not successul".format(gateway_id)
        )
