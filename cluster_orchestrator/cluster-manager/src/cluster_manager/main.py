import json
import logging
import os
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
from oakestra_logging import configure_logging, get_logger
from prometheus_client import start_http_server

from . import gateway_config
from .app_config import (
    ADVERTISED_PORT,
    ADVERTISED_SERVICE_MANAGER_PORT,
    CONFIG,
    GRPC_REQUEST_TIMEOUT,
    RUNTIME_CONFIG,
    SYSTEM_MANAGER_GRPC_ADDR,
)
from .blueprints import blueprints
from .clients.mqtt_client import initialize_mqtt
from .clients.my_prometheus_client import prometheus_init_gauge_metrics
from .ext_requests.network_manager_requests import network_notify_cluster_id
from .ext_requests.system_manager_requests import (
    re_deploy_dead_jobs_routine,
    send_aggregated_info_to_sm,
)
from .proto.cluster_registration_pb2 import CS1Message, CS2Message, SC1Message, SC2Message
from .proto.cluster_registration_pb2_grpc import register_clusterStub

SWAGGER_URL = "/api/docs"
API_URL = "/docs/openapi.json"

configure_logging(os.getenv("OAKESTRA_SERVICE_NAME", "cluster_manager"))
logger = get_logger(__name__)
stdlib_logger = logging.getLogger("cluster_manager")

if gateway_config.mtls_enabled():
    # Validate ROOT_GATEWAY_TRUST
    try:
        gateway_config.root_gateway_grpc_root_certificates()
    except (ValueError, OSError):
        logger.critical(
            "Invalid ROOT_GATEWAY_TRUST",
            event_name="cluster.gateway.invalid_trust",
            exc_info=True,
        )
        raise

app = Flask(__name__)

app.config["OPENAPI_VERSION"] = "3.0.2"
app.config["API_TITLE"] = "Oakestra root api"
app.config["API_VERSION"] = "v1"
app.config["OPENAPI_URL_PREFIX"] = "/docs"
app.config["JWT_ALGORITHM"] = "RS256"
app.logger = stdlib_logger

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
    logger.info(
        "Setting up cluster background jobs",
        event_name="cluster.background_jobs.starting",
        cluster_id=assigned_cluster_id,
    )
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


def _build_grpc_channel():
    """Open the gRPC channel to system_manager.

    If mTLS is enabled and the cert bundle is present, the channel is wrapped in
    TLS with client-cert authentication. Otherwise falls back to insecure (today's
    behaviour for the no-gateway compose path).
    """
    if gateway_config.mtls_enabled():
        # Load bootstrap certificates to build secure grpc channel
        ca_bytes = gateway_config.root_gateway_grpc_root_certificates()
        with open(gateway_config.CLUSTER_KEY_FILE, "rb") as f:
            key_bytes = f.read()
        with open(gateway_config.CLUSTER_CERT_FILE, "rb") as f:
            cert_bytes = f.read()
        creds = grpc.ssl_channel_credentials(
            root_certificates=ca_bytes,
            private_key=key_bytes,
            certificate_chain=cert_bytes,
        )
        logger.info(
            "Opening mTLS gRPC channel to System Manager",
            event_name="cluster.registration.mtls_channel",
        )
        return grpc.secure_channel(SYSTEM_MANAGER_GRPC_ADDR, creds)
    return grpc.insecure_channel(SYSTEM_MANAGER_GRPC_ADDR)


