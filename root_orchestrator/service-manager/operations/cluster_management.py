import logging

from interfaces import mongodb_requests
from network.utils import sanitize

logger = logging.getLogger("root_service_manager")

CLUSTER_STATUS_ACTIVE = "ACTIVE"
CLUSTER_STATUS_ERROR = "ERROR"
CLUSTER_STATUS_OFFLINE = "OFFLINE"


def register_cluster(cluster_port=None, cluster_address=None, cluster_id=None):
    if cluster_port is None or cluster_address is None or cluster_id is None:
        return "Invalid input arguments", 400

    mongodb_requests.mongo_cluster_add(
        cluster_id=cluster_id,
        cluster_port=cluster_port,
        cluster_address=cluster_address,
        status=CLUSTER_STATUS_ACTIVE,
    )
    return "cluster registered", 200


def set_cluster_status(cluster_id, status):
    mongodb_requests.mongo_set_cluster_status(cluster_id, status)


def identify_cluster(cluster_id, advertised_address=None):
    """The registered cluster with this id, or None.

    Routes are pushed to the stored address, so when the cluster advertises a new one
    the stored address follows it.
    """
    if not cluster_id:
        return None
    cluster = mongodb_requests.mongo_get_cluster_by_id(cluster_id)
    if cluster is None:
        return None

    if advertised_address:
        advertised_address = sanitize(advertised_address)
        if advertised_address != cluster.get("cluster_address"):
            logger.info(
                "Cluster %s moved from %s to %s",
                cluster_id,
                cluster.get("cluster_address"),
                advertised_address,
            )
            mongodb_requests.mongo_set_cluster_address(cluster_id, advertised_address)
            cluster["cluster_address"] = advertised_address
    return cluster
