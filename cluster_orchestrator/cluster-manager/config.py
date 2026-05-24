import os

MY_PORT = os.environ.get("MY_PORT") # port on which cluster manager listens
MY_CLUSTER_PORT = os.environ.get("CLUSTER_PORT") # port communicated to root -> different if cluster is behing gateway
MY_CHOSEN_CLUSTER_NAME = os.environ.get("CLUSTER_NAME")
MY_CLUSTER_LOCATION = os.environ.get("CLUSTER_LOCATION")
MY_CLUSTER_IP = os.environ.get("CLUSTER_IP") or ""
NETWORK_COMPONENT_PORT = os.environ.get("CLUSTER_SERVICE_MANAGER_GATEWAY_PORT") or os.environ.get("CLUSTER_SERVICE_MANAGER_PORT")


SYSTEM_MANAGER_ADDR = (
    os.environ.get("SYSTEM_MANAGER_URL") + ":" + os.environ.get("SYSTEM_MANAGER_GRPC_PORT")
)
GRPC_REQUEST_TIMEOUT = 120

# mTLS configuration. When all three files are present and SYSTEM_MANAGER_USE_TLS is truthy,
# cluster→root traffic (gRPC + REST) is wrapped in TLS with client-cert authentication.
SYSTEM_MANAGER_USE_TLS = os.environ.get("SYSTEM_MANAGER_USE_TLS", "").lower() in ("true", "1", "yes")
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
