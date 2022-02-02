import os
import random
import re
import json

from flask_mqtt import Mqtt

from NetworkCoordinateSystem.network_measurements import ping
from NetworkCoordinateSystem.vivaldi_coordinate import VivaldiCoordinate
from mongodb_client import mongo_find_node_by_id_and_update_cpu_mem, mongo_update_job_deployed, mongo_find_job_by_id, find_all_nodes, mongo_upsert_job
from system_manager_requests import system_manager_notify_deployment_status
from cluster_scheduler_requests import scheduler_request_alarm

mqtt = None
app = None
co_public_ip = None
co_private_ip = None
co_router_rtt = None
co_vivaldi_coordinate = None

VIVALDI_VECTOR = 'vivaldi_vector'
VIVALDI_HEIGHT = 'vivaldi_height'
VIVALDI_ERROR = 'vivaldi_error'
PUBLIC_IP = 'public_ip'
PRIVATE_IP = 'private_ip'
ROUTER_RTT = 'router_rtt'
NEIGHBORS = 20
VIVALDI_DIM = 2

def mqtt_init(flask_app):
    global mqtt
    global app
    global co_public_ip
    global co_private_ip
    global co_router_rtt
    global co_vivaldi_coordinate

    app = flask_app
    co_vivaldi_coordinate = VivaldiCoordinate(VIVALDI_DIM)
    co_public_ip = os.environ.get('CLUSTER_PUBLIC_IP')
    co_private_ip = os.environ.get('CLUSTER_PRIVATE_IP')
    co_router_rtt = ping(co_public_ip)

    app.config['MQTT_BROKER_URL'] = os.environ.get('MQTT_BROKER_URL')
    app.config['MQTT_BROKER_PORT'] = int(os.environ.get('MQTT_BROKER_PORT'))
    app.config['MQTT_REFRESH_TIME'] = 1.0  # refresh time in seconds
    mqtt = Mqtt(app)

    @mqtt.on_connect()
    def handle_connect(client, userdata, flags, rc):
        app.logger.info("MQTT - Connected to MQTT Broker")
        mqtt.subscribe('nodes/+/information')
        mqtt.subscribe('nodes/+/job')
        mqtt.subscribe('nodes/+/alarm')
        mqtt.subscribe('nodes/+/test')

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
        app.logger.info(f"TOPIC: {topic}")

        re_nodes_information_topic = re.search("^nodes/.*/information$", topic)
        re_job_deployment_topic = re.search("^nodes/.*/job$", topic)
        re_nodes_alarm_topic = re.search("^nodes/.*/alarm$", topic)
        re_nodes_test_topic = re.search("^nodes/.*/test$", topic)

        # if topic starts with nodes and ends with information
        if re_nodes_information_topic is not None:
            handle_node_information_topic(data)
        # if topic starts with nodes and ends with information
        if re_nodes_test_topic is not None:
            handle_node_test_topic(data)
        # if topic starts with nodes and ends with job
        if re_job_deployment_topic is not None:
            handle_job_deployment_topic(data)
        # if topic starts with nodes and ends with alarm
        if re_nodes_alarm_topic is not None:
            handle_nodes_alarm_topic(data)

def handle_node_test_topic(data):
    topic = data['topic']
    # print(topic)
    topic_split = topic.split('/')
    client_id = topic_split[1]
    payload = json.loads(data['payload'])
    # print(payload)
    test = payload.get('test')
    app.logger.info(f"Received {test}")


def mqtt_publish_edge_deploy(worker_id, job):
    topic = 'nodes/' + worker_id + '/control/deploy'
    data = {'job': job}
    job_id = str(job.get('_id'))  # serialize ObjectId to string
    job.__setitem__('_id', job_id)
    mqtt.publish(topic, json.dumps(data))  # MQTT cannot send JSON, dump it to String here


