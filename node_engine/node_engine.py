from flask import Flask
from apscheduler.schedulers.background import BackgroundScheduler
import socketio
import socket
import time
import json
import os

from hardware_info import HardwareInfo
import dockerclient
from mqtt_client import mqtt_init, publish_cpu_mem
from ne_logging import configure_logging
from technology_support import verify_technology_support
from my_utils import deprecated


MY_PORT = os.environ.get('MY_PORT') or 3000

# PUSHING_INFO_DATA_JOB_INTERVAL: publish cpu/mem values regularly in seconds
PUSHING_INFO_DATA_JOB_INTERVAL = 8

clustermanager_addr = 'http://' + os.environ.get('CLUSTER_MANAGER_IP') + ':' + str(os.environ.get('CLUSTER_MANAGER_PORT'))

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
    node_info.technology = verify_technology_support()

    time.sleep(1)  # Wait to avoid Race Condition between sending first message and receiving connection establishment
    sio.emit('cs1', data=json.dumps(node_info.__dict__), namespace='/init')


@sio.on('sc2', namespace='/init')
def handle_init_final(jsonarg):
    app.logger.info('SocketIO - Received Cluster_Manager_to_Node_Engine_2:')
    data = json.loads(jsonarg)
    mqtt_port = data["MQTT_BROKER_PORT"]
    node_info.id = data["id"]
    node_info.subnetwork = data["SUBNETWORK"]
    dockerclient.node_info=node_info
    app.logger.info("Received mqtt_port: {}".format(mqtt_port))
    app.logger.info("My received ID is: {}\n\n\n".format(node_info.id))

    mqtt_init(app, mqtt_port, node_info.id)
    publish_cpu_mem(node_info.id)
    publish_cpu_memory(node_info.id)
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

    job_send_info = scheduler.add_job(publish_cpu_mem, 'interval', seconds=PUSHING_INFO_DATA_JOB_INTERVAL, args={id})
    scheduler.start()


if __name__ == '__main__':
    node_info = init()

    sio.connect(clustermanager_addr, namespaces=['/init'])
    app.run(debug=False, host='0.0.0.0', port=MY_PORT)
