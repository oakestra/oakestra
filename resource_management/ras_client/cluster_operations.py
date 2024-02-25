import os

from requests import exceptions, get, patch, put

RESOURCE_ABSTRACTOR_ADDR = (
    f"http://{os.environ.get('RESOURCE_ABSTRACTOR_URL')}:"
    f"{os.environ.get('RESOURCE_ABSTRACTOR_PORT')}"
)


def get_resources(**kwargs):
    request_address = RESOURCE_ABSTRACTOR_ADDR + "/api/v1/resources"
    try:
        response = get(request_address, params=kwargs)
        return response.json()
    except exceptions.RequestException:
        print("Calling Resource Abstractor /api/v1/resources not successful.")

    return []


def get_resource_by_id(resource_id):
    request_address = RESOURCE_ABSTRACTOR_ADDR + f"/api/v1/resources/{resource_id}"
    try:
        response = get(request_address)
        # TODO check body not empty.
        return response.json()
    except exceptions.RequestException:
        print(f"Calling Resource Abstractor /api/v1/resources/{resource_id} not successful.")

    return None


def get_resource_by_name(resource_name):
    resources = get_resources(cluster_name=resource_name)
    if resources is None or len(resources) == 0:
        return None

    return resources[0]


def get_resource_by_job_id(job_id):
    resources = get_resources(job_id=job_id)
    if resources is None or len(resources) == 0:
        return None

    return resources[0]


def get_resource_by_ip(ip):
    resources = get_resources(ip=ip)
    if resources is None or len(resources) == 0:
        return None

    return resources[0]


def update_cluster_information(cluster_id, data):
    request_address = f"{RESOURCE_ABSTRACTOR_ADDR}/api/v1/resources/{cluster_id}"
    try:
        response = patch(request_address, data)
        return response.json()
    except exceptions.RequestException:
        print(f"Calling Resource Abstractor /api/v1/resources/{cluster_id} not successful.")

    return None


def create_cluster(data):
    request_address = f"{RESOURCE_ABSTRACTOR_ADDR}/api/v1/resources"
    try:
        response = put(request_address, data)
        return response.json()
    except exceptions.RequestException:
        print("Calling Resource Abstractor /api/v1/resources not successful.")

    return None
