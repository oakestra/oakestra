from requests import delete, get, patch, put
from resource_abstractor_client.client_helper import make_request

JOBS_API = "/api/v1/jobs"


def get_jobs(**kwargs):
    return make_request(get, JOBS_API, params=kwargs)


def get_jobs_of_application(application_id):
    return get_jobs(applicationID=application_id)


def get_job_by_id(job_id, filter={}):
    request_address = f"{JOBS_API}/{job_id}"
    return make_request(get, request_address, params=filter)


def create_job(data):
    return make_request(put, JOBS_API, json=data)


def update_job(job_id, data):
    request_address = f"{JOBS_API}/{job_id}"
    return make_request(patch, request_address, json=data)


def update_job_status(job_id, status, status_detail=None):
    data = {"status": status}
    if status_detail:
        data["status_detail"] = status_detail

    return update_job(job_id, data)


def update_job_instance(job_id, instance_number, data):
    request_address = f"{JOBS_API}/{job_id}/{instance_number}"
    return make_request(patch, request_address, json=data)


def delete_job(job_id):
    request_address = f"{JOBS_API}/{job_id}"
    return make_request(delete, request_address)
