import os

import requests
from oakestra_logging import get_logger

logger = get_logger(__name__)

SCHEDULER_ADDR = (
    "http://"
    + os.environ.get("CLUSTER_SCHEDULER_URL")
    + ":"
    + str(os.environ.get("CLUSTER_SCHEDULER_PORT"))
)


def scheduler_request_deploy(job, instance_number):
    request_addr = SCHEDULER_ADDR + "/api/calculate/deploy"
    try:
        job["_id"] = job["_id"] + "/" + str(instance_number)
        requests.post(request_addr, json=job)
    except requests.exceptions.RequestException:
        logger.exception(
            "Scheduler deployment request failed",
            event_name="scheduler.deploy.request_failed",
            endpoint=request_addr,
            instance_number=instance_number,
        )


def scheduler_request_status():
    request_addr = SCHEDULER_ADDR + "/status"
    try:
        response = requests.get(request_addr)
        return "Scheduler Request successfull.", response.status_code
    except requests.exceptions.RequestException:
        logger.exception(
            "Scheduler status request failed",
            event_name="scheduler.status.request_failed",
        )
        return "Scheduler Request failed."
