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

if config.mtls_enabled():
    # Validate ROOT_GATEWAY_TRUST
    try:
        config.root_gateway_grpc_root_certificates()
    except (ValueError, OSError) as exc:
        logger.critical("Invalid ROOT_GATEWAY_TRUST: %s", exc)
        raise

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
        seconds=config.AGGREGATION_INTERVAL,
        kwargs={
            "my_id": config.MY_ASSIGNED_CLUSTER_ID,
            "running_timeout": 2 * config.AGGREGATION_INTERVAL,
            "node_scheduled_timeout": config.NODE_SCHEDULED_TIMEOUT,
        },
    )
    # job_re_deploy_dead_jobs
    scheduler.add_job(re_deploy_dead_jobs_routine, "interval", seconds=config.AGGREGATION_INTERVAL)

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
        # Load bootstrap certificates to build secure grpc channel
        ca_bytes = config.root_gateway_grpc_root_certificates()
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
            message.cluster_address = config.MY_CLUSTER_ADDRESS

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
    """Register with the system manager over grpc.

    In gateway mode the channel uses mTLS. A cluster whose certificates were
    rejected is re-registered by setting a new CLUSTER_REGISTRATION_TOKEN, which
    cluster_cert_bootstrap redeems before this process starts.
    """
    if not config.MY_CLUSTER_ADDRESS:
        raise RuntimeError(
            "CLUSTER_ADDRESS env var is not set. Set CLUSTER_ADDRESS to "
            "IP/hostname from which this host is reachable by the root."
        )

    if not _try_register():
        raise RuntimeError("gRPC registration with the System Manager failed")


# ........... FINISH - register to System Manager with gRPC.................#
# ..........................................................................#

start_http_server(10001)  # start prometheus server


_CERT_WARN_DAYS = 30


def _check_cluster_cert_expiry():
    """Renew the mTLS client cert in-band when it is near expiry."""
    if not config.mtls_enabled():
        return
    if not config.CLUSTER_CERT_FILE:
        return
    try:
        from pathlib import Path

        from blueprints.certificates_blueprints import _renew_cluster_certs_in_band
        from ext_requests.cluster_certificates import cert_expires_in

        pem = Path(config.CLUSTER_CERT_FILE).read_text()
        remaining = cert_expires_in(pem)
        days = remaining.days
        if remaining.total_seconds() <= 0:
            logger.error(
                "Cluster mTLS client certificate has EXPIRED. "
                "Re-register the cluster by setting a new CLUSTER_REGISTRATION_TOKEN."
            )
        elif days <= _CERT_WARN_DAYS:
            logger.warning(
                "Cluster mTLS client certificate expires in %d day(s). "
                "Certificates will be refreshed automatically.",
                days,
            )
            success, message = _renew_cluster_certs_in_band()
            if success:
                logger.info("Automatic cluster certificate renewal succeeded: %s", message)
            else:
                logger.error(
                    "Automatic cluster certificate renewal FAILED: %s. "
                    "Continuing with the current certificates (expire in %d day(s)).",
                    message,
                    days,
                )
    except Exception as exc:
        logger.error(
            "Cluster certificate expiry check/renewal failed: %s. "
            "Continuing with the current certificates.",
            exc,
        )
    _warn_if_intermediate_expiring()
    _check_mqtt_server_cert_expiry()
    _check_cluster_mqtt_identity_expiry()


def _check_mqtt_server_cert_expiry():
    """Re-issue mosquitto's server cert from the intermediate when near expiry.

    mosquitto reloads its server certificate on SIGHUP without dropping connections.
    Skipped while workers migrate to a new intermediate; the migration's end re-issues it.
    """
    try:
        from ext_requests.cluster_certificates import (
            cert_expires_in,
            get_mqtt_server_identity_paths,
            get_old_cluster_ca_paths,
            write_mqtt_server_identity,
        )

        if get_old_cluster_ca_paths()[0].is_file():
            return
        cert_path, _ = get_mqtt_server_identity_paths()
        days = cert_expires_in(cert_path.read_text()).days
        if days > _CERT_WARN_DAYS:
            return
        logger.warning("MQTT broker server certificate expires in %d day(s); re-issuing it.", days)
        write_mqtt_server_identity(config.MY_CHOSEN_CLUSTER_NAME, config.MY_CLUSTER_ADDRESS)
    except Exception as exc:
        logger.error("Could not re-issue the MQTT broker server certificate: %s", exc)
        return
    config.reload_mqtt()


