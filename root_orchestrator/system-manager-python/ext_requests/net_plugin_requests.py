import logging
import os

import requests

NET_PLUGIN_ADDR = (
    "http://"
    + os.environ.get("NET_PLUGIN_URL", "localhost")
    + ":"
    + str(os.environ.get("NET_PLUGIN_PORT", "10010"))
)

logger = logging.getLogger("system_manager")


def net_inform_service_deploy(job, job_id):
    """
    Inform the network plugin about the deploy
    """
    logger.debug("new job: communicating service deploy to netowkr plugin...")
    logger.debug(job)
    request_addr = NET_PLUGIN_ADDR + "/api/net/service/deploy"
    logger.debug(request_addr)

    logger.debug(f"Inform service deploy with id {job_id}")
    r = requests.post(request_addr, json={"deployment_descriptor": job, "_id": job_id}, timeout=10)
    r.raise_for_status()


def net_inform_service_undeploy(job_id):
    """
    Inform the network plugin about the deploy
    """
    logger.debug("delete job: communicating service undeploy to netowork plugin...")
    request_addr = NET_PLUGIN_ADDR + "/api/net/service/" + str(job_id)
    logger.debug(request_addr)

    try:
        r = requests.delete(request_addr, timeout=10)
        r.raise_for_status()
    except requests.exceptions.ConnectionError:
        logger.error("Calling network plugin " + request_addr + " Connection error.")
    except requests.exceptions.Timeout:
        logger.error("Calling network plugin " + request_addr + " Timeout error.")
    except requests.exceptions.RequestException:
        logger.error("Calling network plugin " + request_addr + " Request Exception.")


def net_inform_instance_deploy(job_id, instance_number, cluster_id):
    """
    Inform the network plugin about the new service's instance scheduled
    """
    logger.debug("new job: communicating instance deploy to network plugin...")
    request_addr = NET_PLUGIN_ADDR + "/api/net/instance/deploy"
    logger.debug(request_addr)

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
    logger.debug("new job: communicating instance undeploy to network plugin...")
    request_addr = NET_PLUGIN_ADDR + "/api/net/" + str(job_id) + "/" + str(instance)
    logger.debug(request_addr)
    try:
        r = requests.delete(request_addr, timeout=10)
        r.raise_for_status()
    except requests.exceptions.ConnectionError:
        logger.error("Calling network plugin " + request_addr + " Connection error.")
    except requests.exceptions.Timeout:
        logger.error("Calling network plugin " + request_addr + " Timeout error.")
    except requests.exceptions.RequestException:
        logger.error("Calling network plugin " + request_addr + " Request Exception.")


def net_register_cluster(cluster_id, cluster_address, cluster_port):
    """
    Inform the network plugin about the new registered cluster
    """
    logger.debug("new job: communicating cluster registration to net component...")
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
        logger.debug(r)
        r.raise_for_status()
    except requests.exceptions.ConnectionError:
        logger.error("Calling network plugin " + request_addr + " Connection error.")
    except requests.exceptions.Timeout:
        logger.error("Calling network plugin " + request_addr + " Timeout error.")
    except requests.exceptions.RequestException:
        logger.error("Calling network plugin " + request_addr + " Request Exception.")
