import json
import logging
import os
import socket
import threading
import time

import config
import grpc
from apscheduler.schedulers.background import BackgroundScheduler
from blueprints import blueprints
from clients.mqtt_client import mqtt_init
from clients.my_prometheus_client import prometheus_init_gauge_metrics
from cm_logging import configure_logging
from ext_requests.system_manager_requests import (
    re_deploy_dead_jobs_routine,
    send_aggregated_info_to_sm,
)
from flask import Flask
from flask_cors import CORS
from flask_smorest import Api
from flask_socketio import SocketIO
from flask_swagger_ui import get_swaggerui_blueprint
from prometheus_client import start_http_server
from proto.clusterRegistration_pb2 import CS1Message, CS2Message, KeyValue, SC1Message, SC2Message
from proto.clusterRegistration_pb2_grpc import register_clusterStub

my_logger = configure_logging()
logger = logging.getLogger("cluster_manager")
app = Flask(__name__)

app.config["OPENAPI_VERSION"] = "3.0.2"
app.config["API_TITLE"] = "Oakestra root api"
app.config["API_VERSION"] = "v1"
app.config["OPENAPI_URL_PREFIX"] = "/docs"
app.config["JWT_ALGORITHM"] = "RS256"
app.logger = logger

socketioserver = SocketIO(app, logger=True, engineio_logger=True)
api = Api(app, spec_kwargs={"x-internal-id": "1", "host": "oakestra.io"})
cors = CORS(app, resources={r"/*": {"origins": "*"}})

mqtt_init(app)

BACKGROUND_JOB_INTERVAL = 15

# Register apis
for bp in blueprints:
    api.register_blueprint(bp)

# Swagger docs
SWAGGER_URL = "/api/docs"
API_URL = "/docs/openapi.json"
swaggerui_blueprint = get_swaggerui_blueprint(
    SWAGGER_URL,
    API_URL,
    config={"app_name": "Oakestra root orchestrator"},
)
app.register_blueprint(swaggerui_blueprint)


def background_job_send_aggregated_information_to_sm():
    logger.info("Set up Background Jobs...")
    scheduler = BackgroundScheduler()
    # job_send_info
    scheduler.add_job(
        send_aggregated_info_to_sm,
        "interval",
        seconds=BACKGROUND_JOB_INTERVAL,
        kwargs={
            "my_id": config.MY_ASSIGNED_CLUSTER_ID,
            "time_interval": 2 * BACKGROUND_JOB_INTERVAL,
        },
    )
    # job_re_deploy_dead_jobs
    scheduler.add_job(re_deploy_dead_jobs_routine, "interval", seconds=BACKGROUND_JOB_INTERVAL)

    scheduler.start()


# ........... BEGIN register to System Manager with gRPC........ .........#
# ........................................................................#


def register_with_system_manager():
    """Registers this cluster manager with the system manager using gRPC."""

    if not config.MY_CLUSTER_ADDRESS:
        raise RuntimeError(
            "CLUSTER_ADDRESS env var is not set. The root orchestrator needs the "
            "reachable IP of this cluster manager. Set CLUSTER_ADDRESS to the "
            "IP/hostname the root can use to reach this host."
        )

    with grpc.insecure_channel(config.SYSTEM_MANAGER_ADDR) as channel:
        stub = register_clusterStub(channel)

        # Send initial greeting (CS1Message)
        greeting = CS1Message()
        greeting.hello_service_manager = json.dumps(
            {
                "cluster_name": config.MY_CHOSEN_CLUSTER_NAME,
                "location": config.MY_CLUSTER_LOCATION,
            }
        )
        sc1: SC1Message = stub.handle_init_greeting(
            greeting, wait_for_ready=True, timeout=config.GRPC_REQUEST_TIMEOUT
        )
        logger.info(
            "Received greeting message from System Manager: " + str(sc1.hello_cluster_manager)
        )

        # Send cluster details (CS2Message)
        details = CS2Message()
        details.manager_port = int(config.MY_PORT)
        details.network_component_port = int(config.NETWORK_COMPONENT_PORT)
        details.cluster_name = config.MY_CHOSEN_CLUSTER_NAME
        details.cluster_location = config.MY_CLUSTER_LOCATION
        details.cluster_address = config.MY_CLUSTER_ADDRESS
        details.cluster_info.append(KeyValue())

        sc2: SC2Message = stub.handle_init_final(
            details, wait_for_ready=True, timeout=config.GRPC_REQUEST_TIMEOUT
        )

        if not sc2.id:
            raise RuntimeError("Registration failed: no cluster id returned by root")

        config.MY_ASSIGNED_CLUSTER_ID = sc2.id
        logger.info(f"Cluster ID received: {sc2.id}. Go ahead with Background Jobs")
        prometheus_init_gauge_metrics(config.MY_ASSIGNED_CLUSTER_ID, app.logger)
        background_job_send_aggregated_information_to_sm()


# ........... FINISH - register to System Manager with gRPC.................#
# ..........................................................................#

start_http_server(10001)  # start prometheus server


def _register_in_background():
    # The root probes GET /api/cluster/status on this cluster_manager during
    # registration. That probe can only succeed once gunicorn's worker has
    # entered its accept loop, which doesn't happen until load_wsgi (i.e. this
    # module's top-level import) returns. So we MUST NOT block the import on
    # the gRPC call — otherwise the root's probe deadlocks against our own
    # startup. Give gunicorn a moment to start serving, then register. On
    # failure, exit the worker so gunicorn respawns it and tries again.
    time.sleep(2)
    try:
        register_with_system_manager()
    except Exception:
        logger.exception("Cluster registration failed; exiting worker for restart")
        os._exit(1)


threading.Thread(target=_register_in_background, daemon=True).start()

if __name__ == "__main__":
    import eventlet

    eventlet.wsgi.server(
        eventlet.listen(("::", int(config.MY_PORT)), family=socket.AF_INET6), app, log=my_logger
    )  # see README for logging notes
