import os

import requests

CLUSTER_MANAGER_ADDR = (
    "http://"
    + os.environ.get("CLUSTER_MANAGER_URL")
    + ":"
    + str(os.environ.get("CLUSTER_MANAGER_PORT"))
)


def manager_request(app, node, job, job_id, instance_num):
    print("manager request")
    app.logger.info("sending scheduling result to cluster-manager...")
    request_addr = CLUSTER_MANAGER_ADDR + "/api/result/" + job_id + "/" + instance_num
    app.logger.info(request_addr)

    try:
        if node is not None:
            node_id = str(node.get("_id"))  # change ObjectID to string to send it via JSON
            node.__setitem__("_id", node_id)
            node.__delitem__(
                "last_modified"
            )  # delete last_modified of the node because it is not serializable
            requests.post(request_addr, json={"node": node, "job": job, "found": True})
        else:
            requests.post(request_addr, json={"found": False})
    except requests.exceptions.RequestException:
        print("Calling Cluster Manager /api/result not successful.")
