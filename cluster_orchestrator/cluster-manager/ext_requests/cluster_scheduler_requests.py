import os

import requests

CLUSTER_SCHEDULER_ADDR = (
    "http://"
    + os.environ.get("CLUSTER_SCHEDULER_URL")
    + ":"
    + str(os.environ.get("CLUSTER_SCHEDULER_PORT"))
)


def scheduler_request_deploy(job, system_job_id, instance_number):
    print("new job: asking cluster_scheduler...")
    print(job)
    request_address = (
        CLUSTER_SCHEDULER_ADDR + "/api/calculate/deploy/" + system_job_id + "/" + instance_number
    )
    print(request_address)
    job_id = str(job.get("_id"))
    job.__setitem__("_id", job_id)
    job.__setitem__("scheduled_node", str(job.get("scheduled_node")))  # deserialize ObjectIDs

    print(job)
    try:
        requests.post(request_address, json=job)
    except requests.exceptions.RequestException:
        print("Calling Cluster Scheduler /api/calculate/deploy not successful.")


def scheduler_request_replicate(job, replicas):
    print("Asking Cluster Scheduler... to replicate")
    request_address = CLUSTER_SCHEDULER_ADDR + "/api/calculate/replicate"
    try:
        requests.post(request_address, json={job, replicas})
    except requests.exceptions.RequestException:
        print("Calling Cluster Scheduler /api/calculate/replicate not successful.")


def scheduler_request_status():
    print("new job: asking cluster_scheduler status...")
    request_addr = CLUSTER_SCHEDULER_ADDR + "/status"
    print(request_addr)
    try:
        response = requests.get(request_addr)
        return "Scheduler Request successfull.", response.status_code
    except requests.exceptions.RequestException:
        print("Calling Cluster Scheduler /status failed.")
        return "Scheduler Request failed."
