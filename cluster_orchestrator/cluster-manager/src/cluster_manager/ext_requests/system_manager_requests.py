import dataclasses
import logging
import threading
import traceback
from typing import Any

import requests
from oakestra_utils.types.statuses import (
    DeploymentStatus,
    NegativeSchedulingStatus,
    PositiveSchedulingStatus,
    convert_to_status,
)

from ..app_config import SYSTEM_MANAGER_ADDR
from ..clients import job_management, resource_aggregation
from ..clients.my_prometheus_client import prometheus_set_metrics
from ..models.job import Job, JobInstance
from .scheduler_requests import scheduler_request_deploy

logger = logging.getLogger("cluster_manager")


def send_aggregated_info_to_sm(
    assigned_cluster_id: str, running_timeout: int, node_scheduled_timeout: int
) -> None:
    try:
        metrics = resource_aggregation.compute_aggregated_worker_metrics()

        metrics_msg = dataclasses.asdict(metrics) if metrics is not None else {}
        metrics_msg.update(
            {"jobs": job_management.aggregate_info(running_timeout, node_scheduled_timeout)}
        )

        logger.debug("sending aggregated info to system manager")
        threading.Thread(
            group=None, target=send_aggregated_info, args=(assigned_cluster_id, metrics_msg)
        ).start()
        prometheus_set_metrics(metrics_msg)
    except Exception as e:
        logger.error(e)
        traceback.print_exc()


def re_deploy_dead_jobs_routine() -> None:
    re_deploy_triggers = [
        DeploymentStatus.FAILED,
        DeploymentStatus.DEAD,
        NegativeSchedulingStatus.NO_WORKER_CAPACITY,
    ]
    try:
        jobs = job_management.get_jobs_with_failed_instances()
        for job in jobs:
            instances = job.instance_list if job.instance_list else []
            for instance in instances:
                if convert_to_status(instance.status) in re_deploy_triggers:
                    logger.info("FAILED INSTANCE, ATTEMPTING RE-DEPLOY")
                    threading.Thread(
                        group=None,
                        target=trigger_undeploy_and_re_deploy,
                        args=(job, instance),
                    ).start()
    except Exception as e:
        # TODO: why are we randomly catching all errors here
        logger.error(e)
        traceback.print_exc()


def send_aggregated_info(assigned_cluster_id: str, data: Any) -> None:
    try:
        requests.post(SYSTEM_MANAGER_ADDR + "/api/information/" + assigned_cluster_id, json=data)
    except requests.exceptions.RequestException:
        logger.error("Calling System Manager /api/information not successful.")


def trigger_undeploy_and_re_deploy(job: Job, instance: JobInstance):
    try:
        job_management.delete_job_instance(
            job.require_id(), instance.require_instance_number(), erase=False
        )
        job_management.update_status(
            job.require_id(),
            instance.require_instance_number(),
            PositiveSchedulingStatus.REQUESTED.value,
            status_detail="Waiting for scheduling decision",
        )
        scheduler_request_deploy(job, instance.require_instance_number())
    except Exception as e:
        # TODO: why are we randomly catching all errors here
        logger.error(e)
