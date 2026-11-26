import requests
from oakestra_logging import get_logger

from ..app_config import SCHEDULER_ADDR
from ..models.job import Job

logger = get_logger(__name__)


def scheduler_request_deploy(job: Job, instance_number: int) -> None:
    try:
        # TODO: weird hack maybe improve API?
        copied_job = job.model_copy(deep=True)
        copied_job.id = job.require_id() + "/" + str(instance_number)
        requests.post(
            SCHEDULER_ADDR + "/api/calculate/deploy", json=copied_job.model_dump(by_alias=True)
        )
    except requests.exceptions.RequestException:
        logger.exception(
            "Scheduler deployment request failed",
            event_name="scheduler.deploy.request_failed",
            endpoint=SCHEDULER_ADDR + "/api/calculate/deploy",
            instance_number=instance_number,
        )
