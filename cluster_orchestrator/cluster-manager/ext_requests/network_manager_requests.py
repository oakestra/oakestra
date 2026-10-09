import os

import requests
from oakestra_logging import get_logger

logger = get_logger(__name__)

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
        logger.exception(
            "Service Manager deployment notification failed",
            event_name="network.deployment.notification_failed",
            job_id=job_id,
        )


def network_notify_migration(job_id, job):
    pass


def network_notify_undeployment(job_id, job):
    pass
