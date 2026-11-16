import logging
import traceback

from interfaces import mongodb_requests
from network.subnetwork_management import new_job_rr_address, new_job_rr_address_v6
from utils.sla_validation import check_valid_sla

logger = logging.getLogger("root_service_manager")


@check_valid_sla
def deploy_request(deployment_descriptor=None, _id=None):
    if _id is None:
        return "Invalid _id", 400

    try:
        s_ip = [
            {
                "IpType": "RR",
                "Address": new_job_rr_address(deployment_descriptor),
                "Address_v6": new_job_rr_address_v6(deployment_descriptor),
            }
        ]
        mongodb_requests.mongo_insert_job(
            {
                "_id": _id,
                "deployment_descriptor": deployment_descriptor,
                "service_ip_list": s_ip,
            }
        )
        return "Instance info added", 200
    except Exception as e:
        logger.error(f"Error in deploy_request: {str(e)}")
        logger.error(traceback.format_exc())
        return "Error during deployment", 500


def remove_service(_id=None):
    if _id is None:
        return "Invalid input parameters", 400

    job = mongodb_requests.mongo_find_job_by_systemid(_id)

    if job is None:
        return "Invalid input parameters", 400

    instances = job.get("instance_list")

    if instances is not None:
        if len(instances) > 0:
            return "There are services still deployed", 400

    mongodb_requests.mongo_remove_job(_id)
    return "Job removed successfully", 200
