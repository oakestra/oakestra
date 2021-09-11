import os
import random
import re
import json
from datetime import datetime

from flask_mqtt import Mqtt
from mongodb_client import mongo_find_node_by_id_and_update_cpu_mem, find_all_nodes


mqtt = None
app = None

VIVALDI_VECTOR = 'vivaldi_vector'
VIVALDI_HEIGHT = 'vivaldi_height'
PUBLIC_IP = 'public_ip'


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
        # if topic starts with nodes and ends with information
        re_nodes_information_topic = re.search("^nodes/.*/information$", topic)
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
            rtt = payload.get('rtt')
            public_ip = payload.get(PUBLIC_IP)
            vivaldi_vector = payload.get(VIVALDI_VECTOR)
            vivaldi_height = payload.get(VIVALDI_HEIGHT)
            mongo_find_node_by_id_and_update_cpu_mem(client_id, cpu_used, cpu_cores_free, mem_used, memory_free_in_MB,
                                                     lat, long, rtt, public_ip, vivaldi_vector, vivaldi_height)

            # Send ack to publisher for latency measurement
            request_time = payload.get('request_time')
            mqtt_publish_ack_message(client_id, request_time)

            # Tell node what nodes it should ping to update vivaldi coordinates
            # TODO: for now just send every other nodes id
            nodes_vivaldi_information = []
            nodes = find_all_nodes()
            for node in nodes:
                node_ip = node.get(PUBLIC_IP)
                vector = node.get(VIVALDI_VECTOR)
                height = node.get(VIVALDI_HEIGHT)
                if node_ip != public_ip:
                    nodes_vivaldi_information.append([node_ip, vector, height])
            # Shuffle the vivaldi information array and only pick first two nodes for ping measurements
            random.shuffle(nodes_vivaldi_information)
            if len(nodes_vivaldi_information) >= 2:
                mqtt_publish_vivaldi_message(client_id, nodes_vivaldi_information[:2])
            else:
                mqtt_publish_vivaldi_message(client_id, nodes_vivaldi_information)


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


def mqtt_publish_ack_message(worker_id, request_time):
    app.logger.info('MQTT - Send to worker: ' + worker_id)
    topic = 'nodes/' + worker_id + '/ack'
    request_dict = {'request_time': request_time}
    mqtt.publish(topic, json.dumps(request_dict))


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
