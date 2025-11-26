from clients.mqtt_client import mqtt_publish_edge_delete, mqtt_publish_edge_deploy
from logs import logger
import traceback


def deploy_to_worker(node_id, job, instance_number):
    try:
        mqtt_publish_edge_deploy(node_id, job, instance_number)
    except Exception as e:
        logger.error(f"Error while deploying to worker: {e}")
        logger.error(traceback.format_exc())


def delete_from_worker(node_id, job_name, instance_number, virtualization):
    try:
        mqtt_publish_edge_delete(node_id, job_name, instance_number, virtualization)
    except Exception as e:
        logger.error(f"Error while deleting from worker: {e}")