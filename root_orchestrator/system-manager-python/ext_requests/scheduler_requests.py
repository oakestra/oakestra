import os

import requests
from oakestra_logging import get_logger

SCHUEDULER_ADDR = (
    "http://"
    + os.environ.get("ROOT_SCHEDULER_URL", "localhost")
    + ":"
    + str(os.environ.get("ROOT_SCHEDULER_PORT", "10004"))
)

logger = get_logger(__name__)


def scheduler_request_deploy(job, job_id):
    request_addr = SCHUEDULER_ADDR + "/api/calculate/deploy"
    try:
        job["_id"] = str(job["_id"])
        requests.post(request_addr, json=job, timeout=10)
    except requests.exceptions.RequestException:
        logger.exception(
            "Scheduler deployment request failed",
            event_name="scheduler.deploy.request_failed",
            job_id=job_id,
        )


def scheduler_request_replicate(job, replicas):
    request_addr = SCHUEDULER_ADDR + "/api/calculate/replicate"
    try:
        job["_id"] = str(job["_id"])
        requests.post(request_addr, json={"job": job, "replicas": replicas}, timeout=10)
    except requests.exceptions.RequestException:
        logger.exception(
            "Scheduler replication request failed",
            event_name="scheduler.replicate.request_failed",
            replica_count=replicas,
        )


def scheduler_request_status():
    request_addr = SCHUEDULER_ADDR + "/status"
    try:
        response = requests.get(request_addr, timeout=5)
        return "Scheduler Request successfull.", response.status_code
    except requests.exceptions.RequestException:
        logger.exception(
            "Scheduler status request failed",
            event_name="scheduler.status.request_failed",
        )
        return "Scheduler Request failed."