def register_with_system_manager():
    """Registers this cluster manager with the system manager using gRPC."""

    with _build_grpc_channel() as channel:
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
            "Received greeting from System Manager",
            event_name="cluster.registration.greeting_received",
            greeting_size=len(sc1.hello_cluster_manager),
        )

        # Send cluster details (CS2Message)
        details = CS2Message()
        details.manager_port = ADVERTISED_PORT
        details.network_component_port = ADVERTISED_SERVICE_MANAGER_PORT
        details.cluster_name = CONFIG.cluster_name
        details.cluster_location = CONFIG.cluster_location
        details.cluster_address = CONFIG.cluster_address

        sc2: SC2Message = stub.handle_init_final(
            details, wait_for_ready=True, timeout=GRPC_REQUEST_TIMEOUT
        )

        assigned_cluster_id = sc2.id
        if not assigned_cluster_id:
            raise RuntimeError("Registration failed: no cluster id returned by root")

        if gateway_config.GATEWAY_ENABLED:
            from .utils.certificates import own_cluster_id

            # Behind the gateway the root registers the cluster under its certificate's id.
            if assigned_cluster_id != own_cluster_id():
                raise RuntimeError(
                    f"System Manager assigned id {assigned_cluster_id}, but this cluster's "
                    f"certificate is for {own_cluster_id()}"
                )

        RUNTIME_CONFIG.assigned_cluster_id = assigned_cluster_id
        # The service manager names the cluster by this id when calling the root.
        network_notify_cluster_id(assigned_cluster_id)

        logger.info(
            "Cluster registration completed",
            event_name="cluster.registration.completed",
            cluster_id=assigned_cluster_id,
        )
        prometheus_init_gauge_metrics(assigned_cluster_id)
        launch_background_jobs(assigned_cluster_id)


# ........... FINISH - register to System Manager with gRPC.................#
# ..........................................................................#

start_http_server(10001)  # start prometheus server


_CERT_WARN_DAYS = 30


