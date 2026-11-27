import requests
from oakestra_logging import get_logger

from ..app_config import SERVICE_MANAGER_ADDR
from ..models.job import Job

logger = get_logger(__name__)


def network_notify_deployment(job: Job) -> None:
    try:
        requests.post(
            SERVICE_MANAGER_ADDR + "/api/net/deployment",
            json={"job_name": job.job_name},
        )
    except requests.exceptions.RequestException:
        logger.exception(
            "Service Manager deployment notification failed",
            event_name="network.deployment.notification_failed",
            job_id=job.id,
        )
