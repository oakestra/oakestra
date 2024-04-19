import json

import requests
from mongodb_client import find_one_edge_node


def edge_request():
    """deprecated
    because commands from CO to EDGE are going via MQTT instead of HTTP"""

    print("edge requesting...")
    edge_obj = find_one_edge_node()
    node_info = json.loads(edge_obj["node_info"])
    port_num = node_info.get("port")
    request_addr = "http://" + edge_obj["ip"] + ":" + str(port_num) + "/docker/start"
    print(request_addr)
    try:
        requests.get(request_addr)
    except requests.exceptions.RequestException:
        print("Calling Worker Node Endpoint not successful.")
