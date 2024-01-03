import json
import os
import time

import service_operations
import socketio
from analyzing_workers import looking_for_dead_workers
from apscheduler.schedulers.background import BackgroundScheduler
from cluster_scheduler_requests import scheduler_request_status
from cm_logging import configure_logging
from flask import Flask, request
from flask_socketio import SocketIO, emit
from mongodb_client import (
    mongo_find_job_by_system_id,
    mongo_init,
    mongo_update_job_status,
    mongo_upsert_node,
)
from mqtt_client import mqtt_init, mqtt_publish_edge_deploy
from my_prometheus_client import prometheus_init_gauge_metrics
from network_plugin_requests import network_notify_deployment
from prometheus_client import start_http_server
from system_manager_requests import re_deploy_dead_services_routine, send_aggregated_info_to_sm

MY_PORT = os.environ.get("MY_PORT")

MY_CHOSEN_CLUSTER_NAME = os.environ.get("CLUSTER_NAME")
MY_CLUSTER_LOCATION = os.environ.get("CLUSTER_LOCATION")
NETWORK_COMPONENT_PORT = os.environ.get("CLUSTER_SERVICE_MANAGER_PORT")
MY_ASSIGNED_CLUSTER_ID = None

SYSTEM_MANAGER_ADDR = (
    "http://" + os.environ.get("SYSTEM_MANAGER_URL") + ":" + os.environ.get("SYSTEM_MANAGER_PORT")
)

my_logger = configure_logging()

app = Flask(__name__)

# socketioserver = SocketIO(app, async_mode='eventlet', logger=, engineio_logger=logging)
socketioserver = SocketIO(app, logger=True, engineio_logger=True)

mongo_init(app)

mqtt_init(app)

sio = socketio.Client()

BACKGROUND_JOB_INTERVAL = 5


# ................... REST API Endpoints ...............#
# ......................................................#


@app.route("/")
def hello_world():
    app.logger.info("Hello World Request")
    app.logger.info("Processing default request")
    return "Hello, World! This is Cluster Manager's REST API"


@app.route("/status")
def status():
    app.logger.info("Incoming Request /status")
    return "ok", 200


@app.route("/api/deploy/<system_job_id>/<instance_number>", methods=["GET", "POST"])
def deploy_task(system_job_id, instance_number):
    app.logger.info("Incoming Request /api/deploy")
    job = request.json  # contains job_id and job_description

    try:
        service_operations.deploy_service(job, system_job_id, instance_number)
    except Exception:
        return "", 500

    return "", 200


@app.route("/api/result/<system_job_id>/<instance_number>", methods=["POST"])
def get_scheduler_result_and_propagate_to_edge(system_job_id, instance_number):
    # print(request)
    app.logger.info("Incoming Request /api/result - received cluster_scheduler result")
    data = request.json  # get POST body
    app.logger.info(data)

    if data.get("found", False):
        resulting_node_id = data.get("node").get("_id")

        mongo_update_job_status(system_job_id, instance_number, "NODE_SCHEDULED", data.get("node"))
        job = mongo_find_job_by_system_id(system_job_id)

        # Inform network plugin about the deployment
        network_notify_deployment(str(job["system_job_id"]), job)

        # Publish job
        mqtt_publish_edge_deploy(resulting_node_id, job, instance_number)
    else:
        mongo_update_job_status(system_job_id, instance_number, "NO_WORKER_CAPACITY", None)
    return "ok"


@app.route("/api/delete/<system_job_id>/<instance_number>")
def delete_service(system_job_id, instance_number):
    """
    find service in db and ask corresponding worker to delete task,
    instance_number -1 undeploy all known instances
    """
    app.logger.info("Incoming Request /api/delete/ - to delete task...")

    try:
        service_operations.delete_service(system_job_id, instance_number)
    except Exception:
        return "", 500

    return "ok", 200


# ................ Scheduler Test ......................#
# ......................................................#


@app.route("/api/test/scheduler", methods=["GET"])
def scheduler_test():
    app.logger.info("Incoming Request /api/jobs - to get all jobs")
    return scheduler_request_status()


# ..................... REST handshake .................#
# ......................................................#


@app.route("/api/node/register", methods=["POST"])
def http_node_registration():
    app.logger.info("Incoming Request /api/node/register - to get all jobs")
    data = request.json  # get POST body
    data.get("token")  # registration_token
    # TODO: check and generate tokens
    client_id = mongo_upsert_node({"ip": request.remote_addr, "node_info": data})
    response = {
        "id": str(client_id),
        "MQTT_BROKER_PORT": os.environ.get("MQTT_BROKER_PORT"),
    }
    return response, 200


# ...... Websocket INIT Handling with edge nodes .......#
# ......................................................#


@socketioserver.on("connect", namespace="/init")
def on_connect():
    app.logger.info("Websocket - Client connected: {}".format(request.remote_addr))
    emit("sc1", {"hello-edge": "please send your node info"}, namespace="/init")


