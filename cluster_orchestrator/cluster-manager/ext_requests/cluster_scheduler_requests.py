import os

import requests

from logs import logger

CLUSTER_SCHEDULER_ADDR = (
    "http://"
    + os.environ.get("CLUSTER_SCHEDULER_URL")
    + ":"
    + str(os.environ.get("CLUSTER_SCHEDULER_PORT"))
)


def scheduler_request_deploy(job, system_job_id, instance_number):
    logger.debug("new job: asking cluster_scheduler...")
    logger.debug(job)
    request_address = (
        CLUSTER_SCHEDULER_ADDR + "/api/calculate/deploy/" + system_job_id + "/" + instance_number
    )
    logger.debug(request_address)
    job_id = str(job.get("_id"))
    job.__setitem__("_id", job_id)
    job.__setitem__("scheduled_node", str(job.get("scheduled_node")))  # deserialize ObjectIDs

    logger.debug(job)
    try:
        requests.post(request_address, json=job)
    except requests.exceptions.RequestException as e:
        logger.error(f"Calling Cluster Scheduler /api/calculate/deploy not successful. Error: {e}")


def scheduler_request_replicate(job, replicas):
    logger.debug("Asking Cluster Scheduler... to replicate")
    request_address = CLUSTER_SCHEDULER_ADDR + "/api/calculate/replicate"
    try:
        requests.post(request_address, json={job, replicas})
    except requests.exceptions.RequestException as e:
        logger.error(f"Calling Scheduler /api/calculate/replicate not successful. Error: {e}")


def scheduler_request_status():
    logger.debug("new job: asking cluster_scheduler status...")
    request_addr = CLUSTER_SCHEDULER_ADDR + "/status"
    logger.debug(request_addr)
    try:
        response = requests.get(request_addr)
        return "Scheduler Request successfull.", response.status_code
    except requests.exceptions.RequestException as e:
        logger.error(f"Calling Cluster Scheduler /status failed. Error: {e}")
        return "Scheduler Request failed."
