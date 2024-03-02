import os

from requests import delete, exceptions, get, patch, put

RESOURCE_ABSTRACTOR_ADDR = (
    f"http://{os.environ.get('RESOURCE_ABSTRACTOR_URL')}:"
    f"{os.environ.get('RESOURCE_ABSTRACTOR_PORT')}"
)
JOBS_API = f"{RESOURCE_ABSTRACTOR_ADDR}/api/v1/jobs"


def get_jobs(**kwargs):
    request_address = JOBS_API
    try:
        response = get(request_address, params=kwargs)
        return response.json()
    except exceptions.RequestException:
        print("Calling Resource Abstractor /api/v1/jobs not successful.")

    return []


def get_job_by_id(job_id, filter={}):
    request_address = f"{JOBS_API}/{job_id}"
    try:
        response = get(request_address, params=filter)
        # TODO check body not empty.
        return response.json()
    except exceptions.RequestException:
        print(f"Calling Resource Abstractor /api/v1/jobs/{job_id} not successful.")

    return None


def create_job(data):
    request_address = JOBS_API
    try:
        response = put(request_address, json=data)
        return response.json()
    except exceptions.RequestException:
        print("Calling Resource Abstractor /api/v1/jobs not successful.")

    return None


def update_job(job_id, data):
    request_address = f"{JOBS_API}/{job_id}"
    try:
        response = patch(request_address, json=data)
        return response.json()
    except exceptions.RequestException:
        print(f"Calling Resource Abstractor /api/v1/jobs/{job_id} not successful.")

    return None


def update_job_status(job_id, status, status_detail=None):
    data = {"status": status}
    if status_detail:
        data["status_detail"] = status_detail

    return update_job(job_id, data)

    return None


def update_job_instance(job_id, instance_number, data):
    request_address = f"{JOBS_API}/{job_id}/{instance_number}"
    try:
        response = patch(request_address, json=data)
        return response.json()
    except exceptions.RequestException:
        print(
            f"Calling Resource Abstractor /api/v1/jobs/{job_id}/{instance_number} not successful."
        )

    return None


def delete_job(job_id):
    request_address = f"{JOBS_API}/{job_id}"
    try:
        response = delete(request_address)
        return response.json()
    except exceptions.RequestException:
        print(f"Calling Resource Abstractor /api/v1/jobs/{job_id} not successful.")

    return None
