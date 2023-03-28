from network.subnetwork_management import *
from interfaces.mongodb_requests import *

# TODO IPv6
def deploy_request(deployment_descriptor=None, system_job_id=None):
    if deployment_descriptor is None or system_job_id is None:
        return "Invalid input parameters", 400

    s_ip = [{
        "IpType": 'RR',
        "Address": new_job_rr_address(deployment_descriptor),
        "Address_v6": new_job_rr_address_v6(deployment_descriptor)
    }]
    job_id = mongo_insert_job(
        {
            'system_job_id': system_job_id,
            'deployment_descriptor': deployment_descriptor,
            'service_ip_list': s_ip
        })
    return "Instance info added", 200


def remove_service(system_job_id=None):
    if system_job_id is None:
        return "Invalid input parameters", 400

    job = mongo_find_job_by_systemid(system_job_id)

    if job is None:
        return "Invalid input parameters", 400

    instances = job.get("instance_list")

    if instances is not None:
        if len(instances) > 0:
            return "There are services still deployed", 400

    mongo_remove_job(system_job_id)
    return "Job removed successfully", 200
