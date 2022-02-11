from interfaces import mongodb_requests

CLUSTER_STATUS_ACTIVE = "ACTIVE"
CLUSTER_STATUS_ERROR = "ERROR"
CLUSTER_STATUS_OFFLINE = "OFFLINE"


def register_cluster(cluster_id=None, cluster_port=None, cluster_address=None):
    if cluster_id is None or cluster_port is None or cluster_address is None:
        return "Invalid input arguments", 400

    mongodb_requests.mongo_cluster_add(
        cluster_id,
        {
            "cluster_port": cluster_port,
            "cluster_address": cluster_address,
            "status": CLUSTER_STATUS_ACTIVE
        }
    )
    return "cluster registered", 200


def set_cluster_status(cluster_id, status):
    mongodb_requests.mongo_set_cluster_status(cluster_id, status)


