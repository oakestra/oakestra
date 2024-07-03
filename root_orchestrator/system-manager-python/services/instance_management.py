import logging
import threading
from typing import List, Optional

from ext_requests.cluster_requests import cluster_request_to_delete_job, cluster_request_to_deploy
from ext_requests.net_plugin_requests import (
    net_inform_instance_deploy,
    net_inform_instance_undeploy,
)
from ext_requests.scheduler_requests import scheduler_request_deploy
from oakestra_utils.types.statuses import PositiveSchedulingStatus, Status, convert_to_status
from resource_abstractor_client import app_operations, cluster_operations, job_operations


def update_job_status(
    job_id: str,
    status: Status,
    status_detail: str,
    instances: List[dict] = [],
) -> Optional[dict]:
    job = job_operations.get_job_by_id(job_id)

    if job is None:
        return None

    for instance in instances:
        job_operations.update_job_instance(job_id, instance["instance_number"], instance)

    return job_operations.update_job_status(job_id, status, status_detail)


def update_job_status_and_instances(
    job_id: str,
    status: Status,
    next_instance_progressive_number: int,
    instance_list: List[dict],
) -> None:
    logging.info(
        f"Updating Job '{job_id}'s status to '{status}' " "and assigning a cluster for this job..."
    )
    updated_job = job_operations.update_job(
        job_id,
        {
            "status": status.value,
            "next_instance_progressive_number": next_instance_progressive_number,
            "instance_list": instance_list,
        },
    )
    if updated_job is None:
        logging.info(f"Updating job '{job_id}'s status to '{status}' failed")


def request_scale_up_instance(microserviceid: str, username: str) -> None:
    service = job_operations.get_job_by_id(microserviceid)
    if service is None:
        logging.warn(f"Service {microserviceid} not found")
        return

    application = app_operations.get_app_by_id(str(service["applicationID"]), username)
    if application is None:
        logging.warn(f"Application {service['applicationID']} not found")
        return

    if microserviceid in application["microservices"]:
        update_job_status(
            job_id=microserviceid,
            status=PositiveSchedulingStatus.REQUESTED,
            status_detail="Waiting for scheduling decision",
        )
        # Request scheduling
        threading.Thread(
            group=None,
            target=scheduler_request_deploy,
            args=(
                service,
                str(microserviceid),
            ),
        ).start()


def request_scale_down_instance(microserviceid, username, which_one=-1):
    """
    remove the instance <which_one> of a service.
    which_one default value is -1 which means "all instances"
    """
    service = job_operations.get_job_by_id(microserviceid)
    if service is None:
        logging.warn(f"Service {microserviceid} not found")
        return

    application = app_operations.get_app_by_id(str(service["applicationID"]), username)
    if application is None:
        logging.warn(f"Application {service['applicationID']} not found")
        return

    if application is not None:
        if microserviceid in application["microservices"]:
            service = mongo_find_job_by_id(microserviceid)
            instances = service.get("instance_list")
            if len(instances) > 0:
                for instance in instances:
                    if which_one == instance["instance_number"] or which_one == -1:
                        # request undeploy network
                        threading.Thread(
                            group=None,
                            target=net_inform_instance_undeploy,
                            args=(
                                microserviceid,
                                which_one,
                            ),
                        ).start()
                        # request undeploy from cluster
                        threading.Thread(
                            group=None,
                            target=cluster_request_to_delete_job,
                            args=(
                                microserviceid,
                                instance["instance_number"],
                            ),
                        ).start()
                        instances.remove(instance)
                mongo_update_job_status_and_instances(
                    microserviceid,
                    service["status"],
                    service["next_instance_progressive_number"],
                    instances,
                )


def instance_scale_up_scheduled_handler(job_id, cluster_id):
    job = job_operations.get_job_by_id(job_id)
    if job is None:
        return

    cluster = cluster_operations.get_resource_by_id(cluster_id)
    if cluster is None:
        return

    instance_number = job["next_instance_progressive_number"]
    instance_info = {
        "instance_number": instance_number,
        "cluster_id": cluster_id,
        "cluster_location": cluster.get("cluster_location", "location-unknown"),
    }
    instance_list = job["instance_list"]
    instance_list.append(instance_info)

    update_job_status_and_instances(
        job_id=job_id,
        status=PositiveSchedulingStatus.CLUSTER_SCHEDULED,
        next_instance_progressive_number=instance_number + 1,
        instance_list=instance_list,
    )

    # inform network component
    net_inform_instance_deploy(str(job_id), instance_number, cluster_id)

    cluster_request_to_deploy(cluster_id, job_id, instance_number)
