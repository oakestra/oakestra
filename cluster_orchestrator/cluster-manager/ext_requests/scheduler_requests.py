import logging
import os

import requests

logger = logging.getLogger("cluster_manager")

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
        logger.error("Calling scheduler", request_addr, "not successful.")


def scheduler_request_replicate(job, replicas):
    request_addr = SCHEDULER_ADDR + "/api/calculate/replicate"
    try:
        job["_id"] = str(job["_id"])
        requests.post(request_addr, json={"job": job, "replicas": replicas})
    except requests.exceptions.RequestException:
        logger.error("Calling Cloud Scheduler /api/calculate/replicate not successful.")


def scheduler_request_status():
    request_addr = SCHEDULER_ADDR + "/status"
    try:
        response = requests.get(request_addr)
        return "Scheduler Request successfull.", response.status_code
    except requests.exceptions.RequestException:
        logger.error("Calling Cloud Scheduler /status not successful.")
        return "Scheduler Request failed."