def _check_cluster_cert_expiry():
    """Renew the mTLS client cert in-band when it is near expiry."""
    if not gateway_config.mtls_enabled():
        return
    if not gateway_config.CLUSTER_CERT_FILE:
        return
    try:
        from pathlib import Path

        from .blueprints.certificates_blueprints import _renew_cluster_certs_in_band
        from .utils.certificates import cert_expires_in

        pem = Path(gateway_config.CLUSTER_CERT_FILE).read_text()
        remaining = cert_expires_in(pem)
        days = remaining.days
        if remaining.total_seconds() <= 0:
            logger.error(
                "Cluster mTLS client certificate has EXPIRED. "
                "Re-register the cluster by setting a new CLUSTER_REGISTRATION_TOKEN.",
                event_name="certificates.cluster_cert.expired",
            )
        elif days <= _CERT_WARN_DAYS:
            logger.warning(
                "Cluster mTLS client certificate expires soon; refreshing it automatically",
                event_name="certificates.cluster_cert.expiring",
                days_remaining=days,
            )
            success, message = _renew_cluster_certs_in_band()
            if success:
                logger.info(
                    "Automatic cluster certificate renewal succeeded",
                    event_name="certificates.cluster_cert.renewed",
                    detail=message,
                )
            else:
                logger.error(
                    "Automatic cluster certificate renewal failed; "
                    "continuing with the current certificates",
                    event_name="certificates.cluster_cert.renewal_failed",
                    detail=message,
                    days_remaining=days,
                )
    except Exception:
        logger.exception(
            "Cluster certificate expiry check/renewal failed; "
            "continuing with the current certificates",
            event_name="certificates.cluster_cert.check_failed",
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
        from .utils.certificates import (
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
        logger.warning(
            "MQTT broker server certificate expires soon; re-issuing it",
            event_name="certificates.mqtt_server_cert.expiring",
            days_remaining=days,
        )
        # mosquitto reloads itself when the file changes.
        write_mqtt_server_identity(CONFIG.cluster_address)
    except Exception:
        logger.exception(
            "Could not re-issue the MQTT broker server certificate",
            event_name="certificates.mqtt_server_cert.reissue_failed",
        )


# Replacing the intermediate CA means every worker must re-bootstrap, so warn early.
_INTERMEDIATE_WARN_DAYS = 90


def _warn_if_intermediate_expiring():
    try:
        from pathlib import Path

        from .utils.certificates import cert_expires_in

        days = cert_expires_in(Path(gateway_config.CLUSTER_CA_CERT_FILE).read_text()).days
    except Exception:
        logger.exception(
            "Could not check the cluster intermediate CA expiry",
            event_name="certificates.intermediate_ca.check_failed",
        )
        return
    if days < 0:
        logger.error(
            "The cluster intermediate CA has EXPIRED — workers cannot connect to MQTT. "
            "Replace it with POST /api/certs/rotate, then re-bootstrap every worker.",
            event_name="certificates.intermediate_ca.expired",
        )
    elif days <= _INTERMEDIATE_WARN_DAYS:
        logger.warning(
            "The cluster intermediate CA expires soon. Replace it with "
            "POST /api/certs/rotate; workers then migrate to it automatically.",
            event_name="certificates.intermediate_ca.expiring",
            days_remaining=days,
        )


def _check_cluster_mqtt_identity_expiry():
    """Re-issue the cluster's own MQTT client cert from the intermediate when near expiry."""
    try:
        from .utils.certificates import (
            cert_expires_in,
            get_cluster_mqtt_identity_paths,
            write_cluster_mqtt_identity,
        )

        cert_path, _ = get_cluster_mqtt_identity_paths()
        days = cert_expires_in(cert_path.read_text()).days
        if days > _CERT_WARN_DAYS:
            return
        logger.warning(
            "Cluster MQTT client certificate expires soon; re-issuing it and "
            "restarting the MQTT clients",
            event_name="certificates.cluster_mqtt_cert.expiring",
            days_remaining=days,
        )
        write_cluster_mqtt_identity()
    except Exception:
        logger.exception(
            "Could not re-issue the cluster MQTT client certificate",
            event_name="certificates.cluster_mqtt_cert.reissue_failed",
        )
        return

    gateway_config.restart_cluster_service_manager()
    logger.warning(
        "Restarting cluster_manager to load the new MQTT client certificate",
        event_name="certificates.cluster_mqtt_cert.restarting",
    )
    os._exit(1)


def _check_cluster_cert_expiry_periodically():
    while True:
        time.sleep(gateway_config.CERT_RENEW_CHECK_INTERVAL_HOURS * 3600)
        _check_cluster_cert_expiry()


def _register_in_background():
    # The root probes GET /api/cluster/status on this cluster_manager during
    # registration. That probe can only succeed once gunicorn's worker has
    # entered its accept loop, which doesn't happen until load_wsgi (i.e. this
    # module's top-level import) returns. So we MUST NOT block the import on
    # the gRPC call — otherwise the root's probe deadlocks against our own
    # startup. Give gunicorn a moment to start serving, then register. On
    # failure, exit the worker so gunicorn respawns it and tries again
    time.sleep(2)
    try:
        _check_cluster_cert_expiry()
        register_with_system_manager()
    except Exception:
        logger.exception(
            "Cluster registration failed; exiting worker for restart",
            event_name="cluster.registration.unrecoverable",
        )
        sys.exit(1)


def _refresh_crl_periodically():
    from .blueprints.certificates_blueprints import refresh_cluster_crl

    delay = 60
    while True:
        time.sleep(delay)
        delay = gateway_config.CRL_REFRESH_INTERVAL_HOURS * 3600
        try:
            if not refresh_cluster_crl()["crl_regenerated"]:
                logger.error(
                    "Periodic CRL refresh could not write the cluster CRL",
                    event_name="certificates.crl.write_failed",
                )
        except Exception:
            logger.exception(
                "Periodic CRL refresh failed", event_name="certificates.crl.refresh_failed"
            )


def _end_expired_worker_migration_periodically():
    from .blueprints.certificates_blueprints import end_expired_worker_migration

    while True:
        try:
            end_expired_worker_migration()
        except Exception:
            logger.exception(
                "Checking the worker migration deadline failed",
                event_name="certificates.worker_migration.check_failed",
            )
        time.sleep(300)


if gateway_config.GATEWAY_ENABLED:
    threading.Thread(target=_refresh_crl_periodically, daemon=True).start()
    threading.Thread(target=_check_cluster_cert_expiry_periodically, daemon=True).start()
    threading.Thread(target=_end_expired_worker_migration_periodically, daemon=True).start()

threading.Thread(target=_register_in_background, daemon=True).start()

if __name__ == "__main__":
    import eventlet.wsgi

    eventlet.wsgi.server(
        eventlet.listen(("::", CONFIG.port), family=socket.AF_INET6), app, log=stdlib_logger
    )  # see README for logging notes
