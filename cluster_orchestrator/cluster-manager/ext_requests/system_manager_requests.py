import logging
import os
import threading
import time
import traceback

import config
import requests
from clients import job_management, resource_aggregation
from clients.my_prometheus_client import prometheus_set_metrics
from oakestra_utils.types.statuses import (
    DeploymentStatus,
    NegativeSchedulingStatus,
    PositiveSchedulingStatus,
    convert_to_status,
)

from ext_requests.scheduler_requests import scheduler_request_deploy

logger = logging.getLogger("cluster_manager")


def _scheme() -> str:
    return "https" if config.mtls_enabled() else "http"


SYSTEM_MANAGER_ADDR = (
    _scheme()
    + "://"
    + os.environ.get("SYSTEM_MANAGER_URL")
    + ":"
    + os.environ.get("SYSTEM_MANAGER_PORT")
)


def _build_session() -> requests.Session:
    session = requests.Session()
    if config.mtls_enabled():
        session.cert = (config.CLUSTER_CERT_FILE, config.CLUSTER_KEY_FILE)
        session.verify = config.root_gateway_verify()
    return session


_session = _build_session()


def send_aggregated_info_to_sm(my_id, running_timeout, node_scheduled_timeout):
    try:
        data = resource_aggregation.aggregate_info()
        data.update(
            {"jobs": job_management.aggregate_info(running_timeout, node_scheduled_timeout)}
        )
        logger.debug("sending aggregated info to system manager: %s", data)
        threading.Thread(group=None, target=send_aggregated_info, args=(my_id, data)).start()
        prometheus_set_metrics(data)
    except Exception as e:
        logger.error(e)
        traceback.print_exc()


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
                        logger.info("FAILED INSTANCE, ATTEMPTING RE-DEPLOY")
                        threading.Thread(
                            group=None,
                            target=trigger_undeploy_and_re_deploy,
                            args=(job, instance),
                        ).start()
    except Exception as e:
        logger.error(e)
        traceback.print_exc()


def send_aggregated_info(my_id, data):
    try:
        resp = _session.post(SYSTEM_MANAGER_ADDR + "/api/information/" + str(my_id), json=data)
    except requests.exceptions.RequestException:
        logger.error("Calling System Manager /api/information not successful.")
        return
    if config.mtls_enabled() and resp.ok:
        _renew_if_root_ca_rotated(resp)


# Minimum time between renewal attempts triggered by a root CA rotation.
_ROTATION_RENEW_RETRY_SECONDS = 300
_last_rotation_renew_attempt = 0.0


def _renew_if_root_ca_rotated(resp):
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

    from blueprints.certificates_blueprints import _renew_cluster_certs_in_band
    from utils.certificates import cert_issued_by

    try:
        with open(config.CLUSTER_CERT_FILE) as f:
            if cert_issued_by(f.read(), root_ca):
                return
    except (OSError, ValueError) as e:
        logger.error("Could not check the cluster certificate against the root CA: %s", e)
        return

    if time.monotonic() - _last_rotation_renew_attempt < _ROTATION_RENEW_RETRY_SECONDS:
        return
    _last_rotation_renew_attempt = time.monotonic()

    logger.warning("Root CA was rotated — renewing cluster certificates and intermediate CA")
    success, message = _renew_cluster_certs_in_band(rotate_intermediate=True)
    if success:
        logger.info("Cluster certificate renewal after root CA rotation succeeded: %s", message)
    else:
        logger.error(
            "Cluster certificate renewal after root CA rotation failed: %s. Retrying in %d s.",
            message,
            _ROTATION_RENEW_RETRY_SECONDS,
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
    except Exception as e:
        logger.error(e)


def cloud_request_incr_node(my_id):
    request_addr = SYSTEM_MANAGER_ADDR + "/api/cluster/" + str(my_id) + "/incr_node"
    try:
        _session.get(request_addr)
    except requests.exceptions.RequestException:
        logger.error("Calling System Manager /api/cluster/../incr_node not successful.")
