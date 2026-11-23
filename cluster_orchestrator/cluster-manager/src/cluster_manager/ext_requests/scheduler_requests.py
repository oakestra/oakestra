import logging

import requests

from ..app_config import SCHEDULER_ADDR
from ..models.job import Job

logger = logging.getLogger("cluster_manager")


def scheduler_request_deploy(job: Job, instance_number: int) -> None:
    try:
        # TODO: weird hack maybe improve API?
        copied_job = job.model_copy(deep=True)
        copied_job.id = job.require_id() + "/" + str(instance_number)
        requests.post(
            SCHEDULER_ADDR + "/api/calculate/deploy", json=copied_job.model_dump(by_alias=True)
        )
    except requests.exceptions.RequestException:
        logger.error("Calling scheduler at /api/calculate/deploy was not successful.")
