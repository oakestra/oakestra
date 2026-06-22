import os

MY_PORT = os.environ.get("MY_PORT")  # port on which cluster manager listens
MY_CLUSTER_PORT = os.environ.get(
    "CLUSTER_PORT"
)  # port communicated to root -> different if cluster is behing gateway
MY_CHOSEN_CLUSTER_NAME = os.environ.get("CLUSTER_NAME")
MY_CLUSTER_LOCATION = os.environ.get("CLUSTER_LOCATION")
MY_CLUSTER_IP = os.environ.get("CLUSTER_IP") or ""
NETWORK_COMPONENT_PORT = os.environ.get("CLUSTER_SERVICE_MANAGER_GATEWAY_PORT") or os.environ.get(
    "CLUSTER_SERVICE_MANAGER_PORT"
)


# `or ""` keeps this module importable in contexts where the gRPC env vars
# are absent (e.g. the cluster_cert_bootstrap one-shot container).
SYSTEM_MANAGER_ADDR = (os.environ.get("SYSTEM_MANAGER_URL") or "") + ":" + (
    os.environ.get("SYSTEM_MANAGER_GRPC_PORT") or ""
)
GRPC_REQUEST_TIMEOUT = 120

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


# What to verify the root gateway's *server* certificate against:
#   ""       -> the internal root CA file (default; works with the fallback
#               gateway cert, which is signed by the internal CA)
#   "system" -> the system trust store (root uses a BYO public cert)
#   <path>   -> a custom CA bundle
ROOT_GATEWAY_TRUST = os.environ.get("ROOT_GATEWAY_TRUST", "")


def root_gateway_verify():
    """Value for requests' `verify=` when talking to the root gateway."""
    if ROOT_GATEWAY_TRUST == "system":
        return True
    return ROOT_GATEWAY_TRUST or ROOT_CA_FILE


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
