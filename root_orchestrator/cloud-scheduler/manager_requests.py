import os

import requests
from bson.json_util import dumps

SYSTEM_MANAGER_ADDR = (
    "http://"
    + os.environ.get("SYSTEM_MANAGER_URL")
    + ":"
    + str(os.environ.get("SYSTEM_MANAGER_PORT"))
)


def manager_request(cluster, job_id, job, replicas):
    print("sending scheduling result to system-manager...")
    request_address = SYSTEM_MANAGER_ADDR + "/api/result/deploy"
    print(request_address)
    try:
        requests.post(
            request_address,
            json={"cluster_id": str(cluster.get("_id")), "job_id": job_id},
        )
    except requests.exceptions.RequestException:
        print("Calling System Manager /api/result/deploy not successful.")


def manager_request_replicate(cluster, job_id, job, replicas):
    request_address = SYSTEM_MANAGER_ADDR + "/api/result/replicate"
    try:
        requests.post(
            request_address,
            json=dumps({"cluster": cluster, "job": job, "job_id": job_id, "replicas": replicas}),
        )
    except requests.exceptions.RequestException:
        print("Callig System Manager /api/result/replicate not successful.")
