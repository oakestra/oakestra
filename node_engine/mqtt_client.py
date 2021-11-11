import os

import numpy as np
import socketio
from flask_mqtt import Mqtt
import json
import re

from flask_socketio import SocketIO

import Monitoring.monitoring
from NetworkCoordinateSystem.network_measurements import get_netem_delay, parallel_ping, ping, get_ip_info
from NetworkCoordinateSystem.vivaldi_coordinate import VivaldiCoordinate
from cpu_mem import get_cpu_memory, get_memory
from dockerclient import start_container, stop_container
from mirageosclient import run_unikernel_mirageos
from coordinates import get_coordinates
from ne_logging import configure_logging

mqtt = None
app = None
sio = socketio.Client(app, logger=False, engineio_logger=False)
vivaldi_coordinate = None
public_ip = None
private_ip = None
router_rtt = None
node_info = {}
VIVALDI_DIM = 2

def mqtt_init(flask_app, mqtt_port=1883, my_id=None):
    global mqtt
    global app
    global req
    global vivaldi_coordinate
    global public_ip
    global private_ip
    global router_rtt

    app = flask_app
    # TODO: first check if node already exists in mongodb an get coords if thats the case
    vivaldi_coordinate = VivaldiCoordinate(VIVALDI_DIM)
    public_ip, private_ip, router_rtt = get_ip_info()
    app.config['MQTT_BROKER_URL'] = os.environ.get('CLUSTER_MANAGER_IP')
    app.config['MQTT_BROKER_PORT'] = int(mqtt_port)
    app.config['MQTT_REFRESH_TIME'] = 3.0  # refresh time in seconds
    mqtt = Mqtt(app)
    app.logger.info('initialized mqtt')
    mqtt.subscribe('nodes/' + my_id + '/control/+')
    # Subscribe to the ping channel. If a worker receives a message via that channel it should ping the IP contained in the message
    mqtt.subscribe('nodes/' + my_id + '/ping')
    mqtt.subscribe('nodes/' + my_id + '/vivaldi')

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
        re_nodes_topic_ping = re.search("^nodes/" + my_id + "/ping$", topic)
        re_nodes_topic_vivaldi = re.search("^nodes/" + my_id + "/vivaldi", topic)

        payload = data.get('payload')
        # If the node received a message via the ping channel it should ping the IP contained in the received message
        if re_nodes_topic_ping is not None:
            handle_nodes_topic_ping(payload)
        if re_nodes_topic_vivaldi is not None:
            handle_nodes_topic_vivaldi(payload)
        if re_nodes_topic_control_deploy is not None:
            handle_nodes_topic_control_deploy(payload)
        elif re_nodes_topic_control_delete is not None:
            handle_nodes_topic_control_delete(payload)


def publish_cpu_mem(my_id):
    app.logger.info('Publishing CPU+Memory usage... my ID: {0}'.format(my_id))
    cpu_used, free_cores, memory_used, free_memory_in_MB = get_cpu_memory()
    mem_value = get_memory()
    topic = 'nodes/' + my_id + '/information'
    lat, long = get_coordinates()
    # If ip address of this node is private, the node has to ping the network router such that this RTT can be added
    # to nodes pinging this network. Otherwise the Vivaldi network coordinates would update themself with respect to
    # the router and not the nodes within the network
    is_netem_configured = os.environ.get('IS_NETEM_CONFIGURED') == 'TRUE'
    mqtt.publish(topic, json.dumps({'cpu': cpu_used, 'free_cores': free_cores,
                                    'memory': memory_used, 'memory_free_in_MB': free_memory_in_MB,
                                    'lat': lat, 'long': long,'vivaldi_vector': vivaldi_coordinate.vector.tolist(),
                                    'vivaldi_height': vivaldi_coordinate.height,
                                    'vivaldi_error': vivaldi_coordinate.error,
                                    'public_ip': public_ip, 'private_ip': private_ip, 'router_rtt': router_rtt,
                                    'netem_delay': get_netem_delay(is_netem_configured)}))


def publish_sla_alarm(my_id, violated_job, ip_rtt_stats):
    # app.logger.info(f"Publishing SLA violation alarm... my ID: {my_id}")
    print(f"Publishing SLA violation alarm... my ID: {my_id}\n")
    # topic = f"nodes/{my_id}/alarm"
    topic = 'nodes/' + my_id + '/alarm'
    # ip_rtt_stats = {<violating ip>: <violating rtt>,...}
    mqtt.publish(topic, json.dumps({'job': violated_job, 'ip_rtt_stats': ip_rtt_stats}))


