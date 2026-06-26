import json
import logging
import os
import socket
import threading
import time
from pathlib import Path

import config
import grpc
import requests as http_requests
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


def _build_grpc_channel():
    """Open the gRPC channel to system_manager.

    If mTLS is enabled and the cert bundle is present, the channel is wrapped in
    TLS with client-cert authentication. Otherwise falls back to insecure (today's
    behaviour for the no-gateway compose path).
    """
    if config.mtls_enabled():
        # Specify the CA to verify the root gateway's server cert
        # "system" -> OS trust store (root uses a publicly trusted BYO cert).
        # <path>   -> that CA bundle (privately issued cert).
        # ""       -> fall back to ROOT_CA_FILE (/certs/ca.crt), the default
        #             when the public cert is signed by the internal CA.
        # "insecure" has no effect here: gRPC always verifies the server cert.
        if config.ROOT_GATEWAY_TRUST == "system":
            ca_bytes = None
        else:
            trust_file = config.ROOT_GATEWAY_TRUST or config.ROOT_CA_FILE
            with open(trust_file, "rb") as f:
                ca_bytes = f.read()
        with open(config.CLUSTER_KEY_FILE, "rb") as f:
            key_bytes = f.read()
        with open(config.CLUSTER_CERT_FILE, "rb") as f:
            cert_bytes = f.read()
        creds = grpc.ssl_channel_credentials(
            root_certificates=ca_bytes,
            private_key=key_bytes,
            certificate_chain=cert_bytes,
        )
        logger.info("Opening secure gRPC channel to system_manager (mTLS)")
        return grpc.secure_channel(config.SYSTEM_MANAGER_ADDR, creds)
    logger.info("Opening insecure gRPC channel to system_manager")
    return grpc.insecure_channel(config.SYSTEM_MANAGER_ADDR)


def _refresh_cluster_certs() -> bool:
    """Re-bootstrap cluster cert material using CLUSTER_REGISTRATION_TOKEN.

    Called automatically when gRPC registration fails, so a stale cert bundle
    (e.g. after a root CA rotation) is replaced without manual intervention.
    Returns True if the cert files were successfully replaced.
    """
    token = os.environ.get("CLUSTER_REGISTRATION_TOKEN") or ""
    if not token:
        return False

    root_url = os.environ.get("SYSTEM_MANAGER_URL") or ""
    root_port = os.environ.get("SYSTEM_MANAGER_PORT") or "443"
    cluster_name = config.MY_CHOSEN_CLUSTER_NAME or ""
    cluster_ip = config.MY_CLUSTER_IP or ""

    if not root_url or not cluster_name:
        logger.error("Cannot refresh certs: SYSTEM_MANAGER_URL or CLUSTER_NAME not configured")
        return False

    base_url = f"https://{root_url}:{root_port}"
    alt_names = [name for name in (cluster_ip,) if name]

    logger.info("Attempting cert refresh from %s", base_url)
    try:
        resp = http_requests.post(
            f"{base_url}/api/certs/cluster-bootstrap",
            json={"token": token, "cluster_name": cluster_name, "alt_names": alt_names},
            verify=config.root_gateway_verify(),
            timeout=30,
        )
    except http_requests.exceptions.RequestException as e:
        logger.error("Cert refresh failed — could not reach root: %s", e)
        return False

    if resp.status_code == 401:
        logger.error("Cert refresh failed — token rejected (invalid, expired, or already used)")
        return False
    if resp.status_code != 200:
        logger.error(
            "Cert refresh failed — root returned %d: %s", resp.status_code, resp.text[:200]
        )
        return False

    payload = resp.json()
    cert_dir = Path(config.ROOT_CA_FILE).parent if config.ROOT_CA_FILE else Path("/certs")

    def _write(path, content, mode):
        p = Path(path)
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(content)
        os.chmod(p, mode)

    _write(config.ROOT_CA_FILE, payload["root_ca"], 0o644)
    _write(config.CLUSTER_CERT_FILE, payload["client_cert"], 0o644)
    _write(config.CLUSTER_KEY_FILE, payload["client_key"], 0o600)
    _write(config.CLUSTER_CA_CERT_FILE, payload["cluster_ca_cert"], 0o644)
    _write(config.CLUSTER_CA_KEY_FILE, payload["cluster_ca_key"], 0o600)

    logger.info("Cluster certificates refreshed in %s", cert_dir)
    if config.reload_mqtt():
        logger.info("MQTT broker reloaded with new certificates")
    else:
        logger.warning("Cert files updated but MQTT broker reload failed — restart mqtt manually")
    return True


def _try_register() -> bool:
    """Attempt a single gRPC registration. Returns True on success."""
    response = None
    with _build_grpc_channel() as channel:
        stub = register_clusterStub(channel)

        try:
            message = CS1Message()
            message.hello_service_manager = json.dumps(
                {
                    "cluster_name": config.MY_CHOSEN_CLUSTER_NAME,
                    "location": config.MY_CLUSTER_LOCATION,
                }
            )
            response: SC1Message = stub.handle_init_greeting(
                message, wait_for_ready=True, timeout=config.GRPC_REQUEST_TIMEOUT
            )
            logger.info(
                "Received greeting message from System Manager: "
                + str(response.hello_cluster_manager)
            )
        except grpc.RpcError as e:
            logger.error(f"Error sending CS1 to System Manager: {e}")
            return False

        try:
            message = CS2Message()
            message.manager_port = int(config.MY_CLUSTER_PORT)
            message.network_component_port = int(config.NETWORK_COMPONENT_PORT)
            message.cluster_name = config.MY_CHOSEN_CLUSTER_NAME
            message.cluster_location = config.MY_CLUSTER_LOCATION
            message.cluster_ip = config.MY_CLUSTER_IP

            key_value_message = KeyValue()
            message.cluster_info.append(key_value_message)

            response: SC2Message = stub.handle_init_final(
                message, wait_for_ready=True, timeout=config.GRPC_REQUEST_TIMEOUT
            )
            logger.info(f"Cluster ID received: {response.id}")
        except grpc.RpcError as e:
            logger.error(f"Error sending CS2 to System Manager: {e}")
            return False

    if response and response.id:
        config.MY_ASSIGNED_CLUSTER_ID = response.id
        logger.info("Received ID. Go ahead with Background Jobs")
        prometheus_init_gauge_metrics(config.MY_ASSIGNED_CLUSTER_ID, app.logger)
        background_job_send_aggregated_information_to_sm()
        return True

    logger.error("No ID received from System Manager.")
    return False


def register_with_system_manager():
    """Register with the system manager, refreshing certs and retrying once on failure."""
    if _try_register():
        return

    token = os.environ.get("CLUSTER_REGISTRATION_TOKEN") or ""
    if not token:
        logger.error(
            "gRPC registration failed and CLUSTER_REGISTRATION_TOKEN is not set — "
            "cannot attempt cert refresh. Set the token and restart."
        )
        return

    logger.warning("gRPC registration failed — refreshing cluster certificates with provided token")
    if _refresh_cluster_certs():
        logger.info("Certificates refreshed, retrying gRPC registration")
        _try_register()
    else:
        logger.error("Cert refresh failed — cluster is not registered")


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
