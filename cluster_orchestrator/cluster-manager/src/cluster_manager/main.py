import json
import logging
import socket
import sys
import threading
import time

import grpc
from apscheduler.schedulers.background import BackgroundScheduler
from flask import Flask
from flask_cors import CORS
from flask_smorest import Api
from flask_socketio import SocketIO
from flask_swagger_ui import get_swaggerui_blueprint
from prometheus_client import start_http_server

from .app_config import CONFIG, GRPC_REQUEST_TIMEOUT, RUNTIME_CONFIG, SYSTEM_MANAGER_GRPC_ADDR
from .app_logging import configure_logging
from .blueprints import blueprints
from .clients.mqtt_client import initialize_mqtt
from .clients.my_prometheus_client import prometheus_init_gauge_metrics
from .ext_requests.system_manager_requests import (
    re_deploy_dead_jobs_routine,
    send_aggregated_info_to_sm,
)
from .proto.cluster_registration_pb2 import CS1Message, CS2Message, KeyValue, SC1Message, SC2Message
from .proto.cluster_registration_pb2_grpc import register_clusterStub

SWAGGER_URL = "/api/docs"
API_URL = "/docs/openapi.json"

logger: logging.Logger = configure_logging()
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

initialize_mqtt()

# Register apis
for bp in blueprints:
    api.register_blueprint(bp)

# Swagger docs
swaggerui_blueprint = get_swaggerui_blueprint(
    SWAGGER_URL,
    API_URL,
    config={"app_name": "Oakestra root orchestrator"},
)
app.register_blueprint(swaggerui_blueprint)


def launch_background_jobs(assigned_cluster_id: str):
    logger.info("Setting up background jobs...")
    scheduler = BackgroundScheduler()

    aggregation_interval = (
        CONFIG.aggregation_interval if CONFIG.aggregation_interval is not None else 15
    )
    node_scheduled_timeout = (
        CONFIG.node_scheduled_timeout if CONFIG.node_scheduled_timeout is not None else 15
    )

    # job_send_info
    scheduler.add_job(
        send_aggregated_info_to_sm,
        "interval",
        seconds=aggregation_interval,
        kwargs={
            "assigned_cluster_id": assigned_cluster_id,
            "running_timeout": aggregation_interval * 2,
            "node_scheduled_timeout": node_scheduled_timeout,
        },
    )

    # job_re_deploy_dead_jobs
    scheduler.add_job(re_deploy_dead_jobs_routine, "interval", seconds=aggregation_interval)

    scheduler.start()


# ........... BEGIN register to System Manager with gRPC........ .........#
# ........................................................................#


def register_with_system_manager():
    """Registers this cluster manager with the system manager using gRPC."""

    with grpc.insecure_channel(SYSTEM_MANAGER_GRPC_ADDR) as channel:
        stub = register_clusterStub(channel)

        # Send initial greeting (CS1Message)
        greeting = CS1Message()
        greeting.hello_service_manager = json.dumps(
            {"cluster_name": CONFIG.cluster_name, "location": CONFIG.cluster_location}
        )
        sc1: SC1Message = stub.handle_init_greeting(
            greeting, wait_for_ready=True, timeout=GRPC_REQUEST_TIMEOUT
        )
        logger.info(
            "Received greeting message from System Manager: " + str(sc1.hello_cluster_manager)
        )

        # Send cluster details (CS2Message)
        details = CS2Message()
        details.manager_port = CONFIG.port
        details.network_component_port = CONFIG.cluster_service_manager_port
        details.cluster_name = CONFIG.cluster_name
        details.cluster_location = CONFIG.cluster_location
        details.cluster_address = CONFIG.cluster_address
        details.cluster_info.append(KeyValue())

        sc2: SC2Message = stub.handle_init_final(
            details, wait_for_ready=True, timeout=GRPC_REQUEST_TIMEOUT
        )

        if not sc2.id:
            raise RuntimeError("Registration failed: no cluster id returned by root")

        assigned_cluster_id = sc2.id
        RUNTIME_CONFIG.assigned_cluster_id = assigned_cluster_id

        logger.info(f"Cluster ID received: {assigned_cluster_id}. Go ahead with Background Jobs")
        prometheus_init_gauge_metrics(assigned_cluster_id, app.logger)
        launch_background_jobs(assigned_cluster_id)


# ........... FINISH - register to System Manager with gRPC.................#
# ..........................................................................#

start_http_server(10001)  # start prometheus server


REGISTER_TIMEOUT_SECONDS = 10.0


def _wait_until_serving():
    """Waits until the server starts accepting TCP connections on loopback."""
    start_time = time.monotonic()
    while time.monotonic() - start_time < REGISTER_TIMEOUT_SECONDS:
        try:
            # socket.create_connection tries IPv6 (::1) and IPv4 (127.0.0.1) automatically
            with socket.create_connection(("localhost", CONFIG.port), timeout=0.2):
                logger.info(f"Server is listening and accepting traffic on port {CONFIG.port}")
                return True
        except (TimeoutError, OSError):
            pass
        time.sleep(0.05)

    raise TimeoutError(
        f"Server did not open port {CONFIG.port} within {REGISTER_TIMEOUT_SECONDS}s."
    )


def _register_in_background():
    # The root probes GET /api/cluster/status on this cluster_manager during
    # registration. That probe can only succeed once gunicorn's worker has
    # entered its accept loop, which doesn't happen until load_wsgi (i.e. this
    # module's top-level import) returns. So we MUST NOT block the import on
    # the gRPC call — otherwise the root's probe deadlocks against our own
    # startup. Give gunicorn a moment to start serving, then register. On
    # failure, exit the worker so gunicorn respawns it and tries again.
    try:
        _wait_until_serving()
        register_with_system_manager()
    except Exception:
        logger.exception("Cluster registration failed; exiting worker for restart")
        sys.exit(1)


threading.Thread(target=_register_in_background, daemon=True).start()

if __name__ == "__main__":
    import eventlet.wsgi

    eventlet.wsgi.server(
        eventlet.listen(("::", CONFIG.port), family=socket.AF_INET6), app, log=logger
    )  # see README for logging notes
