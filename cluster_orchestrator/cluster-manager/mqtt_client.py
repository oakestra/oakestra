import json
import os
import re

import paho.mqtt.client as paho_mqtt
from mongodb_client import (
    mongo_find_node_by_id_and_update_cpu_mem,
    mongo_update_job_deployed,
    mongo_update_service_resources,
)

mqtt = None
app = None


def handle_connect(client, userdata, flags, rc):
    app.logger.info("MQTT - Connected to MQTT Broker")
    mqtt.subscribe("nodes/+/information")
    mqtt.subscribe("nodes/+/job")
    mqtt.subscribe("nodes/+/jobs/resources")


def handle_logging(client, userdata, level, buf):
    if level == "MQTT_LOG_ERR":
        app.logger.info("Error: {}".format(buf))


def handle_mqtt_message(client, userdata, message):
    data = dict(topic=message.topic, payload=message.payload.decode())
    app.logger.info("MQTT - Received from worker: ")
    app.logger.info(data)

    topic = data["topic"]

    re_nodes_information_topic = re.search("^nodes/.*/information$", topic)
    re_job_deployment_topic = re.search("^nodes/.*/job$", topic)
    re_job_resources_topic = re.search("^nodes/.*/jobs/resources$", topic)

    topic_split = topic.split("/")
    client_id = topic_split[1]
    payload = json.loads(data["payload"])

    # if topic starts with nodes and ends with information
    if re_nodes_information_topic is not None:
        mongo_find_node_by_id_and_update_cpu_mem(client_id, payload)
    if re_job_deployment_topic is not None:
        sname = payload.get("sname")
        status = payload.get("status")
        instance = payload.get("instance")
        publicip = payload.get("publicip", "--")
        mongo_update_job_deployed(sname, instance, status, publicip, client_id)
    if re_job_resources_topic is not None:
        services = payload.get("services")
        for service in services:
            try:
                # If unable to update then worker has outdated information
                # and service must be undeployed
                if (
                    mongo_update_service_resources(
                        service.get("job_name"),
                        service,
                        client_id,
                        service.get("instance"),
                    )
                    is None
                ):
                    mqtt_publish_edge_delete(
                        client_id,
                        service.get("job_name"),
                        service.get("instance"),
                        service.get("virtualization"),
                    )
            except Exception as e:
                app.logger.error("MQTT - unable to update service resources")
                app.logger.error(e)


def mqtt_init(flask_app):
    global mqtt
    global app
    app = flask_app
    mqtt = paho_mqtt.Client()
    mqtt.on_connect = handle_connect
    mqtt.on_message = handle_mqtt_message
    mqtt.reconnect_delay_set(min_delay=1, max_delay=120)
    mqtt.max_queued_messages_set(1000)
    mqtt.connect(
        os.environ.get("MQTT_BROKER_URL"),
        int(os.environ.get("MQTT_BROKER_PORT")),
        keepalive=5,
    )
    mqtt.loop_start()


def mqtt_publish_edge_deploy(worker_id, job, instance_number):
    topic = "nodes/" + worker_id + "/control/deploy"
    data = job
    data["instance_number"] = int(instance_number)
    job_id = str(job.get("_id"))  # serialize ObjectId to string
    job.__setitem__("_id", job_id)
    mqtt.publish(topic, json.dumps(data))  # MQTT cannot send JSON, dump it to String here


def mqtt_publish_edge_delete(worker_id, job_name, instance_number, runtime="docker"):
    topic = "nodes/" + worker_id + "/control/delete"
    data = {
        "job_name": job_name,
        "virtualization": runtime,
        "instance_number": int(instance_number),
    }
    mqtt.publish(topic, json.dumps(data))
