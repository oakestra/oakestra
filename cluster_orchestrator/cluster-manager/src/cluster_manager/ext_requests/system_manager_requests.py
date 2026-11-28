import dataclasses
import threading
import time
from typing import Any

import requests
from oakestra_logging import get_logger
from oakestra_utils.types.statuses import (
    DeploymentStatus,
    NegativeSchedulingStatus,
    PositiveSchedulingStatus,
    convert_to_status,
)

from .. import gateway_config
from ..app_config import CONFIG, SYSTEM_MANAGER_ADDR
from ..clients import job_management, resource_aggregation
from ..clients.my_prometheus_client import prometheus_set_metrics
from ..models.job import Job, JobInstance
from .scheduler_requests import scheduler_request_deploy

logger = get_logger(__name__)


def _system_manager_base() -> str:
    if gateway_config.mtls_enabled():
        return "https://" + CONFIG.system_manager_url + ":" + str(CONFIG.system_manager_port)
    return SYSTEM_MANAGER_ADDR


def _build_session() -> requests.Session:
    session = requests.Session()
    if gateway_config.mtls_enabled():
        session.cert = (gateway_config.CLUSTER_CERT_FILE, gateway_config.CLUSTER_KEY_FILE)
        session.verify = gateway_config.root_gateway_verify()
    return session


SYSTEM_MANAGER_BASE = _system_manager_base()
_session = _build_session()


def _post_to_system_manager(url: str, **kwargs) -> requests.Response:
    """POST to the root: over the mTLS session behind the gateway, plainly otherwise."""
    if gateway_config.mtls_enabled():
        return _session.post(url, **kwargs)
    return requests.post(url, **kwargs)


def send_aggregated_info_to_sm(
    assigned_cluster_id: str, running_timeout: int, node_scheduled_timeout: int
) -> None:
    update_logger = logger.bind(
        cluster_id=assigned_cluster_id,
        operation="resource_aggregation",
        running_timeout_seconds=running_timeout,
        node_scheduled_timeout_seconds=node_scheduled_timeout,
    )
    try:
        metrics = resource_aggregation.compute_aggregated_worker_metrics()

        metrics_msg = dataclasses.asdict(metrics) if metrics is not None else {}
        metrics_msg.update(
            {"jobs": job_management.aggregate_info(running_timeout, node_scheduled_timeout)}
        )

        update_logger.debug(
            "Sending aggregated cluster information",
            event_name="cluster.resources.send_started",
            job_count=len(metrics_msg.get("jobs", [])),
            field_count=len(metrics_msg),
        )
        threading.Thread(
            group=None, target=send_aggregated_info, args=(assigned_cluster_id, metrics_msg)
        ).start()
        prometheus_set_metrics(metrics_msg)
    except Exception:
        update_logger.exception(
            "Failed to prepare aggregated cluster information",
            event_name="cluster.resources.prepare_failed",
        )


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
                    logger.info(
                        "Attempting to redeploy failed instance",
                        event_name="job.instance.redeploy_started",
                        job_id=job.require_id(),
                        instance_number=instance.require_instance_number(),
                    )
                    threading.Thread(
                        group=None,
                        target=trigger_undeploy_and_re_deploy,
                        args=(job, instance),
                    ).start()
    except Exception:
        # TODO: why are we randomly catching all errors here
        logger.exception(
            "Failed while scanning jobs for redeployment",
            event_name="jobs.redeploy.scan_failed",
        )


def send_aggregated_info(assigned_cluster_id: str, data: Any) -> None:
    try:
        resp = _post_to_system_manager(
            SYSTEM_MANAGER_BASE + "/api/information/" + assigned_cluster_id, json=data
        )
    except requests.exceptions.RequestException:
        logger.exception(
            "System Manager resource update failed",
            event_name="system_manager.resources.update_failed",
            cluster_id=assigned_cluster_id,
        )
        return
    if gateway_config.mtls_enabled() and resp.ok:
        _renew_if_root_ca_rotated(resp)


# Minimum time between renewal attempts triggered by a root CA rotation.
_ROTATION_RENEW_RETRY_SECONDS = 300
_last_rotation_renew_attempt = 0.0


def _renew_if_root_ca_rotated(resp: requests.Response) -> None:
    """Renew the client cert if the root reports a CA that did not sign it.

    The root returns its current CA with every /api/information response; after a
    rotation it no longer matches the issuer of this cluster's client cert.
    """
    global _last_rotation_renew_attempt
    try:
        root_ca = resp.json().get("root_ca")
    except (ValueError, AttributeError):
        return
    if not root_ca:
        return

    from ..blueprints.certificates_blueprints import _renew_cluster_certs_in_band
    from ..utils.certificates import cert_issued_by

    try:
        with open(gateway_config.CLUSTER_CERT_FILE) as f:
            if cert_issued_by(f.read(), root_ca):
                return
    except (OSError, ValueError):
        logger.exception(
            "Could not check the cluster certificate against the root CA",
            event_name="certificates.cluster_cert.issuer_check_failed",
        )
        return

    if time.monotonic() - _last_rotation_renew_attempt < _ROTATION_RENEW_RETRY_SECONDS:
        return
    _last_rotation_renew_attempt = time.monotonic()

    logger.warning(
        "Root CA was rotated — renewing cluster certificates and intermediate CA",
        event_name="certificates.root_ca.rotated",
    )
    success, message = _renew_cluster_certs_in_band(rotate_intermediate=True)
    if success:
        logger.info(
            "Cluster certificate renewal after root CA rotation succeeded",
            event_name="certificates.cluster_cert.renewed_after_rotation",
            detail=message,
        )
    else:
        logger.error(
            "Cluster certificate renewal after root CA rotation failed; retrying later",
            event_name="certificates.cluster_cert.renewal_after_rotation_failed",
            detail=message,
            retry_in_seconds=_ROTATION_RENEW_RETRY_SECONDS,
        )


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
    except Exception:
        # TODO: why are we randomly catching all errors here
        logger.exception(
            "Failed to redeploy job instance",
            event_name="job.instance.redeploy_failed",
            job_id=job.require_id(),
            instance_number=instance.require_instance_number(),
        )
