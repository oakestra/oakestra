import os

import requests

SCHEDULER_ADDR = (
    "http://"
    + os.environ.get("CLUSTER_SCHEDULER_URL")
    + ":"
    + str(os.environ.get("CLUSTER_SCHEDULER_PORT"))
)


def scheduler_request_deploy(job, instance_number):
    print("new job instance: asking scheduler...")
    request_addr = SCHEDULER_ADDR + "/api/calculate/deploy"
    print(job)
    try:
        job["_id"] = job["_id"] + "/" + str(instance_number)
        requests.post(request_addr, json=job)
    except requests.exceptions.RequestException:
        print("Calling scheduler", request_addr, "not successful.")


def scheduler_request_replicate(job, replicas):
    print("replicate ")
    request_addr = SCHEDULER_ADDR + "/api/calculate/replicate"
    try:
        job["_id"] = str(job["_id"])
        requests.post(request_addr, json={"job": job, "replicas": replicas})
    except requests.exceptions.RequestException:
        print("Calling Cloud Scheduler /api/calculate/replicate not successful.")


def scheduler_request_status():
    print("new job: asking scheduler status...")
    request_addr = SCHEDULER_ADDR + "/status"
    print(request_addr)
    try:
        response = requests.get(request_addr)
        return "Scheduler Request successfull.", response.status_code
    except requests.exceptions.RequestException:
        print("Calling Cloud Scheduler /status not successful.")
        return "Scheduler Request failed."