# Replacing the intermediate CA means every worker must re-bootstrap, so warn early.
_INTERMEDIATE_WARN_DAYS = 90


def _warn_if_intermediate_expiring():
    try:
        from pathlib import Path

        from ext_requests.cluster_certificates import cert_expires_in

        days = cert_expires_in(Path(config.CLUSTER_CA_CERT_FILE).read_text()).days
    except Exception as exc:
        logger.error("Could not check the cluster intermediate CA expiry: %s", exc)
        return
    if days < 0:
        logger.error(
            "The cluster intermediate CA has EXPIRED — workers cannot connect to MQTT. "
            "Replace it with POST /api/certs/rotate, then re-bootstrap every worker."
        )
    elif days <= _INTERMEDIATE_WARN_DAYS:
        logger.warning(
            "The cluster intermediate CA expires in %d day(s). Replace it with "
            "POST /api/certs/rotate; workers then migrate to it automatically.",
            days,
        )


def _check_cluster_mqtt_identity_expiry():
    """Re-issue the cluster's own MQTT client cert from the intermediate when near expiry."""
    try:
        from ext_requests.cluster_certificates import (
            cert_expires_in,
            get_cluster_mqtt_identity_paths,
            write_cluster_mqtt_identity,
        )

        cert_path, _ = get_cluster_mqtt_identity_paths()
        days = cert_expires_in(cert_path.read_text()).days
        if days > _CERT_WARN_DAYS:
            return
        logger.warning(
            "Cluster MQTT client certificate expires in %d day(s); re-issuing it and "
            "restarting the MQTT clients.",
            days,
        )
        write_cluster_mqtt_identity(config.MY_CHOSEN_CLUSTER_NAME)
    except Exception as exc:
        logger.error("Could not re-issue the cluster MQTT client certificate: %s", exc)
        return

    config.restart_cluster_service_manager()
    logger.warning("Restarting cluster_manager to load the new MQTT client certificate.")
    os._exit(1)


def _check_cluster_cert_expiry_periodically():
    while True:
        time.sleep(config.CERT_RENEW_CHECK_INTERVAL_HOURS * 3600)
        _check_cluster_cert_expiry()


def _register_in_background():
    # Wait until cluster manager has completed startup.
    time.sleep(2)
    try:
        _check_cluster_cert_expiry()
        register_with_system_manager()
    except Exception:
        logger.exception("Cluster registration failed; exiting worker for restart")
        os._exit(1)


def _refresh_crl_periodically():
    from blueprints.certificates_blueprints import refresh_cluster_crl

    delay = 60
    while True:
        time.sleep(delay)
        delay = config.CRL_REFRESH_INTERVAL_HOURS * 3600
        try:
            if not refresh_cluster_crl()["crl_regenerated"]:
                logger.error("Periodic CRL refresh could not write the cluster CRL")
        except Exception:
            logger.exception("Periodic CRL refresh failed")


def _end_expired_worker_migration_periodically():
    from blueprints.certificates_blueprints import end_expired_worker_migration

    while True:
        try:
            end_expired_worker_migration()
        except Exception:
            logger.exception("Checking the worker migration deadline failed")
        time.sleep(300)


if config.GATEWAY_ENABLED:
    threading.Thread(target=_refresh_crl_periodically, daemon=True).start()
    threading.Thread(target=_check_cluster_cert_expiry_periodically, daemon=True).start()
    threading.Thread(target=_end_expired_worker_migration_periodically, daemon=True).start()

threading.Thread(target=_register_in_background, daemon=True).start()

if __name__ == "__main__":
    import eventlet

    eventlet.wsgi.server(
        eventlet.listen(("::", int(config.MY_PORT)), family=socket.AF_INET6), app, log=my_logger
    )  # see README for logging notes
