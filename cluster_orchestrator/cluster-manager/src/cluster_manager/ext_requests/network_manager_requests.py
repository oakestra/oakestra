import logging

import requests

from ..app_config import SERVICE_MANAGER_ADDR
from ..models.job import Job

logger = logging.getLogger("cluster_manager")


def network_notify_deployment(job: Job) -> None:
    try:
        requests.post(
            SERVICE_MANAGER_ADDR + "/api/net/deployment",
            json={"job_name": job.job_name},
        )
    except requests.exceptions.RequestException:
        logger.error("Calling Service Manager /api/net/deployment not successful.")
