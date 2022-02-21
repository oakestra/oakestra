from flask import Blueprint
from apscheduler.schedulers.background import BackgroundScheduler
from apscheduler.triggers.interval import IntervalTrigger
import socketio
import numpy as np
import time
import json
import os

from app.blueprints.node_engine.net_manager_requests import net_manager_register
from app.extensions import mqtt_client as mqtt_client
from app.blueprints.node_engine import hardware_info, dockerclient, net_manager_requests, my_utils, technology_support
from app.blueprints.network_measurement import network_measurement
from app.extensions.logging import configure_logging
from app.models.vivaldi_coordinate import VivaldiCoordinate

PUBLIC_WORKER_PORT = os.environ.get('PUBLIC_WORKER_PORT') or 3000
CLUSTERMANAGER_ADDR = f"http://{os.environ.get('CLUSTER_MANAGER_IP')}:{os.environ.get('CLUSTER_MANAGER_PORT')}"

my_logger = configure_logging("node_engine")
node_engine = Blueprint("node_engine", __name__)

# PUSHING_INFO_DATA_JOB_INTERVAL: publish cpu/mem values regularly in seconds
PUSHING_INFO_DATA_JOB_INTERVAL = 8
mqtt_port = None
node_info = {}
sio = socketio.Client()

@node_engine.route("/alarm")
def publish_alarm_api():
    mqtt_client.publish_sla_alarm("TEST", None, ip_rtt_stats=None)
    return "ok", 200

def init_node_engine():
    global node_info
    node_info = hardware_info.HardwareInfo()
    sio.connect(CLUSTERMANAGER_ADDR, namespaces=['/init'])

@node_engine.route('/')
def hello_world():
    my_logger.info("Hello World Request")
    return "Hello, World! This is Node Engine's REST API"


@node_engine.route('/status')
def status():
    my_logger.info('Incoming Request /status')
    return "ok", 200


@my_utils.deprecated
@node_engine.route('/docker/start')
def start_docker_container():
    my_logger.info('Incoming Request /docker/start')
    my_logger.info('Starting docker container......')
    # node_engine.logger.error('Processing default request')
    # return start_container("library/nginx:alpine")  # as first example start an nginx


@node_engine.route('/docker/stop_all')
def stop_all():
    my_logger.info('Incoming Request /docker/stop_all - Stop all...')
    return dockerclient.stop_all_running_containers()



def get_node_infos():
    # print(node_info.uname)
    # print(node_info.cpu_count_physical)
    # print(node_info.cpu_count_total)
    # print(node_info.svmem)
    # print(node_info.swap)
    # print(node_info.partitions)
    # print(node_info.if_addrs)
    node_info.port = os.environ.get("PUBLIC_WORKER_PORT")
    # node_info.technology = technology_support.verify_technology_support()
    node_info.virtualization = technology_support.verify_technology_support()

    time.sleep(1)  # Wait to avoid Race Condition between sending first message and receiving connection establishment
    # Transform each entry in list 'gpus' from type GPUtil.GPU to a dict, because GPU cannot be serialized.
    for gpu in node_info.gpus:
        node_info.gpus[node_info.gpus.index(gpu)] = gpu.__dict__

    return node_info


def publish_cpu_memory():
    scheduler = BackgroundScheduler()
    #job_send_info = scheduler.add_job(mqtt_client.publish_cpu_mem, 'interval', seconds=PUSHING_INFO_DATA_JOB_INTERVAL)
    # @implNote: Create trigger with specified timezone to avoid warning from apscheduler
    # see https://stackoverflow.com/questions/69776414/pytzusagewarning-the-zone-attribute-is-specific-to-pytzs-interface-please-mig
    trigger = IntervalTrigger(seconds=PUSHING_INFO_DATA_JOB_INTERVAL, timezone="Europe/Berlin")

    job_send_info = scheduler.add_job(mqtt_client.publish_cpu_mem, trigger=trigger) #, args=(id, str(viv_vector), viv_height, viv_error))
    scheduler.start()

# ........ Websocket INIT begin with Cluster Manager .......
############################################################

@sio.on('sc1', namespace='/init')
def handle_init_greeting(jsonarg):
    my_logger.info('SocketIO - Received Cluster_Manager_to_Node_Engine_1 : ' + str(jsonarg))

    # print(node_info.uname)
    # print(node_info.cpu_count_physical)
    # print(node_info.cpu_count_total)
    # print(node_info.svmem)
    # print(node_info.swap)
    # print(node_info.partitions)
    # print(node_info.if_addrs)
    node_info.port = PUBLIC_WORKER_PORT
    # node_info.technology = verify_technology_support()
    node_info.virtualization = technology_support.verify_technology_support()

    time.sleep(1)  # Wait to avoid Race Condition between sending first message and receiving connection establishment
    # Transform each entry in list 'gpus' from type GPUtil.GPU to a dict, because GPU cannot be serialized.
    for gpu in node_info.gpus:
        node_info.gpus[node_info.gpus.index(gpu)] = gpu.__dict__
    sio.emit('cs1', data=json.dumps(node_info.__dict__), namespace='/init')


@sio.on('sc2', namespace='/init')
def handle_init_final(jsonarg):
    global mqtt_port
    # get initial node config
    data = json.loads(jsonarg)
    my_logger.info(f'SocketIO - Received Cluster_Manager_to_Node_Engine_2: {data}')
    mqtt_port = data["MQTT_BROKER_PORT"]
    node_info.id = data["id"]
    my_logger.info("Received mqtt_port: {}".format(mqtt_port))
    my_logger.info("My received ID is: {}\n\n\n".format(node_info.id))

    # get nodes Vivaldi information
    vivaldi_info = data["VIVALDI"]
    my_logger.info(f"Vivaldi info received from CO: {vivaldi_info}")
    # register to the netManager
    net_manager_register(node_info.id)

    # publish node info
    ip_info = network_measurement.get_ip_info()
    viv_vector = vivaldi_info["vector"]
    viv_height = vivaldi_info["height"]
    viv_error = vivaldi_info["error"]
    vivaldi_coord = VivaldiCoordinate(len(viv_vector))
    vivaldi_coord.vector = np.array(viv_vector)
    vivaldi_coord.height = float(viv_height)
    vivaldi_coord.error = float(viv_error)
    mqtt_client.mqtt_init(node_info, vivaldi_coord, ip_info)
    mqtt_client.publish_cpu_mem()
    publish_cpu_memory()

    # disconnect the Socket
    sio.sleep(1)
    sio.disconnect()


@sio.event(namespace='/init')
def connect():
    my_logger.info("SocketIO - connected!")


@sio.event()
def disconnect(sid):
    my_logger.info("SocketIO - disconnected!")


@sio.event(namespace='/init')
def connect_error(message):
    my_logger.info("SocketIO - error: " + message)


# ........ Websocket INIT finish with Cluster Manager ......
############################################################