def mqtt_publish_edge_delete(worker_id, job):
    topic = 'nodes/' + worker_id + '/control/delete'
    data = {'job': job}
    job_id = str(job.get('_id'))
    app.logger.info(f"Send delete command for job {job_id} to worker {worker_id}")
    job.__setitem__('_id', job_id)
    mqtt.publish(topic, json.dumps(data))


def mqtt_publish_ping_message(worker_id, target_ips):
    app.logger.info('MQTT - Send to worker: ' + worker_id)
    topic = 'nodes/' + worker_id + '/ping'
    target_ip_dict = {'target_ips': target_ips}
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


def handle_node_information_topic(data):
    topic = data['topic']
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
    worker_public_ip = payload.get(PUBLIC_IP)
    worker_private_ip = payload.get(PRIVATE_IP)
    worker_router_rtt = payload.get(ROUTER_RTT)
    worker_vivaldi_vector = payload.get(VIVALDI_VECTOR)
    worker_vivaldi_height = payload.get(VIVALDI_HEIGHT)
    worker_vivaldi_error = payload.get(VIVALDI_ERROR)
    app.logger.info(f"VIVALDI: {worker_vivaldi_vector}")
    mongo_find_node_by_id_and_update_cpu_mem(client_id, cpu_used, cpu_cores_free, mem_used, memory_free_in_MB,
                                             lat, long, worker_public_ip, worker_private_ip, worker_router_rtt,
                                             worker_vivaldi_vector, worker_vivaldi_height, worker_vivaldi_error)

    publish_vivaldi_message(client_id, worker_public_ip, worker_private_ip)



def handle_job_deployment_topic(data):
    topic = data['topic']
    # print(topic)
    topic_split = topic.split('/')
    client_id = topic_split[1]
    payload = json.loads(data['payload'])
    job_id = payload.get('job_id')
    status = payload.get('status')
    NsIp = payload.get('ns_ip')
    deployment_info_from_worker_node(job_id, status, NsIp, client_id)


def handle_nodes_alarm_topic(data):
    topic = data["topic"]
    client_id = topic.split("/")[1]
    payload = json.loads(data['payload'])
    job = payload.get("job")
    strategy = job.get("sla_violation_strategy")
    app.logger.info(f"{strategy} violating service")
    if strategy == "migrate":
        # In case of a migration the instance has to be removed from the job's instance list
        instance_list = job['instance_list']
        app.logger.info(f"Remove node {client_id} from jobs instance list: {instance_list}")
        for i, instance in enumerate(instance_list):
            node_id = instance.get('worker_id')
            if node_id == client_id:
                # Remove worker id from instance
                del job["instance_list"][i]["worker_id"]
        app.logger.info(f"Update Job: {job}")
        mongo_upsert_job(job)
        mqtt_publish_edge_delete(client_id, job)

    scheduler_request_alarm(data)


def publish_vivaldi_message(client_id, worker_public_ip, worker_private_ip):
    # Tell the node what other nodes it should ping to update vivaldi coordinates
    vivaldi_information = []
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
            if worker_public_ip == node_public_ip and worker_private_ip != node_private_ip:
                vivaldi_information.append(
                    (node_public_ip, node_private_ip, node_router_rtt, node_vector, node_height, node_error))
            # Case 2: Node has public_ip so just check that ip to avoid self-ping
            elif worker_public_ip != node_public_ip:
                vivaldi_information.append(
                    (node_public_ip, node_private_ip, node_router_rtt, node_vector, node_height, node_error))

    # Add vivaldi information of the cluster orchestrator
    co_vector = co_vivaldi_coordinate.vector
    co_height = co_vivaldi_coordinate.height
    co_error = co_vivaldi_coordinate.error
    if validate_vivaldi_not_none(co_vector, co_height, co_error):
        vivaldi_information.append((co_public_ip, co_private_ip, co_router_rtt, co_vector.tolist(), co_height, co_error))
    # Shuffle the vivaldi information array and only pick first 'NEIGHBORS' nodes for ping measurements
    random.shuffle(vivaldi_information)
    mqtt_publish_vivaldi_message(client_id, vivaldi_information[:NEIGHBORS])