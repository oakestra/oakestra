import os
import random
import re
import json
from datetime import datetime

from flask_mqtt import Mqtt
from mongodb_client import mongo_find_node_by_id_and_update_cpu_mem, mongo_update_job_deployed, mongo_find_job_by_id, find_all_nodes
from system_manager_requests import system_manager_notify_deployment_status

mqtt = None
app = None

VIVALDI_VECTOR = 'vivaldi_vector'
VIVALDI_HEIGHT = 'vivaldi_height'
VIVALDI_ERROR = 'vivaldi_error'
PUBLIC_IP = 'public_ip'
PRIVATE_IP = 'private_ip'
ROUTER_RTT = 'router_rtt'
NEIGHBORS = 6


def mqtt_init(flask_app):
    global mqtt
    global app
    app = flask_app

    app.config['MQTT_BROKER_URL'] = os.environ.get('MQTT_BROKER_URL')
    app.config['MQTT_BROKER_PORT'] = int(os.environ.get('MQTT_BROKER_PORT'))
    app.config['MQTT_REFRESH_TIME'] = 1.0  # refresh time in seconds
    mqtt = Mqtt(app)

    @mqtt.on_connect()
    def handle_connect(client, userdata, flags, rc):
        app.logger.info("MQTT - Connected to MQTT Broker")
        mqtt.subscribe('nodes/+/information')
        mqtt.subscribe('nodes/+/job')

    @mqtt.on_log()
    def handle_logging(client, userdata, level, buf):
        if level == 'MQTT_LOG_ERR':
            app.logger.info('Error: {}'.format(buf))

    @mqtt.on_message()
    def handle_mqtt_message(client, userdata, message):
        data = dict(
            topic=message.topic,
            payload=message.payload.decode()
        )
        app.logger.info('MQTT - Received from worker: ')
        app.logger.info(data)

        topic = data['topic']

        re_nodes_information_topic = re.search("^nodes/.*/information$", topic)
        re_job_deployment_topic = re.search("^nodes/.*/job$", topic)

        # if topic starts with nodes and ends with information
        if re_nodes_information_topic is not None:
            # print(topic)
            topic_split = topic.split('/')
            client_id = topic_split[1]
            payload = json.loads(data['payload'])
            # print(payload)
            cpu_used = payload.get('cpu')
            mem_used = payload.get('memory')
            cpu_cores_free = payload.get('free_cores')
            memory_free_in_MB = payload.get('memory_free_in_MB')
            lat = payload.get('lat')
            long = payload.get('long')
            public_ip = payload.get(PUBLIC_IP)
            private_ip = payload.get(PRIVATE_IP)
            router_rtt = payload.get(ROUTER_RTT)
            vivaldi_vector = payload.get(VIVALDI_VECTOR)
            vivaldi_height = payload.get(VIVALDI_HEIGHT)
            vivaldi_error = payload.get(VIVALDI_ERROR)
            app.logger.info(f"VIVALDI: {vivaldi_vector}")
            # TODO: Remove later. Currently just required for accuracy evaluation
            netem_delay = payload.get('netem_delay')
            mongo_find_node_by_id_and_update_cpu_mem(client_id, cpu_used, cpu_cores_free, mem_used, memory_free_in_MB,
                                                     lat, long, public_ip, private_ip, router_rtt, vivaldi_vector,
                                                     vivaldi_height, vivaldi_error, netem_delay)

            # Tell the node what other nodes it should ping to update vivaldi coordinates
            nodes_vivaldi_information = []
            nodes = find_all_nodes()
            for node in nodes:
                node_public_ip = node.get(PUBLIC_IP)
                node_private_ip = node.get(PRIVATE_IP)
                node_router_rtt = node.get(ROUTER_RTT)
                node_vector = node.get(VIVALDI_VECTOR)
                node_height = node.get(VIVALDI_HEIGHT)
                node_error = node.get(VIVALDI_ERROR)
                if validate_vivaldi_not_none(node_vector, node_height, node_error):
                    # Case 1: If node is in same network check private_ip to avoid self-ping
                    if public_ip == node_public_ip and private_ip != node_private_ip:
                        nodes_vivaldi_information.append(
                            (node_public_ip, node_private_ip, node_router_rtt, node_vector, node_height, node_error))
                    # Case 2: Node has public_ip so just check that ip to avoid self-ping
                    elif public_ip != node_public_ip:
                        nodes_vivaldi_information.append(
                            (node_public_ip, node_private_ip, node_router_rtt, node_vector, node_height, node_error))

            # Shuffle the vivaldi information array and only pick first two nodes for ping measurements
            random.shuffle(nodes_vivaldi_information)
            mqtt_publish_vivaldi_message(client_id, nodes_vivaldi_information[:NEIGHBORS])


        if re_job_deployment_topic is not None:
            # print(topic)
            topic_split = topic.split('/')
            client_id = topic_split[1]
            payload = json.loads(data['payload'])
            job_id = payload.get('job_id')
            status = payload.get('status')
            NsIp = payload.get('ns_ip')
            deployment_info_from_worker_node(job_id, status, NsIp, client_id)


def mqtt_publish_edge_deploy(worker_id, job):
    topic = 'nodes/' + worker_id + '/control/deploy'
    data = job
    job_id = str(job.get('_id'))  # serialize ObjectId to string
    job.__setitem__('_id', job_id)
    mqtt.publish(topic, json.dumps(data))  # MQTT cannot send JSON, dump it to String here


def mqtt_publish_edge_delete(worker_id, job):
    topic = 'nodes/' + worker_id + '/control/delete'
    data = job
    job_id = str(job.get('_id'))
    job.__setitem__('_id', job_id)
    mqtt.publish(topic, json.dumps(data))


def mqtt_publish_ping_message(worker_id, target_ip):
    app.logger.info('MQTT - Send to worker: ' + worker_id)
    topic = 'nodes/' + worker_id + '/ping'
    target_ip_dict = {'target_ip': target_ip}
    mqtt.publish(topic, json.dumps(target_ip_dict))


def mqtt_publish_vivaldi_message(worker_id, nodes_vivaldi_information):
    app.logger.info('MQTT - Send to worker: ' + worker_id)
    topic = 'nodes/' + worker_id + '/vivaldi'
    vivaldi_info_dict = {'vivaldi_info': nodes_vivaldi_information}
    mqtt.publish(topic, json.dumps(vivaldi_info_dict))


def deployment_info_from_worker_node(job_id, status, NsIp, node_id):
    app.logger.debug('JOB-DEPLOYMENT-UPDATE: sending job info to the root')
    # Update mongo job
    mongo_update_job_deployed(job_id, status, NsIp, node_id)
    job = mongo_find_job_by_id(job_id)
    app.logger.debug(job)
    # Notify System manager
    system_manager_notify_deployment_status(job, node_id)

def validate_vivaldi_not_none(vector, height, error):
    return vector is not None and height is not None and error is not None