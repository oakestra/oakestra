import os
from typing import NamedTuple

MY_PORT = os.environ.get("MY_PORT")  # port on which cluster manager listens
# port communicated to root -> different if cluster is behind gateway
MY_CLUSTER_PORT = os.environ.get("CLUSTER_PORT") or MY_PORT
MY_CHOSEN_CLUSTER_NAME = os.environ.get("CLUSTER_NAME")
MY_CLUSTER_LOCATION = os.environ.get("CLUSTER_LOCATION")
MY_CLUSTER_ADDRESS = os.environ.get("CLUSTER_ADDRESS")
MY_ASSIGNED_CLUSTER_ID = None
NETWORK_COMPONENT_PORT = os.environ.get("CLUSTER_SERVICE_MANAGER_GATEWAY_PORT") or os.environ.get(
    "CLUSTER_SERVICE_MANAGER_PORT"
)


SYSTEM_MANAGER_ADDR = (
    (os.environ.get("SYSTEM_MANAGER_URL") or "")
    + ":"
    + (os.environ.get("SYSTEM_MANAGER_GRPC_PORT") or "")
)
GRPC_REQUEST_TIMEOUT = 120

# Set only by override-gateway.yml; gates the gateway-only certificate APIs.
GATEWAY_ENABLED = os.environ.get("GATEWAY_ENABLED", "").lower() in ("true", "1", "yes")
# CRLs are signed with a 30-day nextUpdate; they are re-signed this often.
CRL_REFRESH_INTERVAL_HOURS = float(os.environ.get("CRL_REFRESH_INTERVAL_HOURS") or 24)
# How often the cluster's mTLS client cert is checked and renewed when near expiry.
CERT_RENEW_CHECK_INTERVAL_HOURS = float(os.environ.get("CERT_RENEW_CHECK_INTERVAL_HOURS") or 24)
# How long workers may keep using certs from a replaced intermediate while they renew.
INTERMEDIATE_GRACE_PERIOD_HOURS = float(os.environ.get("INTERMEDIATE_GRACE_PERIOD_HOURS") or 24)
# Renew worker certificates this long before they expire.
WORKER_CERT_RENEW_DAYS = float(os.environ.get("WORKER_CERT_RENEW_DAYS") or 30)

# mTLS configuration. When all three files are present and SYSTEM_MANAGER_USE_TLS is truthy,
# cluster→root traffic (gRPC + REST) is wrapped in TLS with client-cert authentication.
SYSTEM_MANAGER_USE_TLS = os.environ.get("SYSTEM_MANAGER_USE_TLS", "").lower() in (
    "true",
    "1",
    "yes",
)
CLUSTER_CERT_FILE = os.environ.get("CLUSTER_CERT_FILE")
CLUSTER_KEY_FILE = os.environ.get("CLUSTER_KEY_FILE")
ROOT_CA_FILE = os.environ.get("ROOT_CA_FILE")


def mtls_enabled() -> bool:
    if not SYSTEM_MANAGER_USE_TLS:
        return False
    return all(
        path and os.path.isfile(path)
        for path in (CLUSTER_CERT_FILE, CLUSTER_KEY_FILE, ROOT_CA_FILE)
    )


# How the root gateway's *server* certificate is verified. Every consumer goes through
# parse_root_gateway_trust() so the same value always means the same thing:
#   "system"   -> the OS trust store (root gateway uses a publicly trusted BYO cert)
#   <path>     -> a CA bundle; a path inside the container, e.g. /certs/root-gateway-ca.crt
#   ""         -> the internal root CA (ROOT_CA_FILE)
#   "insecure" -> no verification; HTTPS only, the gRPC channel cannot skip it
ROOT_GATEWAY_TRUST = os.environ.get("ROOT_GATEWAY_TRUST", "")

TRUST_SYSTEM = "system"
TRUST_INSECURE = "insecure"
TRUST_CA_BUNDLE = "ca_bundle"


class GatewayTrust(NamedTuple):
    mode: str
    ca_file: str | None = None