def publish_deploy_status(my_id, job_id, status, ns_ip):
    app.logger.info('Publishing Deployment status... my ID: {0}'.format(my_id))
    topic = 'nodes/' + my_id + '/job'
    mqtt.publish(topic, json.dumps({'job_id': job_id, 'status': status,
                                    'ns_ip': ns_ip}))


def handle_nodes_topic_control_deploy(payload):
    app.logger.info("MQTT - Received .../control/deploy command")
    app.logger.info(f"PAYLOAD {payload}")
    address = None
    job = payload.get('job')
    job_id = job['_id']
    job_name = job['job_name']
    virtualization = job['virtualization']
    image_url = job['code']
    port = job['port']
    constraints = payload.get('constraints')
    if virtualization == 'docker':
        address, container_id = start_container(job=job)
        app.logger.info("Start Celery monitoring task")
        Monitoring.monitoring.monitor_docker_container(payload, container_id)
    if virtualization == 'mirage':
        commands = payload.get('commands')
        run_unikernel_mirageos(image_url, job_name, job_name, commands)
    if address is not None:
        publish_deploy_status(node_info.id, job.get('_id'), 'DEPLOYED', address)
    else:
        publish_deploy_status(node_info.id, job.get('_id'), 'FAILED', '')


def handle_nodes_topic_control_delete(payload):
    job = payload.get('job')
    virtualization = job['virtualization']
    job_name = job['job_nameb']
    app.logger.info('MQTT - Received .../control/delete command')
    if virtualization == 'docker':
        stop_container(job_name)


def handle_nodes_topic_vivaldi(payload):
    app.logger.info("MQTT - Received Vivaldi command")
    vivaldi_info = payload.get('vivaldi_info')
    app.logger.info(f"Received vivaldi infos: {vivaldi_info}")
    # Dict has target IP as key and a tuple consisting of the remote VivaldiCoordinate and the router_rtt if required
    ip_vivaldi_dict = {}
    for info in vivaldi_info:
        # vivaldi_info = (public_ip, private_ip, router_rtt, vector, height, error)
        remote_public_ip = info[0]
        remote_private_ip = info[1]
        remote_router_rtt = float(info[2])
        remote_vector = info[3]
        remote_height = info[4]
        remote_error = info[5]
        remote_vivaldi = VivaldiCoordinate(VIVALDI_DIM)
        remote_vivaldi.vector = np.array(remote_vector)
        remote_vivaldi.height = float(remote_height)
        remote_vivaldi.error = float(remote_error)
        # this node is in same network as remote and both behind router -> same public ip and private ip not null -> ping private ip
        if public_ip == remote_public_ip:
            ip_vivaldi_dict.setdefault(remote_private_ip, []).append((remote_vivaldi, None))
        # This node and the remote node are not within the same network
        if public_ip != remote_public_ip:
            ip_vivaldi_dict.setdefault(remote_public_ip, []).append((remote_vivaldi, remote_router_rtt))

            # Ping received IPs in parallel
            statistics = parallel_ping(ip_vivaldi_dict.keys())
            app.logger.info(f"Ping statistics: {statistics}")
            for ip, rtt in statistics.items():
                viv_router_rtts = ip_vivaldi_dict[ip] # TODO: Naming!
                for _viv, _router_rtt in viv_router_rtts:
                    if _router_rtt is not None:
                        app.logger.info(f"IF: update to {ip} with {rtt+_router_rtt}")
                        total_rtt = rtt + _router_rtt
                        vivaldi_coordinate.update(total_rtt, _viv)
                    else:
                        app.logger.info(f"ELSE: update to {ip} with {rtt}")
                        vivaldi_coordinate.update(rtt, _viv)

socketioserver = SocketIO(app, logger=True, engineio_logger=True)
def handle_nodes_topic_ping(payload):
    app.logger.info("MQTT - Received PING command")
    target_ips = payload.get('target_ips')
    statistics = parallel_ping(target_ips)
    app.logger.info(f"PING RESULT: {statistics}")
    sio.connect("http://46.244.221.241:10000", namespaces=['/ping'])
    sio.emit('cs1', data=json.dumps(statistics), namespace='/ping')
    sio.disconnect()