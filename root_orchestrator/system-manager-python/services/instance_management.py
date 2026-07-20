import threading
from typing import List, Optional

from ext_requests.cluster_requests import cluster_request_to_delete_job, cluster_request_to_deploy
from ext_requests.net_plugin_requests import (
    net_inform_instance_deploy,
    net_inform_instance_undeploy,
)
from ext_requests.scheduler_requests import scheduler_request_deploy
from oakestra_logging import get_logger
from oakestra_utils.types.statuses import PositiveSchedulingStatus, Status, convert_to_status
from resource_abstractor_client import app_operations, candidate_operations, job_operations

logger = get_logger(__name__)


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
    logger.info(
        "Updating job status and cluster assignment",
        event_name="job.status.update_started",
        job_id=job_id,
        status=status.value,
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
        logger.error(
            "Failed to update job status and cluster assignment",
            event_name="job.status.update_failed",
            job_id=job_id,
            status=status.value,
        )


def request_scale_up_instance(microserviceid: str, username: str) -> None:
    service = job_operations.get_job_by_id(microserviceid)
    if service is None:
        logger.warning(
            "Service not found", event_name="service.not_found", service_id=microserviceid
        )
        return

    application = app_operations.get_app_by_id(str(service["applicationID"]), username)
    if application is None:
        logger.warning(
            "Application not found",
            event_name="application.not_found",
            application_id=service["applicationID"],
        )
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
        logger.warning(
            "Service not found", event_name="service.not_found", service_id=microserviceid
        )
        return

    application = app_operations.get_app_by_id(str(service["applicationID"]), username)
    if application is None:
        logger.warning(
            "Application not found",
            event_name="application.not_found",
            application_id=service["applicationID"],
        )
        return

    if microserviceid in application["microservices"]:
        instances = service.get("instance_list")

        if len(instances) > 0:
            for instance in instances:
                if which_one == instance["instance_number"] or which_one == -1:
                    net_inform_instance_undeploy(microserviceid, which_one)
                    cluster_request_to_delete_job(microserviceid, instance["instance_number"])
                    instances.remove(instance)

            update_job_status_and_instances(
                microserviceid,
                convert_to_status(service["status"]),
                service["next_instance_progressive_number"],
                instances,
            )


def instance_scale_up_scheduled_handler(job_id, cluster_id):
    job = job_operations.get_job_by_id(job_id)
    if job is None:
        return

    cluster = candidate_operations.get_candidate_by_id(cluster_id)
    if cluster is None:
        return

    instance_number = job["next_instance_progressive_number"]
    instance_info = {
        "instance_number": instance_number,
        "cluster_id": cluster_id,
        "cluster_location": cluster.get("candidate_location", "location-unknown"),
        "status": PositiveSchedulingStatus.CLUSTER_SCHEDULED.value,
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