def parse_root_gateway_trust(value: str, internal_ca_file: str | None) -> GatewayTrust:
    """Interpret a ROOT_GATEWAY_TRUST value. Raises ValueError if it cannot be used."""
    value = (value or "").strip()
    if value in (TRUST_SYSTEM, TRUST_INSECURE):
        return GatewayTrust(value)
    ca_file = value or internal_ca_file
    if not ca_file:
        raise ValueError("ROOT_GATEWAY_TRUST is empty and ROOT_CA_FILE is not set")
    if not os.path.isfile(ca_file):
        raise ValueError(
            f"ROOT_GATEWAY_TRUST CA bundle {ca_file!r} does not exist inside the container; "
            "use 'system', 'insecure', or a path in the mounted cert directory (e.g. /certs/)"
        )
    return GatewayTrust(TRUST_CA_BUNDLE, ca_file)


def root_gateway_trust() -> GatewayTrust:
    return parse_root_gateway_trust(ROOT_GATEWAY_TRUST, ROOT_CA_FILE)


def root_gateway_verify():
    """Value for requests' `verify=` when talking to the root gateway."""
    trust = root_gateway_trust()
    if trust.mode == TRUST_SYSTEM:
        return True
    if trust.mode == TRUST_INSECURE:
        return False
    return trust.ca_file


def root_gateway_grpc_root_certificates() -> bytes | None:
    """Trust anchors (PEM) for the gRPC channel to the root gateway; None means the OS store."""
    trust = root_gateway_trust()
    if trust.mode == TRUST_INSECURE:
        raise ValueError(
            "ROOT_GATEWAY_TRUST=insecure is not supported for the gRPC channel to the root "
            "(gRPC always verifies the server certificate); set 'system' or a CA bundle path"
        )
    if trust.mode == TRUST_SYSTEM:
        return None
    with open(trust.ca_file, "rb") as f:
        return f.read()


# Intermediate CA material. Cluster CA must be signed against root CA
CLUSTER_CA_CERT_FILE = os.environ.get("CLUSTER_CA_CERT_FILE")
CLUSTER_CA_KEY_FILE = os.environ.get("CLUSTER_CA_KEY_FILE")


def cluster_ca_enabled() -> bool:
    return all(
        path and os.path.isfile(path)
        for path in (CLUSTER_CA_CERT_FILE, CLUSTER_CA_KEY_FILE, ROOT_CA_FILE)
    )


MQTT_CONTAINER_NAME = os.environ.get("MQTT_CONTAINER_NAME", "mqtt")


def reload_mqtt() -> bool:
    """Send SIGHUP to the mosquitto broker so it reloads its TLS certificates.

    Mosquitto 2.0 re-reads cert files on SIGHUP without dropping connections.
    """
    try:
        import docker

        client = docker.from_env()
        container = client.containers.get(MQTT_CONTAINER_NAME)
        container.kill("HUP")
        return True
    except Exception as e:
        import logging

        logging.getLogger("cluster_manager").error("Could not reload MQTT broker: %s", e)
        return False


KONG_EXTERNAL_CONTAINER_NAME = os.environ.get(
    "KONG_EXTERNAL_CONTAINER_NAME", "cluster_kong_external"
)


def reload_kong_external() -> bool:
    """Reload the external gateway so it re-reads ca.crt, which verifies the root's calls."""
    try:
        import docker

        container = docker.from_env().containers.get(KONG_EXTERNAL_CONTAINER_NAME)
        result = container.exec_run("kong reload")
        if result.exit_code != 0:
            raise RuntimeError(result.output.decode("utf-8", errors="replace").strip())
        return True
    except Exception as e:
        import logging

        logging.getLogger("cluster_manager").error("Could not reload the external gateway: %s", e)
        return False


CLUSTER_SERVICE_MANAGER_CONTAINER_NAME = os.environ.get(
    "CLUSTER_SERVICE_MANAGER_CONTAINER_NAME", "cluster_service_manager"
)


def restart_cluster_service_manager() -> bool:
    """Restart cluster_service_manager, e.g. so it loads a new MQTT client certificate."""
    try:
        import docker

        docker.from_env().containers.get(CLUSTER_SERVICE_MANAGER_CONTAINER_NAME).restart()
        return True
    except Exception as e:
        import logging

        logging.getLogger("cluster_manager").error(
            "Could not restart cluster_service_manager: %s", e
        )
        return False


AGGREGATION_INTERVAL = int(os.environ.get("AGGREGATION_INTERVAL", 15))
# seconds; deploy command sent but no worker ACK yet
NODE_SCHEDULED_TIMEOUT = int(os.environ.get("NODE_SCHEDULED_TIMEOUT", 15))
