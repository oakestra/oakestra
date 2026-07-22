import os
import threading

import requests
from clients import job_management, resource_aggregation
from clients.my_prometheus_client import prometheus_set_metrics
from oakestra_logging import get_logger
from oakestra_utils.types.statuses import (
    DeploymentStatus,
    NegativeSchedulingStatus,
    PositiveSchedulingStatus,
    convert_to_status,
)

from ext_requests.scheduler_requests import scheduler_request_deploy

logger = get_logger(__name__)

SYSTEM_MANAGER_ADDR = (
    "http://" + os.environ.get("SYSTEM_MANAGER_URL") + ":" + os.environ.get("SYSTEM_MANAGER_PORT")
)


def send_aggregated_info_to_sm(my_id, time_interval):
    update_logger = logger.bind(
        cluster_id=my_id,
        operation="resource_aggregation",
        aggregation_interval_seconds=time_interval,
    )
    try:
        data = resource_aggregation.aggregate_info(time_interval)
        data.update({"jobs": job_management.aggregate_info(time_interval)})
        update_logger.debug(
            "Sending aggregated cluster information",
            event_name="cluster.resources.send_started",
            job_count=len(data.get("jobs", [])),
            field_count=len(data),
        )
        threading.Thread(group=None, target=send_aggregated_info, args=(my_id, data)).start()
        prometheus_set_metrics(data)
    except Exception:
        update_logger.exception(
            "Failed to prepare aggregated cluster information",
            event_name="cluster.resources.prepare_failed",
        )


def re_deploy_dead_jobs_routine():
    re_deploy_triggers = [
        DeploymentStatus.FAILED,
        DeploymentStatus.DEAD,
        NegativeSchedulingStatus.NO_WORKER_CAPACITY,
    ]
    try:
        jobs = job_management.get_jobs_with_failed_instances()
        if jobs is not None:
            for job in jobs:
                for instance in job.get("instance_list", []):
                    if convert_to_status(instance.get("status")) in re_deploy_triggers:
                        logger.info(
                            "Attempting to redeploy failed instance",
                            event_name="job.instance.redeploy_started",
                            job_id=str(job.get("_id")),
                            instance_number=instance.get("instance_number"),
                        )
                        threading.Thread(
                            group=None,
                            target=trigger_undeploy_and_re_deploy,
                            args=(job, instance),
                        ).start()
    except Exception:
        logger.exception(
            "Failed while scanning jobs for redeployment",
            event_name="jobs.redeploy.scan_failed",
        )


def send_aggregated_info(my_id, data):
    try:
        requests.post(SYSTEM_MANAGER_ADDR + "/api/information/" + str(my_id), json=data)
    except requests.exceptions.RequestException:
        logger.exception(
            "System Manager resource update failed",
            event_name="system_manager.resources.update_failed",
            cluster_id=my_id,
        )


def trigger_undeploy_and_re_deploy(service, instance):
    try:
        job_management.delete_job_instance(
            service.get("_id"), instance.get("instance_number"), erase=False
        )
        job_management.update_status(
            service.get("_id"),
            instance.get("instance_number"),
            PositiveSchedulingStatus.REQUESTED.value,
            status_detail="Waiting for scheduling decision",
        )
        scheduler_request_deploy(service, instance.get("instance_number"))
    except Exception:
        logger.exception(
            "Failed to redeploy job instance",
            event_name="job.instance.redeploy_failed",
            job_id=str(service.get("_id")),
            instance_number=instance.get("instance_number"),
        )


def cloud_request_incr_node(my_id):
    request_addr = SYSTEM_MANAGER_ADDR + "/api/cluster/" + str(my_id) + "/incr_node"
    try:
        requests.get(request_addr)
    except requests.exceptions.RequestException:
        logger.exception(
            "System Manager node-count update failed",
            event_name="system_manager.node_count.update_failed",
            cluster_id=my_id,
        )
