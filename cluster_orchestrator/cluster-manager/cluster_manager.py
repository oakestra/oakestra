import os
from flask import Flask, request
from flask_socketio import SocketIO, emit
import json
import socketio
import sys
from apscheduler.schedulers.background import BackgroundScheduler
import time
from prometheus_client import start_http_server
import threading
from mongodb_client import mongo_init, mongo_upsert_node, mongo_upsert_job, mongo_find_job_by_system_id, \
    mongo_update_job_status, mongo_find_node_by_name, mongo_find_job_by_id, mongo_remove_job
from mqtt_client import mqtt_init, mqtt_publish_edge_deploy, mqtt_publish_edge_delete
from cluster_scheduler_requests import scheduler_request_deploy, scheduler_request_replicate, scheduler_request_status
from cm_logging import configure_logging
from system_manager_requests import send_aggregated_info_to_sm
from analyzing_workers import looking_for_dead_workers
from my_prometheus_client import prometheus_init_gauge_metrics, prometheus_set_metrics
from network_plugin_requests import *

MY_PORT = os.environ.get('MY_PORT')

MY_CHOSEN_CLUSTER_NAME = os.environ.get('CLUSTER_NAME')
MY_CLUSTER_LOCATION = os.environ.get('CLUSTER_LOCATION')
NETWORK_COMPONENT_PORT = os.environ.get('CLUSTER_SERVICE_MANAGER_PORT')
MY_ASSIGNED_CLUSTER_ID = None

SYSTEM_MANAGER_ADDR = 'http://' + os.environ.get('SYSTEM_MANAGER_URL') + ':' + os.environ.get('SYSTEM_MANAGER_PORT')

my_logger = configure_logging()

app = Flask(__name__)

# socketioserver = SocketIO(app, async_mode='eventlet', logger=, engineio_logger=logging)
socketioserver = SocketIO(app, logger=True, engineio_logger=True)

mongo_init(app)

mqtt_init(app)

sio = socketio.Client()

BACKGROUND_JOB_INTERVAL = 15


# ................... REST API Endpoints ...............#
# ......................................................#


@app.route('/')
def hello_world():
    app.logger.info('Hello World Request')
    app.logger.info('Processing default request')
    return "Hello, World! This is Cluster Manager's REST API"


@app.route('/status')
def status():
    app.logger.info('Incoming Request /status')
    return "ok", 200


@app.route('/api/deploy', methods=['GET', 'POST'])
def deploy_task():
    app.logger.info('Incoming Request /api/deploy')
    job = request.json  # contains job_id and job_description
    job_obj = mongo_upsert_job(job)
    scheduler_request_deploy(job_obj)
    return "ok"


@app.route('/api/result', methods=['POST'])
def get_scheduler_result_and_propagate_to_edge():
    # print(request)
    app.logger.info('Incoming Request /api/result - received cluster_scheduler result')
    data = request.json  # get POST body
    app.logger.info(data)

    job = data.get('job')
    resulting_node_id = data.get('node').get('_id')

    mongo_update_job_status(job.get('_id'), 'NODE_SCHEDULED', data.get('node'))
    job = mongo_find_job_by_id(job.get('_id'))

    # Inform network plugin about the deployment
    threading.Thread(group=None, target=network_notify_deployment,
                     args=(str(job['system_job_id']), job)).start()

    mqtt_publish_edge_deploy(resulting_node_id, job)
    return "ok"


@app.route('/api/delete/<system_job_id>')
def delete_service(system_job_id):
    """find service in db and ask corresponding worker to delete task"""
    app.logger.info('Incoming Request /api/delete/ - to delete task...')
    # job_id is the system_job_id assigned by System Manager
    job = mongo_find_job_by_system_id(system_job_id)
    node_id = job.get('instance_list')[0].get('worker_id')  # undeploy the first instance available
    job_id = str(job.get('_id'))
    job.__setitem__('_id', job_id)
    mqtt_publish_edge_delete(node_id, job)
    mongo_remove_job(str(job_id))
    return "ok"


@app.route('/api/replicate/', methods=['GET', 'POST'])
def replicate():
    """Replicate the amount of services"""
    app.logger.info('Incoming Request /api/replicate - to replicate amount of services')
    data = request.json
    job_id = data.get('job')
    desired_replicas = data.get('replicas')

    job_obj = mongo_find_job_by_system_id(job_id)
    current_replicas = job_obj.get('replicas')

    scheduler_request_replicate(job_obj, replicas=desired_replicas)

    return "ok", 200


@app.route('/api/move/', methods=['GET', 'POST'])
def move_application_from_node_to_node():
    """Request to move service from one Node to another Node in this cluster"""
    data = request.json  # get POST body
    job = data.get('job')
    node_source = data.get('node_from')
    node_target = data.get('node_to')

    job_obj = mongo_find_job_by_system_id(job)
    node_source_obj = mongo_find_node_by_name(node_source)
    node_target_obj = mongo_find_node_by_name(node_target)
    if node_source_obj == 'Error':
        return "Source Node Not Found", 404
    elif node_target_obj == 'Error':
        return "Target Node Not Found", 404
    # TODO ask Scheduler for permission and whether target_node is schedulable
    mqtt_publish_edge_delete(str(node_source_obj.get('_id')), job_obj)
    mqtt_publish_edge_deploy(str(node_target_obj.get('_id')), job_obj)
    return "ok", 200


# ................ Scheduler Test ......................#
# ......................................................#


