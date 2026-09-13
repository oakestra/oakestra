import os

import requests
from oakestra_logging import get_logger

NET_PLUGIN_ADDR = (
    "http://"
    + os.environ.get("NET_PLUGIN_URL", "localhost")
    + ":"
    + str(os.environ.get("NET_PLUGIN_PORT", "10010"))
)

logger = get_logger(__name__)


def net_inform_service_deploy(job, job_id):
    """
    Inform the network plugin about the deploy
    """
    request_addr = NET_PLUGIN_ADDR + "/api/net/service/deploy"
    logger.debug(
        "Informing network plugin about service deployment",
        event_name="network.service.deploy_started",
        job_id=job_id,
        endpoint=request_addr,
    )
    r = requests.post(request_addr, json={"deployment_descriptor": job, "_id": job_id}, timeout=10)
    r.raise_for_status()


def net_inform_service_undeploy(job_id):
    """
    Inform the network plugin about the deploy
    """
    request_addr = NET_PLUGIN_ADDR + "/api/net/service/" + str(job_id)

    try:
        r = requests.delete(request_addr, timeout=10)
        r.raise_for_status()
    except requests.exceptions.RequestException:
        logger.exception(
            "Network-plugin service undeployment request failed",
            event_name="network.service.undeploy_failed",
            job_id=job_id,
            endpoint=request_addr,
        )


def net_inform_instance_deploy(job_id, instance_number, cluster_id):
    """
    Inform the network plugin about the new service's instance scheduled
    """
    request_addr = NET_PLUGIN_ADDR + "/api/net/instance/deploy"
    logger.debug(
        "Informing network plugin about instance deployment",
        event_name="network.instance.deploy_started",
        job_id=job_id,
        instance_number=instance_number,
        cluster_id=cluster_id,
    )

    r = requests.post(
        request_addr,
        json={
            "instance_number": instance_number,
            "cluster_id": cluster_id,
            "_id": job_id,
        },
        timeout=10,
    )
    r.raise_for_status()


def net_inform_instance_undeploy(job_id, instance):
    """
    Inform the network plugin about an undeployed instance
    """
    request_addr = NET_PLUGIN_ADDR + "/api/net/" + str(job_id) + "/" + str(instance)
    try:
        r = requests.delete(request_addr, timeout=10)
        r.raise_for_status()
    except requests.exceptions.RequestException:
        logger.exception(
            "Network-plugin instance undeployment request failed",
            event_name="network.instance.undeploy_failed",
            job_id=job_id,
            instance_number=instance,
        )


def net_register_cluster(cluster_id, cluster_address, cluster_port):
    """
    Inform the network plugin about the new registered cluster
    """
    request_addr = NET_PLUGIN_ADDR + "/api/net/cluster"
    try:
        r = requests.post(
            request_addr,
            json={
                "cluster_id": cluster_id,
                "cluster_address": cluster_address,
                "cluster_port": cluster_port,
            },
            timeout=10,
        )
        r.raise_for_status()
    except requests.exceptions.RequestException:
        logger.exception(
            "Network-plugin cluster registration request failed",
            event_name="network.cluster.register_failed",
            cluster_id=cluster_id,
        )
