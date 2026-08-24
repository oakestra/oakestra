from typing import Optional

from oakestra_utils.types.statuses import Status

from resource_abstractor_client.client_helper import make_request, translate_kwargs
from resource_abstractor_client.openapi.api.jobs import (
    append_job_instance as append_job_instance_op,
)
from resource_abstractor_client.openapi.api.jobs import delete_job as delete_job_op
from resource_abstractor_client.openapi.api.jobs import (
    delete_job_instance as delete_job_instance_op,
)
from resource_abstractor_client.openapi.api.jobs import get_job as get_job_op
from resource_abstractor_client.openapi.api.jobs import get_job_instance as get_job_instance_op
from resource_abstractor_client.openapi.api.jobs import list_jobs as list_jobs_op
from resource_abstractor_client.openapi.api.jobs import patch_job as patch_job_op
from resource_abstractor_client.openapi.api.jobs import patch_job_instance as patch_job_instance_op
from resource_abstractor_client.openapi.api.jobs import upsert_job as upsert_job_op
from resource_abstractor_client.openapi.models.job import Job
from resource_abstractor_client.openapi.models.job_instance import JobInstance
from resource_abstractor_client.openapi.models.job_instance_append import JobInstanceAppend

# Wire query parameter name -> the generated _get_kwargs()'s Python parameter name.
# See client_helper.translate_kwargs.
_LIST_PARAMS = {
    "applicationID": "application_id",
    "job_name": "job_name",
}
_GET_JOB_PARAMS = {"instance_number": "instance_number"}


def get_jobs(**kwargs):
    params = translate_kwargs(kwargs, _LIST_PARAMS)
    return make_request(list_jobs_op._get_kwargs(**params))


def get_jobs_of_application(application_id):
    return get_jobs(applicationID=application_id)


def get_job_by_id(job_id, filter={}):
    params = translate_kwargs(filter, _GET_JOB_PARAMS)
    return make_request(get_job_op._get_kwargs(job_id, **params))


def get_job_instance(job_id, instance_number, filter={}):
    # getJobInstance has no query parameters in the spec - filter is accepted (and
    # ignored) only to keep this function's signature unchanged for existing callers.
    return make_request(get_job_instance_op._get_kwargs(job_id, instance_number))


def append_job_instance(job_id, instance_number, instance_data):
    body = JobInstanceAppend.from_dict(instance_data)
    return make_request(append_job_instance_op._get_kwargs(job_id, instance_number, body=body))


def create_job(data):
    return make_request(upsert_job_op._get_kwargs(body=Job.from_dict(data)))


def update_job(job_id: str, data: dict) -> Optional[dict]:
    return make_request(patch_job_op._get_kwargs(job_id, body=Job.from_dict(data)))


def update_job_status(
    job_id: str,
    status: Status,
    status_detail: str = None,
) -> Optional[dict]:
    data = {"status": status.value}
    if status_detail:
        data["status_detail"] = status_detail
    return update_job(job_id, data)


def update_job_instance(job_id, instance_number, data):
    body = JobInstance.from_dict(data)
    return make_request(patch_job_instance_op._get_kwargs(job_id, instance_number, body=body))


def delete_job_instance(job_id, instance_number):
    return make_request(delete_job_instance_op._get_kwargs(job_id, instance_number))


def delete_job(job_id):
    return make_request(delete_job_op._get_kwargs(job_id))
