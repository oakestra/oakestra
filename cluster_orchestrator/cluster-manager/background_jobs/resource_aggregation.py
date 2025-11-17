import threading
from logs import logger
from clients.mongodb_client import mongo_aggregate_node_information
from ext_requests.system_manager_requests import send_aggregated_info
import traceback
from clients.my_prometheus_client import prometheus_set_metrics


def aggregate_cluster_resources_and_send_to_sm(my_id, time_interval):
    try:
        data = mongo_aggregate_node_information(time_interval)
        threading.Thread(group=None, target=send_aggregated_info, args=(my_id, data)).start()
        prometheus_set_metrics(data)
    except Exception as e:
        logger.error(f"Error in aggregating resources and sending to SM: {e}")
        traceback.print_exc()