import logging
import traceback

from interfaces import mongodb_requests

def init_cluster(cluster_id):
    if cluster_id is None:
        return "Invalid argument", 400

    # table query the root to get the instances
    try:
        mongodb_requests.mongo_update_cluster_info(cluster_id=cluster_id)
    except Exception as e:
        logging.error("Incoming Request /api/net/deployment failed service_resolution")
        logging.debug(traceback.format_exc())
        print(traceback.format_exc())
        return "Service resolution failed", 500

    return "initialized succesfully", 200