@app.route('/api/test/scheduler', methods=['GET'])
def scheduler_test():
    app.logger.info('Incoming Request /api/jobs - to get all jobs')
    return scheduler_request_status()


# ..................... REST handshake .................#
# ......................................................#

@app.route('/api/node/register', methods=['POST'])
def http_node_registration():
    app.logger.info('Incoming Request /api/node/register - to get all jobs')
    data = request.json  # get POST body
    registration_token = data.get("token")
    # TODO: check and generate tokens
    client_id = mongo_upsert_node({"ip": request.remote_addr, "node_info": data})
    response = {
        "id": str(client_id),
        "MQTT_BROKER_PORT": os.environ.get('MQTT_BROKER_PORT')
    }
    return response, 200


# ...... Websocket INIT Handling with edge nodes .......#
# ......................................................#

@socketioserver.on('connect', namespace='/init')
def on_connect():
    app.logger.info('Websocket - Client connected: {}'.format(request.remote_addr))
    emit('sc1', {'hello-edge': 'please send your node info'}, namespace='/init')


@socketioserver.on('cs1', namespace='/init')
def handle_init_worker(message):
    app.logger.info('Websocket - Received Edge_to_Cluster_Manager_1: {}'.format(request.remote_addr))
    app.logger.info(message)

    client_id = mongo_upsert_node({"ip": request.remote_addr, "node_info": json.loads(message)})

    init_packet = {
        "id": str(client_id),
        "MQTT_BROKER_PORT": os.environ.get('MQTT_BROKER_PORT')
    }

    # create ID and send it along with MQTT_Broker info to the client. save id into database
    emit('sc2', json.dumps(init_packet), namespace='/init')

    # no report here because regularly reports are sent anyways over mqtt.
    # cloud_request_incr_node(MY_ASSIGNED_CLUSTER_ID)  # report to system-manager about new edge node


@socketioserver.on('disconnect', namespace='/init')
def test_disconnect():
    app.logger.info('Websocket - Client disconnected')


# ........... BEGIN register to System Manager .........#
# ......................................................#

@sio.on('sc1', namespace='/register')
def handle_init_greeting(jsonarg):
    app.logger.info('Websocket - received System_Manager_to_Cluster_Manager_1 : ' + str(jsonarg))
    data = {
        'manager_port': MY_PORT,
        'network_component_port': NETWORK_COMPONENT_PORT,
        'cluster_name': MY_CHOSEN_CLUSTER_NAME,
        'cluster_info': {},
        'cluster_location': MY_CLUSTER_LOCATION
    }
    time.sleep(1)  # Wait to Avoid Race Condition!

    sio.emit('cs1', data, namespace='/register')
    app.logger.info('Websocket - Cluster Info sent. (Cluster_Manager_to_System_Manager)')


@sio.on('sc2', namespace='/register')
def handle_init_final(jsonarg):
    app.logger.info('Websocket - received System_Manager_to_Cluster_Manager_2:' + str(jsonarg))
    data = json.loads(jsonarg)

    app.logger.info("My received ID is: {}\n\n\n".format(data['id']))

    global MY_ASSIGNED_CLUSTER_ID
    MY_ASSIGNED_CLUSTER_ID = data['id']

    sio.disconnect()
    if MY_ASSIGNED_CLUSTER_ID is not None:
        app.logger.info('Received ID. Go ahead with Background Jobs')
        prometheus_init_gauge_metrics(MY_ASSIGNED_CLUSTER_ID)
        background_job_send_aggregated_information_to_sm()
    else:
        app.logger.info('No ID received.')


@sio.event()
def connect():
    app.logger.info("Websocket - I'm connected to System_Manager!")


@sio.event()
def connect_error(m):
    app.logger.info("Websocket connection failed with System_Manager!")


@sio.event()
def error(sid, data):
    app.logger.info('>>>> Websocket error with System_Manager <<<<< ')


@sio.event()
def disconnect(m):
    app.logger.info("Websocket disconnected with SM!")


def init_cm_to_sm():
    app.logger.info('Connecting to System_Manager...')
    try:
        sio.connect(SYSTEM_MANAGER_ADDR + '/register', namespaces=['/register'])
    except Exception as e:
        app.logger.error('SocketIO - Connection Establishment with System Manager failed!')
    time.sleep(1)


# ......... FINISH - register at System Manager ........#
# ......................................................#


def background_job_send_aggregated_information_to_sm():
    app.logger.info("Set up Background Jobs...")
    scheduler = BackgroundScheduler()
    job_send_info = scheduler.add_job(send_aggregated_info_to_sm, 'interval', seconds=BACKGROUND_JOB_INTERVAL,
                                      kwargs={'my_id': MY_ASSIGNED_CLUSTER_ID,
                                              'time_interval': BACKGROUND_JOB_INTERVAL})
    job_dead_nodes = scheduler.add_job(looking_for_dead_workers, 'interval', seconds=2 * BACKGROUND_JOB_INTERVAL,
                                       kwargs={'interval': BACKGROUND_JOB_INTERVAL})
    scheduler.start()


if __name__ == '__main__':
    # socketioserver.run(app, debug=True, host='0.0.0.0', port=MY_PORT)
    # app.run(debug=True, host='0.0.0.0', port=MY_PORT)

    start_http_server(10001)  # start prometheus server

    import eventlet

    init_cm_to_sm()
    eventlet.wsgi.server(eventlet.listen(('0.0.0.0', int(MY_PORT))), app, log=my_logger)  # see README for logging notes
