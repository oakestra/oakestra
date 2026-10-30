import logging
import os

import requests

logger = logging.getLogger("cluster_manager")

SERVICE_MANAGER_ADDR = (
    "http://"
    + os.environ.get("CLUSTER_SERVICE_MANAGER_ADDR")
    + ":"
    + os.environ.get("CLUSTER_SERVICE_MANAGER_PORT")
)


def network_notify_deployment(job_id, job):
    job["_id"] = str(job["_id"])
    try:
        requests.post(
            SERVICE_MANAGER_ADDR + "/api/net/deployment",
            json={"job_name": job["job_name"]},
        )
    except requests.exceptions.RequestException:
        logger.error("Calling Service Manager /api/net/deployment not successful.")


def network_notify_cluster_id(cluster_id):
    try:
        requests.post(
            SERVICE_MANAGER_ADDR + "/api/net/cluster/id",
            json={"cluster_id": cluster_id},
        )
    except requests.exceptions.RequestException:
        logger.error("Calling Service Manager /api/net/cluster/id not successful.")


def network_notify_migration(job_id, job):
    pass


def network_notify_undeployment(job_id, job):
    pass
