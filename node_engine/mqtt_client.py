import os
import uuid
from flask_mqtt import Mqtt
import json
import re

from cpu_mem import get_cpu_memory, get_memory
from dockerclient import start_container, stop_container
from mirageosclient import run_unikernel_mirageos

mqtt = None
app = None
node_info = {}


def mqtt_init(flask_app, mqtt_port=1883, my_id=None):
    global mqtt
    global app

    app = flask_app

    app.config['MQTT_BROKER_URL'] = os.environ.get('CLUSTER_MANAGER_IP')
    app.config['MQTT_BROKER_PORT'] = int(mqtt_port)
    app.config['MQTT_REFRESH_TIME'] = 3.0  # refresh time in seconds
    mqtt = Mqtt(app)
    app.logger.info('initialized mqtt')
    mqtt.subscribe('nodes/' + my_id + '/control/+')

    @mqtt.on_message()
    def handle_mqtt_message(client, userdata, message):
        data = dict(
            topic=message.topic,
            payload=json.loads(message.payload.decode())
        )
        app.logger.info(data)

        topic = data.get('topic')
        # if topic starts with nodes and ends with controls
        re_nodes_topic_control_deploy = re.search("^nodes/" + my_id + "/control/deploy$", topic)
        re_nodes_topic_control_delete = re.search("^nodes/" + my_id + "/control/delete$", topic)

        payload = data.get('payload')
        image_technology = payload.get('image_runtime')
        image_url = payload.get('image')
        job_name = payload.get('job_name')
        port = payload.get('port')

        if re_nodes_topic_control_deploy is not None:
            app.logger.info("MQTT - Received .../control/deploy command")
            address = None
            if image_technology == 'docker':
                address = start_container(job=payload)
            if image_technology == 'mirage':
                commands = payload.get('commands')
                run_unikernel_mirageos(image_url, job_name, job_name, commands)
            if address is not None:
                publish_deploy_status(my_id, payload.get('_id'), 'DEPLOYED')
            else:
                publish_deploy_status(my_id, payload.get('_id'), 'FAILED')
        elif re_nodes_topic_control_delete is not None:
            app.logger.info('MQTT - Received .../control/delete command')
            if image_technology == 'docker':
                stop_container(job_name)


def publish_cpu_mem(my_id):
    app.logger.info('Publishing CPU+Memory usage... my ID: {0}'.format(my_id))
    cpu_used, free_cores, memory_used, free_memory_in_MB = get_cpu_memory()
    mem_value = get_memory()
    topic = 'nodes/' + my_id + '/information'
    mqtt.publish(topic, json.dumps({'cpu': cpu_used, 'free_cores': free_cores,
                                    'memory': memory_used, 'memory_free_in_MB': free_memory_in_MB}))


def publish_deploy_status(my_id, job_id, status):
    app.logger.info('Publishing Deployment status... my ID: {0}'.format(my_id))
    topic = 'nodes/' + my_id + '/job'
    mqtt.publish(topic, json.dumps({'job_id': job_id, 'status': status}))
