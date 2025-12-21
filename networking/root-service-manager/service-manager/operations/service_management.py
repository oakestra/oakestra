from network.subnetwork_management import *
from interfaces.mongodb_requests import *
from utils.sla_validation import check_valid_sla


@check_valid_sla
def deploy_request(deployment_descriptor=None, _id=None):
    if _id is None:
        return "Invalid _id", 400

    s_ip = [
        {
            "IpType": "RR",
            "Address": new_job_rr_address(deployment_descriptor),
            "Address_v6": new_job_rr_address_v6(deployment_descriptor),
        }
    ]
    job_id = mongo_insert_job(
        {
            "_id": _id,
            "deployment_descriptor": deployment_descriptor,
            "service_ip_list": s_ip,
        }
    )
    return "Instance info added", 200


def remove_service(_id=None):
    if _id is None:
        return "Invalid input parameters", 400

    job = mongo_find_job_by_systemid(_id)

    if job is None:
        return "Invalid input parameters", 400

    instances = job.get("instance_list")

    if instances is not None:
        if len(instances) > 0:
            return "There are services still deployed", 400

    mongo_remove_job(_id)
    return "Job removed successfully", 200