@socketioserver.on("cs1", namespace="/init")
def handle_init_worker(message):
    app.logger.info(
        "Websocket - Received Edge_to_Cluster_Manager_1: {}".format(request.remote_addr)
    )
    app.logger.info(message)

    client_id = mongo_upsert_node({"ip": request.remote_addr, "node_info": json.loads(message)})

    init_packet = {
        "id": str(client_id),
        "MQTT_BROKER_PORT": os.environ.get("MQTT_BROKER_PORT"),
    }

    # create ID and send it along with MQTT_Broker info to the client. save id into database
    emit("sc2", json.dumps(init_packet), namespace="/init")

    # no report here because regularly reports are sent anyways over mqtt.
    # report to system-manager about new edge node
    # cloud_request_incr_node(MY_ASSIGNED_CLUSTER_ID)


@socketioserver.on("disconnect", namespace="/init")
def test_disconnect():
    app.logger.info("Websocket - Client disconnected")


# ........... BEGIN register to System Manager .........#
# ......................................................#


@sio.on("sc1", namespace="/register")
def handle_init_greeting(jsonarg):
    app.logger.info("Websocket - received System_Manager_to_Cluster_Manager_1 : " + str(jsonarg))
    data = {
        "manager_port": MY_PORT,
        "network_component_port": NETWORK_COMPONENT_PORT,
        "cluster_name": MY_CHOSEN_CLUSTER_NAME,
        "cluster_info": {},
        "cluster_location": MY_CLUSTER_LOCATION,
    }
    time.sleep(1)  # Wait to Avoid Race Condition!

    sio.emit("cs1", data, namespace="/register")
    app.logger.info("Websocket - Cluster Info sent. (Cluster_Manager_to_System_Manager)")


@sio.on("sc2", namespace="/register")
def handle_init_final(jsonarg):
    app.logger.info("Websocket - received System_Manager_to_Cluster_Manager_2:" + str(jsonarg))
    data = json.loads(jsonarg)

    app.logger.info("My received ID is: {}\n\n\n".format(data["id"]))

    global MY_ASSIGNED_CLUSTER_ID
    MY_ASSIGNED_CLUSTER_ID = data["id"]

    sio.disconnect()
    if MY_ASSIGNED_CLUSTER_ID is not None:
        app.logger.info("Received ID. Go ahead with Background Jobs")
        prometheus_init_gauge_metrics(MY_ASSIGNED_CLUSTER_ID, app.logger)
        background_job_send_aggregated_information_to_sm()
    else:
        app.logger.info("No ID received.")


@sio.event()
def connect():
    app.logger.info("Websocket - I'm connected to System_Manager!")


@sio.event()
def connect_error(m):
    app.logger.info("Websocket connection failed with System_Manager!")


@sio.event()
def error(sid, data):
    app.logger.info(">>>> Websocket error with System_Manager <<<<< ")


@sio.event()
def disconnect(m):
    app.logger.info("Websocket disconnected with SM!")


def init_cm_to_sm():
    app.logger.info("Connecting to System_Manager...")
    try:
        sio.connect(SYSTEM_MANAGER_ADDR + "/register", namespaces=["/register"])
    except Exception:
        app.logger.error("SocketIO - Connection Establishment with System Manager failed!")
    time.sleep(1)


# ......... FINISH - register at System Manager ........#
# ......................................................#


def background_job_send_aggregated_information_to_sm():
    app.logger.info("Set up Background Jobs...")
    scheduler = BackgroundScheduler()
    # job_send_info
    scheduler.add_job(
        send_aggregated_info_to_sm,
        "interval",
        seconds=BACKGROUND_JOB_INTERVAL,
        kwargs={
            "my_id": MY_ASSIGNED_CLUSTER_ID,
            "time_interval": 2 * BACKGROUND_JOB_INTERVAL,
        },
    )
    # job_dead_nodes
    scheduler.add_job(
        looking_for_dead_workers,
        "interval",
        seconds=BACKGROUND_JOB_INTERVAL,
        kwargs={"interval": 2 * BACKGROUND_JOB_INTERVAL},
    )
    # job_re_deploy_dead_jobs
    scheduler.add_job(re_deploy_dead_services_routine, "interval", seconds=BACKGROUND_JOB_INTERVAL)

    scheduler.start()


if __name__ == "__main__":
    # socketioserver.run(app, debug=True, host='0.0.0.0', port=MY_PORT)
    # app.run(debug=True, host='0.0.0.0', port=MY_PORT)

    start_http_server(10001)  # start prometheus server

    import eventlet

    init_cm_to_sm()
    eventlet.wsgi.server(
        eventlet.listen(("0.0.0.0", int(MY_PORT))), app, log=my_logger
    )  # see README for logging notes
