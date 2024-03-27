from requests import get, patch, put
from resource_abstractor_client.client_helper import make_request

RESOURCES_API = "/api/v1/resources"


def get_resources(**kwargs):
    return make_request(get, RESOURCES_API, params=kwargs)


def get_resource_by_id(resource_id):
    request_address = f"{RESOURCES_API}/{resource_id}"
    return make_request(get, request_address)


def get_resource_by_name(resource_name):
    resources = get_resources(cluster_name=resource_name)
    return resources[0] if resources else None


def get_resource_by_ip(ip):
    resources = get_resources(ip=ip)
    return resources[0] if resources else None


def update_cluster_information(cluster_id, data):
    request_address = f"{RESOURCES_API}/{cluster_id}"
    return make_request(patch, request_address, json=data)


def create_cluster(data):
    return make_request(put, RESOURCES_API, json=data)
