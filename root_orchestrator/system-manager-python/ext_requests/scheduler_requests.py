import os

import requests

SCHUEDULER_ADDR = (
    "http://"
    + os.environ.get("CLOUD_SCHEDULER_URL", "localhost")
    + ":"
    + str(os.environ.get("CLOUD_SCHEDULER_PORT", "10004"))
)


def scheduler_request_deploy(job, job_id):
    print("new job: asking cloud_scheduler...")
    request_addr = SCHUEDULER_ADDR + "/api/calculate/deploy"
    print(request_addr)
    try:
        job["_id"] = str(job["_id"])
        requests.post(request_addr, json={"job": job, "system_job_id": job_id})
    except requests.exceptions.RequestException:
        print("Calling Cloud Scheduler /api/calculate/deploy not successful.")


def scheduler_request_replicate(job, replicas):
    print("replicate ")
    request_addr = SCHUEDULER_ADDR + "/api/calculate/replicate"
    try:
        job["_id"] = str(job["_id"])
        requests.post(request_addr, json={"job": job, "replicas": replicas})
    except requests.exceptions.RequestException:
        print("Calling Cloud Scheduler /api/calculate/replicate not successful.")


def scheduler_request_status():
    print("new job: asking cloud_scheduler status...")
    request_addr = SCHUEDULER_ADDR + "/status"
    print(request_addr)
    try:
        response = requests.get(request_addr)
        return "Scheduler Request successfull.", response.status_code
    except requests.exceptions.RequestException:
        print("Calling Cloud Scheduler /status not successful.")
        return "Scheduler Request failed."
