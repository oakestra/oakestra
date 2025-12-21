import logging
import traceback

from interfaces import mongodb_requests

logger = logging.getLogger("cluster_service_manager")


def init_cluster(cluster_id):
    if cluster_id is None:
        return "Invalid argument", 400

    # table query the root to get the instances
    try:
        mongodb_requests.mongo_update_cluster_info(worker_id=cluster_id)
    except Exception:
        logger.error("Incoming Request /api/net/deployment failed service_resolution")
        logger.debug(traceback.format_exc())
        print(traceback.format_exc())
        return "Service resolution failed", 500

    return "initialized succesfully", 200
