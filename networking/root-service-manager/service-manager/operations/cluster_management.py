from interfaces import mongodb_requests


def register_cluster(cluster_id=None, cluster_port=None, cluster_address=None):
    if cluster_id is None or cluster_port is None or cluster_address is None:
        return "Invalid input arguments", 400

    mongodb_requests.mongo_cluster_add(
        cluster_id,
        {
            "cluster_port": cluster_port,
            "cluster_address": cluster_address,
            "interests": []
        }
    )
    return "cluster registered", 200
