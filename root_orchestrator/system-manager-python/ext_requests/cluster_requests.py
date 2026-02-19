import logging

import requests
from resource_abstractor_client import candidate_operations, job_operations
from services.cluster_management import find_cluster_of_job
from utils.network import sanitize

logger = logging.getLogger("system_manager")


def cluster_request_status(cluster_id):
    cluster = candidate_operations.get_candidate_by_id(cluster_id)
    try:
        cluster_addr = "http://" + cluster.get("ip") + ":" + str(cluster.get("port")) + "/status"
        requests.get(cluster_addr, timeout=5)
    except requests.exceptions.RequestException:
        logger.error("Calling Cluster Orchestrator /status not successful.")


def cluster_request_to_deploy(cluster_id, job_id, instance_number):
    cluster = candidate_operations.get_candidate_by_id(cluster_id)
    if cluster is None:
        logger.error(f"Cluster with {cluster_id} not found.")
        return

    job = job_operations.get_job_instance(job_id, instance_number)
    if job is None:
        logger.error(f"Job with {job_id} not found.")
        return

    try:
        logger.debug(
            f"Preparing deploy request for job {job} instance {instance_number} to cluster {cluster}"
        )
        cluster_addr = (
            "http://"
            + sanitize(cluster.get("ip"), request=True)
            + ":"
            + str(cluster.get("port"))
            + "/api/service/"
            + str(job_id)
            + "/"
            + str(instance_number)
        )
        job["_id"] = str(job["_id"])
        logger.info(f"Deploy request to {cluster_addr}")
        requests.post(cluster_addr, json=job, timeout=10)
    except Exception as e:
        logger.error(f"Calling Cluster Orchestrator {cluster_addr} not successful: {e}")


def cluster_request_to_delete_job(job_id, instance_number):
    cluster = find_cluster_of_job(job_id, int(instance_number))
    if cluster is None:
        logger.error(f"Cluster for job {job_id} not found.")
        return

    try:
        cluster_addr = (
            "http://"
            + sanitize(cluster.get("ip"), request=True)
            + ":"
            + str(cluster.get("port"))
            + "/api/service/"
            + str(job_id)
            + "/"
            + str(instance_number)
        )
        logger.info(f"Delete request to {cluster_addr}")
        requests.delete(cluster_addr, timeout=10)
    except Exception as e:
        logger.error(f"Calling Cluster Orchestrator {cluster_addr} job not successful: {e}")


def cluster_request_to_delete_job_by_ip(job_id, instance_number, ip):
    try:
        cluster = candidate_operations.get_candidate_by_ip(ip)
        if cluster is None:
            logger.error(f"Cluster with {ip} not found")
            return

        cluster_addr = (
            "http://"
            + sanitize(cluster.get("ip"), request=True)
            + ":"
            + str(cluster.get("port"))
            + "/api/service/"
            + str(job_id)
            + "/"
            + str(instance_number)
        )
        logger.info(f"Delete request to {cluster_addr}")
        requests.delete(cluster_addr, timeout=10)
    except Exception as e:
        logger.error(e)
        logger.error(f"Calling Cluster Orchestrator {cluster_addr} job by ip not successful.")


def cluster_request_to_replicate_up(cluster_obj, job_obj, int_replicas):
    cluster_addr = (
        "http://"
        + sanitize(cluster_obj.get("ip"), request=True)
        + ":"
        + str(cluster_obj.get("port"))
        + "/api/replicate/"
    )
    try:
        requests.post(cluster_addr, json={"job": job_obj, "int_replicas": int_replicas}, timeout=10)
        return 1
    except requests.exceptions.RequestException:
        logger.error(f"Calling Cluster Orchestrator {cluster_addr} /api/replicate not successful.")


def cluster_request_to_replicate_down(cluster_obj, job_obj, int_replicas):
    cluster_addr = (
        "http://"
        + sanitize(cluster_obj.get("ip"), request=True)
        + ":"
        + str(cluster_obj.get("port"))
        + "/api/replicate/"
    )
    try:
        requests.post(cluster_addr, json={"job": job_obj, "int_replicas": int_replicas}, timeout=10)
        return 1
    except requests.exceptions.RequestException:
        logger.error(f"Calling Cluster Orchestrator {cluster_addr} /api/replicate not successful.")


def cluster_request_to_move_within_cluster(cluster_obj, job_id, node_from, node_to):
    cluster_addr = (
        "http://"
        + sanitize(cluster_obj.get("ip"), request=True)
        + ":"
        + str(cluster_obj.get("port"))
        + "/api/move/"
    )
    try:
        requests.post(
            cluster_addr,
            json={"job": job_id, "node_from": node_from, "node_to": node_to},
            timeout=10,
        )
        return 1
    except requests.exceptions.RequestException:
        logger.error(f"Calling Cluster Orchestrator {cluster_addr} /api/move not successful.")
