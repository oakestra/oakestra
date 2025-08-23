from requests import get, patch, put

from resource_abstractor_client.client_helper import make_request

RESOURCES_API = "/api/v1/resources"


def get_candidates(**kwargs):
    return make_request(get, RESOURCES_API, params=kwargs)


def get_candidate_by_id(candidate_id):
    request_address = f"{RESOURCES_API}/{candidate_id}"
    return make_request(get, request_address)


def get_candidate_by_name(candidate_name):
    candidates = get_candidates(cluster_name=candidate_name)
    return candidates[0] if candidates else None


def get_candidate_by_ip(ip):
    candidates = get_candidates(ip=ip)
    return candidates[0] if candidates else None


def update_candidate_information(candidate_id, data):
    request_address = f"{RESOURCES_API}/{candidate_id}"
    return make_request(patch, request_address, json=data)


def create_candidate(data):
    return make_request(put, RESOURCES_API, json=data)
