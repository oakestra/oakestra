import requests
from oakestra_logging import get_logger
from resource_abstractor_client import candidate_operations, job_operations
from services.cluster_management import find_cluster_of_job
from utils.network import sanitize

logger = get_logger(__name__)


def cluster_request_status(cluster_id):
    request_logger = logger.bind(cluster_id=cluster_id, operation="status")
    cluster = candidate_operations.get_candidate_by_id(cluster_id)
    try:
        cluster_addr = "http://" + cluster.get("ip") + ":" + str(cluster.get("port")) + "/status"
        request_logger = request_logger.bind(cluster_addr=cluster_addr)
        requests.get(cluster_addr, timeout=5)
    except requests.exceptions.RequestException:
        request_logger.exception(
            "Cluster status request failed",
            event_name="cluster.status.request_failed",
        )


def cluster_request_to_deploy(cluster_id, job_id, instance_number):
    request_logger = logger.bind(
        cluster_id=cluster_id,
        job_id=job_id,
        instance_number=instance_number,
        operation="deploy",
    )
    cluster = candidate_operations.get_candidate_by_id(cluster_id)
    if cluster is None:
        request_logger.error("Cluster not found", event_name="cluster.not_found")
        return

    job = job_operations.get_job_instance(job_id, instance_number)
    if job is None:
        request_logger.error("Job not found", event_name="job.not_found")
        return

    try:
        request_logger.debug(
            "Preparing cluster deployment request",
            event_name="cluster.deploy.preparing",
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
        request_logger = request_logger.bind(cluster_addr=cluster_addr)
        job["_id"] = str(job["_id"])
        request_logger.info(
            "Sending cluster deployment request",
            event_name="cluster.deploy.requested",
        )
        requests.post(cluster_addr, json=job, timeout=10)
    except Exception:
        request_logger.exception(
            "Cluster deployment request failed",
            event_name="cluster.deploy.failed",
        )


def cluster_request_to_delete_job(job_id, instance_number):
    request_logger = logger.bind(job_id=job_id, instance_number=instance_number, operation="delete")
    cluster = find_cluster_of_job(job_id, int(instance_number))
    if cluster is None:
        request_logger.error("Cluster for job not found", event_name="job.cluster.not_found")
        return

    request_logger = request_logger.bind(cluster_id=cluster.get("_id"))
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
        request_logger = request_logger.bind(cluster_addr=cluster_addr)
        request_logger.info(
            "Sending cluster deletion request",
            event_name="cluster.delete.requested",
        )
        requests.delete(cluster_addr, timeout=10)
    except Exception:
        request_logger.exception(
            "Cluster deletion request failed",
            event_name="cluster.delete.failed",
        )


def cluster_request_to_delete_job_by_ip(job_id, instance_number, cluster_id):
    request_logger = logger.bind(
        cluster_id=cluster_id,
        job_id=job_id,
        instance_number=instance_number,
        operation="delete",
    )
    try:
        cluster = candidate_operations.get_candidate_by_id(cluster_id)
        if cluster is None:
            request_logger.error("Cluster not found", event_name="cluster.not_found")
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
        request_logger = request_logger.bind(cluster_addr=cluster_addr)
        request_logger.info(
            "Sending deletion request to cluster",
            event_name="cluster.delete.requested",
        )
        requests.delete(cluster_addr, timeout=10)
    except Exception:
        request_logger.exception(
            "Cluster deletion request failed",
            event_name="cluster.delete.failed",
        )


def cluster_request_to_replicate_up(cluster_obj, job_obj, int_replicas):
    cluster_addr = (
        "http://"
        + sanitize(cluster_obj.get("ip"), request=True)
        + ":"
        + str(cluster_obj.get("port"))
        + "/api/replicate/"
    )
    request_logger = logger.bind(
        cluster_id=cluster_obj.get("_id"),
        job_id=job_obj.get("_id"),
        cluster_addr=cluster_addr,
        operation="scale_up",
        replicas=int_replicas,
    )
    try:
        requests.post(cluster_addr, json={"job": job_obj, "int_replicas": int_replicas}, timeout=10)
        return 1
    except requests.exceptions.RequestException:
        request_logger.exception(
            "Cluster replication request failed",
            event_name="cluster.replication.request_failed",
        )


def cluster_request_to_replicate_down(cluster_obj, job_obj, int_replicas):
    cluster_addr = (
        "http://"
        + sanitize(cluster_obj.get("ip"), request=True)
        + ":"
        + str(cluster_obj.get("port"))
        + "/api/replicate/"
    )
    request_logger = logger.bind(
        cluster_id=cluster_obj.get("_id"),
        job_id=job_obj.get("_id"),
        cluster_addr=cluster_addr,
        operation="scale_down",
        replicas=int_replicas,
    )
    try:
        requests.post(cluster_addr, json={"job": job_obj, "int_replicas": int_replicas}, timeout=10)
        return 1
    except requests.exceptions.RequestException:
        request_logger.exception(
            "Cluster replication request failed",
            event_name="cluster.replication.request_failed",
        )


def cluster_request_to_move_within_cluster(cluster_obj, job_id, node_from, node_to):
    cluster_addr = (
        "http://"
        + sanitize(cluster_obj.get("ip"), request=True)
        + ":"
        + str(cluster_obj.get("port"))
        + "/api/move/"
    )
    request_logger = logger.bind(
        cluster_id=cluster_obj.get("_id"),
        job_id=job_id,
        cluster_addr=cluster_addr,
        operation="move",
        node_from=node_from,
        node_to=node_to,
    )
    try:
        requests.post(
            cluster_addr,
            json={"job": job_id, "node_from": node_from, "node_to": node_to},
            timeout=10,
        )
        return 1
    except requests.exceptions.RequestException:
        request_logger.exception(
            "Cluster move request failed",
            event_name="cluster.move.request_failed",
        )
