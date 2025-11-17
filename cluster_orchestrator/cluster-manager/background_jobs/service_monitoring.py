import threading
import traceback
from clients.mongodb_client import (
    mongo_get_services_with_failed_instanes,
    mongo_find_job_by_system_id,
)
from oakestra_utils.types.statuses import (
    DeploymentStatus,
    NegativeSchedulingStatus,
    convert_to_status,
)
from ext_requests.cluster_scheduler_requests import scheduler_request_deploy
import services.service_operations as service_operations
from ext_requests.worker_node_request import deploy_to_worker
from logs import logger


# Background job that looks for services with failed instances
# and triggers a re-deploy
def re_deploy_dead_services_routine():
    re_deploy_triggers = [
        DeploymentStatus.FAILED,
        DeploymentStatus.DEAD,
        NegativeSchedulingStatus.NO_WORKER_CAPACITY,
    ]

    try:
        services = mongo_get_services_with_failed_instanes()
        if services is not None:
            for service in services:
                for instance in service.get("instance_list", []):
                    status = convert_to_status(instance.get("status"))
                    # we trigger a re-deploy only if the instance is not currently deployed
                    #  or its status is in the re_deploy_triggers list
                    if status in re_deploy_triggers or instance.get("node") is None:
                        logger.info(
                            f"Re-deploying service {service.get('system_job_id')} instance "
                            f"{instance.get('instance_number')} with status {status}"
                        )
                        threading.Thread(
                            group=None,
                            target=trigger_undeploy_and_re_deploy,
                            args=(service, instance),
                        ).start()
                    else:
                        # if the instance is still deployed but in UNKNOWN status
                        # we refresh its deployment
                        logger.info(
                            "Re-freshing deployment of service "
                            f"{service.get('system_job_id')} instance "
                            f"{instance.get('instance_number')} with status {status}"
                        )
                        threading.Thread(
                            group=None,
                            target=refresh_deployment,
                            args=(service, instance),
                        ).start()
                            
    except Exception as e:
        logger.error(f"Error while re-deploying dead service: {e}")
        traceback.print_exc()


def trigger_undeploy_and_re_deploy(service, instance):
    try:
        service_operations.delete_service(
            service.get("system_job_id"), instance.get("instance_number"), erase=False
        )
        scheduler_request_deploy(
            service,
            str(service.get("system_job_id")),
            str(instance.get("instance_number")),
        )
    except Exception as e:
        logger.error(f"Error while triggering undeploy and ra-deploy of dead service: {e}")


# Tries to re-deploy the service in the same machine as before.
# The node will discard the re-deploy if the instance is still running.
def refresh_deployment(service, instance):
    try:
        node_id = service.get("node").get("_id")
        job = mongo_find_job_by_system_id(service.get("system_job_id"))
        deploy_to_worker(node_id, job, instance.get("instance_number"))
    except Exception as e:
        logger.error(f"Error while refreshing deployment of service: {e}")