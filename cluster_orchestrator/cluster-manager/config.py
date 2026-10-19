import os

MY_PORT = os.environ.get("MY_PORT")

MY_CHOSEN_CLUSTER_NAME = os.environ.get("CLUSTER_NAME")
MY_CLUSTER_LOCATION = os.environ.get("CLUSTER_LOCATION")
MY_CLUSTER_ADDRESS = os.environ.get("CLUSTER_ADDRESS")
NETWORK_COMPONENT_PORT = os.environ.get("CLUSTER_SERVICE_MANAGER_PORT")
MY_ASSIGNED_CLUSTER_ID = None


SYSTEM_MANAGER_ADDR = (
    os.environ.get("SYSTEM_MANAGER_URL") + ":" + os.environ.get("SYSTEM_MANAGER_GRPC_PORT")
)
GRPC_REQUEST_TIMEOUT = 120

AGGREGATION_INTERVAL = int(os.environ.get("AGGREGATION_INTERVAL", 15))
# seconds; deploy command sent but no worker ACK yet
NODE_SCHEDULED_TIMEOUT = int(os.environ.get("NODE_SCHEDULED_TIMEOUT", 15))
