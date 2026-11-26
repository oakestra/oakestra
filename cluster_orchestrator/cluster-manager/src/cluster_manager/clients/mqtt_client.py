import json
import logging
import re
from typing import Any

import paho.mqtt.client as paho_mqtt
from paho.mqtt.client import MQTTMessage
from resource_abstractor_client import candidate_operations
from typing_extensions import assert_type

from ..app_config import CONFIG
from ..models.job import Job
from ..models.mqtt import NodeInformationMessage, NodeJobMessage, NodeJobResourceMessage
from .job_management import update_deployed_instance_worker, update_instance_resources

logger = logging.getLogger("cluster_manager")

_mqtt: paho_mqtt.Client | None = None


def ensure_mqtt() -> paho_mqtt.Client:
    if not _mqtt:
        raise RuntimeError("Expected MQTT to be initialized")
    return _mqtt


def handle_connect(_client: Any, _userdata: Any, _flags: Any, _rc: Any) -> None:
    mqtt = ensure_mqtt()

    logger.info("MQTT - Connected to MQTT Broker")
    mqtt.subscribe("nodes/+/information")
    mqtt.subscribe("nodes/+/job")
    mqtt.subscribe("nodes/+/jobs/resources")


def handle_logging(_client: Any, _userdata: Any, level: str, buf: Any) -> None:
    if level == "MQTT_LOG_ERR":
        logger.info(f"Error: {buf}")


# TODO: add type validation for all message types
def handle_mqtt_message(_client: Any, _userdata: Any, message: MQTTMessage):
    payload_bytes: bytes = assert_type(message.payload, bytes)

    topic = message.topic
    payload_str = payload_bytes.decode()
    logger.info("MQTT - Received from worker - %s: %s", topic, payload_str)

    re_nodes_information_topic = re.search("^nodes/.*/information$", topic)
    re_job_deployment_topic = re.search("^nodes/.*/job$", topic)
    re_job_resources_topic = re.search("^nodes/.*/jobs/resources$", topic)

    topic_split = topic.split("/")
    client_id = topic_split[1]
    payload = json.loads(payload_str)

    if re_nodes_information_topic is not None:
        handle_node_information_message(client_id, payload)

    elif re_job_deployment_topic is not None:
        handle_node_job_message(payload)

    elif re_job_resources_topic is not None:
        handle_node_job_resources_message(client_id, payload)


def handle_node_information_message(client_id: str, payload: Any):
    message = NodeInformationMessage.model_validate(payload)

    updated = candidate_operations.update_candidate_information(
        client_id, message.model_dump(by_alias=True, exclude_none=True)
    )
    if updated is None:
        mqtt = ensure_mqtt()
        mqtt.publish(
            "nodes/" + client_id + "/control/error",
            json.dumps({"message": "Node not registered to the cluster"}),
        )


def handle_node_job_message(payload: Any):
    message = NodeJobMessage.model_validate(payload)

    update_deployed_instance_worker(
        message.job_name,
        message.instance_number,
        message.status,
        message.status_detail,
        message.public_ip,
    )


def handle_node_job_resources_message(client_id: str, payload: Any):
    message = NodeJobResourceMessage.model_validate(payload)

    for resources_entry in message.instance_resources:
        if resources_entry.instance_number is None:
            logger.info(
                "Missing instance number in %s",
                resources_entry.model_dump_json(indent=2, by_alias=True),
            )
            continue

        # If unable to update then worker has outdated information
        # and service must be undeployed
        if not update_instance_resources(
            resources_entry.job_name, resources_entry.require_instance_number(), resources_entry
        ):
            mqtt_publish_edge_delete(
                client_id,
                resources_entry.require_job_name(),
                resources_entry.require_instance_number(),
                resources_entry.virtualization,
            )


def initialize_mqtt():
    mqtt = paho_mqtt.Client()

    global _mqtt
    _mqtt = mqtt

    mqtt.on_connect = handle_connect
    mqtt.on_message = handle_mqtt_message
    mqtt.reconnect_delay_set(min_delay=1, max_delay=120)
    mqtt.max_queued_messages_set(1000)
    if CONFIG.mqtt_cert:
        try:
            mqtt.tls_set(
                ca_certs=CONFIG.mqtt_cert + "/ca.crt",
                certfile=CONFIG.mqtt_cert + "/cluster.crt",
                keyfile=CONFIG.mqtt_cert + "/cluster.key",
                keyfile_password=CONFIG.cluster_keyfile_password,
            )
            logger.info("MQTT - TLS configured")
        except FileNotFoundError as e:
            logger.error("MQTT - Unable to load certificate files")
            logger.error(e)

    mqtt.connect(
        CONFIG.mqtt_broker_url.strip("[]"),
        CONFIG.mqtt_broker_port,
        keepalive=5,
    )
    mqtt.loop_start()


def mqtt_publish_edge_deploy(worker_id: str, job: Job, instance_number: int):
    topic = "nodes/" + worker_id + "/control/deploy"

    data: dict[str, Any] = job.model_dump(by_alias=True)
    data["instance_number"] = instance_number

    mqtt = ensure_mqtt()
    mqtt.publish(topic, json.dumps(data))  # MQTT cannot send JSON, dump it to String here


def mqtt_publish_edge_delete(
    worker_id: str, job_name: str, instance_number: int, runtime: str | None = "docker"
):
    topic = "nodes/" + worker_id + "/control/delete"

    data: dict[str, Any] = {
        "job_name": job_name,
        "virtualization": runtime,
        "instance_number": instance_number,
    }

    mqtt = ensure_mqtt()
    mqtt.publish(topic, json.dumps(data))
