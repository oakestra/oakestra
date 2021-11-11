from flask import Flask, request, Response
from apscheduler.schedulers.background import BackgroundScheduler
import socketio
import socket
import time
import json
import os

from flask_socketio import SocketIO, emit

from hardware_info import HardwareInfo
import dockerclient
import mqtt_client
from ne_logging import configure_logging
from net_manager_requests import net_manager_register
from technology_support import verify_technology_support
from my_utils import deprecated
from NetworkCoordinateSystem.network_measurements import parallel_ping
MY_PORT = os.environ.get('MY_PORT') or 3000

# PUSHING_INFO_DATA_JOB_INTERVAL: publish cpu/mem values regularly in seconds
PUSHING_INFO_DATA_JOB_INTERVAL = 8

# clustermanager_addr = 'http://' + os.environ.get('CLUSTER_MANAGER_IP') + ':' + str(
#     os.environ.get('CLUSTER_MANAGER_PORT'))
CLUSTER_MANAGER_IP="46.244.221.241"
CLUSTER_MANAGER_PORT=10000
clustermanager_addr = f"http://{CLUSTER_MANAGER_IP}:{CLUSTER_MANAGER_PORT}"
my_logger = configure_logging()

app = Flask(__name__)

sio = socketio.Client(app, logger=my_logger, engineio_logger=my_logger)

node_info = {}


def init():
    return HardwareInfo()


@app.route('/')
def hello_world():
    app.logger.info("Hello World Request")
    return "Hello, World! This is Node Engine's REST API"


@app.route('/status')
def status():
    app.logger.info('Incoming Request /status')
    return "ok", 200


@deprecated
@app.route('/docker/start')
def start_docker_container():
    app.logger.info('Incoming Request /docker/start')
    app.logger.info('Starting docker container......')
    # app.logger.error('Processing default request')
    # return start_container("library/nginx:alpine")  # as first example start an nginx


@app.route('/docker/stop_all')
def stop_all():
    app.logger.info('Incoming Request /docker/stop_all - Stop all...')
    return dockerclient.stop_all_running_containers()

@app.route('/ping', methods=['POST'])
def ping():
    app.logger.info(f"Incoming Request /api/deploy Body:{request.json}")
    target_ips = request.json
    statistics = parallel_ping(target_ips)

    return statistics, 200

# # ...... Websocket PING Handling with Cluster Manager .......#
# # ...........................................................#
socketioserver = SocketIO(app, logger=False, engineio_logger=False)

@socketioserver.on('connect', namespace='/ping')
def on_connect_ping():
    app.logger.info(f"Websocket - Client connected: {request.remote_addr}")

# def handle_nodes_topic_ping(payload):
#     app.logger.info("MQTT - Received PING command")
#     target_ips = payload.get('target_ips')
#     statistics = parallel_ping(target_ips)
#     app.logger.info(f"PING RESULT: {statistics}")
#     sio.connect("http://192.168.178.79:10000", namespaces=['/ping'])
#     sio.emit('cs1', data=json.dumps(statistics), namespace='/ping')
#     sio.disconnect()

@socketioserver.on('ping', namespace='/ping')
def handle_ping(message):
    target_ips = json.loads(message)
    statistics = parallel_ping(target_ips)
    app.logger.info(f"Ping results: {statistics}")
    emit('pong', data=json.dumps(statistics), namespace='/ping')


# ........ Websocket INIT begin with Cluster Manager .......
############################################################

@sio.on('sc1', namespace='/init')
def handle_init_greeting(jsonarg):
    app.logger.info('SocketIO - Received Cluster_Manager_to_Node_Engine_1 : ' + str(jsonarg))

    # print(node_info.uname)
    # print(node_info.cpu_count_physical)
    # print(node_info.cpu_count_total)
    # print(node_info.svmem)
    # print(node_info.swap)
    # print(node_info.partitions)
    # print(node_info.if_addrs)
    node_info.port = MY_PORT
    # node_info.technology = verify_technology_support()
    node_info.virtualization = verify_technology_support()

    time.sleep(1)  # Wait to avoid Race Condition between sending first message and receiving connection establishment
    # Transform each entry in list 'gpus' from type GPUtil.GPU to a dict, because GPU cannot be serialized.
    for gpu in node_info.gpus:
        node_info.gpus[node_info.gpus.index(gpu)] = gpu.__dict__
    sio.emit('cs1', data=json.dumps(node_info.__dict__), namespace='/init')


@sio.on('sc2', namespace='/init')
def handle_init_final(jsonarg):
    # get initial node config
    app.logger.info('SocketIO - Received Cluster_Manager_to_Node_Engine_2:')
    data = json.loads(jsonarg)
    mqtt_port = data["MQTT_BROKER_PORT"]
    node_info.id = data["id"]
    node_info.subnetwork = data["SUBNETWORK"]
    mqtt_client.node_info = node_info
    app.logger.info("Received mqtt_port: {}".format(mqtt_port))
    app.logger.info("My received ID is: {}\n\n\n".format(node_info.id))

    # register to the netManager
    # TODO: analyse why pyshark only listens to GoProdyTun when we register the netmanager
    # net_manager_register(node_info.subnetwork)

    # publish node info
    mqtt_client.mqtt_init(app, mqtt_port, node_info.id)
    mqtt_client.publish_cpu_mem(node_info.id)
    publish_cpu_memory(node_info.id)

    # disconnect the Socket
    sio.sleep(1)
    sio.disconnect()


@sio.event(namespace='/init')
def connect():
    app.logger.info("SocketIO - connected!")


@sio.event()
def disconnect(sid):
    app.logger.info("SocketIO - disconnected!")


@sio.event(namespace='/init')
def connect_error(message):
    app.logger.info("SocketIO - error: " + message)


# ........ Websocket INIT finish with Cluster Manager ......
############################################################


def publish_cpu_memory(id):
    scheduler = BackgroundScheduler()

    job_send_info = scheduler.add_job(mqtt_client.publish_cpu_mem, 'interval', seconds=PUSHING_INFO_DATA_JOB_INTERVAL,
                                      args={id})
    scheduler.start()


if __name__ == '__main__':
    node_info = init()
    # Start redis-server to SLA monitoring background jobs
    if not dockerclient.is_running("worker_redis"):
        container_id = dockerclient.start_redis()
    sio.connect(clustermanager_addr, namespaces=['/init'])


    app.run(debug=False, host='0.0.0.0', port=MY_PORT)
