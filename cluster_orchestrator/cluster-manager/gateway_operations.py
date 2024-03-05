from bson import ObjectId
from gateway_db import (
    mongo_add_gateway_node,
    mongo_add_gateway_service,
    mongo_add_gateway_service_to_node,
    mongo_delete_gateway_service,
    mongo_delete_netmanager_client,
    mongo_find_available_gateway_by_port,
    mongo_find_available_idle_worker,
    mongo_register_netmanager_client,
)
from network_plugin_requests import network_notify_gateway_deploy, network_notify_gateway_update


def deploy_gateway(service):
    """
    Decision making on whether a new gateway is needed for instances of service
    otherwise just update the firewall of available gateway
    @param service: service data to expose
    @return message and http code
    """

    cluster_id = service["cluster_id"]
    del service["cluster_id"]

    # add service to collection of exposed services
    mongo_add_gateway_service(service)

    # get a deployed gateway
    # able to expose the requested service on the desired port
    gateway = mongo_find_available_gateway_by_port(service["exposed_port"])
    if gateway is None:
        # if no gateway available, deploy a new one
        # returns None if no gateway is available
        gateway = deploy_gateway_process_on_cluster(cluster_id)
        # if deployment impossible, raise exception to return 500
        if gateway is None:
            mongo_delete_gateway_service(service_id=service["microserviceID"])
            raise Exception("no gateways available")

    # update its firewall rules
    update_gateway_service_exposal(gateway, service)

    return gateway


# looks for idle node and tries to deploy gateway on it
# returns gateway data of deployed gateway
def deploy_gateway_process_on_cluster(cluster_id):
    # find idle netmanager
    idle_node = mongo_find_available_idle_worker()
    if idle_node is None:
        return None

    gateway_node_data = prepare_gateway_node_entry(idle_node, cluster_id)

    # notify cluster service-manager
    deployed_gateway, status = network_notify_gateway_deploy(gateway_node_data)
    # gw_job
    if status != 200:
        # TODO: cleanup
        return None

    # remove netmanager entry from table of netmanagers
    # and add to active gateways
    mongo_add_gateway_node(gateway_node_data)

    return deployed_gateway


def update_gateway_service_exposal(gateway, service):
    service_info = prepare_gateway_node_service_entry(service)
    mongo_add_gateway_service_to_node(gateway["gateway_id"], service_info)

    # send notification to service-manager
    network_notify_gateway_update(gateway, service_info)
    return


def handle_gateway_post(gateway_id, data):
    # TODO: handle_gateway_post
    return


def handle_gateway_put(gateway_id, data):
    # TODO: handle_gateway_put
    return


def handle_gateway_delete(gateway_id):
    # TODO: handle_gateway_delete
    return


def prepare_gateway_node_service_entry(service):
    service["_id"] = ObjectId(service["_id"])
    return service


# register a new netmanager to the cluster
def register_netmanager(node_data):
    net_id = mongo_register_netmanager_client(node_data)
    if net_id is None:
        raise Exception("could not register new netmanager")
    return {"id": net_id}


def delete_netmanager(node_id):
    mongo_delete_netmanager_client(node_id)


def prepare_gateway_node_entry(node_info, cluster_id):
    """
    formats struct to database structure
    @returns:
    {
        'gateway_id': string,
        'host': string,
        'host_port': int,
        'cluster_id': string,
        'gateway_ipv4': string,
        'gateway_ipv6': string,
        'used_ports': [],
        'services': []
    }
    """
    data = {}
    data["gateway_id"] = str(node_info["_id"])
    if node_info.get("ip") != "":
        data["gateway_ipv4"] = node_info["ip"]
    if node_info.get("ipv6") != "":
        data["gateway_ipv6"] = node_info["ipv6"]
    data["host"] = node_info["host"]
    # used for proxy tunneling
    data["host_port"] = node_info["port"]
    data["cluster_id"] = cluster_id
    # list of ports in use which cannot be used for gateway exposure
    data["used_ports"] = [node_info["port"]]
    # list of services this gateway exposes
    data["services"] = []
    return data
