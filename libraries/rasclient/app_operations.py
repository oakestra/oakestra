from rasclient.client_helper import RESOURCE_ABSTRACTOR_ADDR, make_request
from requests import delete, get, patch, post

APPS_API = f"{RESOURCE_ABSTRACTOR_ADDR}/api/v1/apps"


def get_apps(**kwargs):
    return make_request(get, APPS_API, params=kwargs) or []


def get_user_apps(user_id, filter={}):
    request_address = f"{APPS_API}/{user_id}"
    return make_request(get, request_address, params=filter) or []


def get_app_by_name_and_namespace(app_name, app_ns, user_id, filter={}):
    filter = {**filter, "application_name": app_name, "application_namespace": app_ns}
    request_address = f"{APPS_API}/{user_id}/"
    return make_request(get, request_address, params=filter)


def get_app_by_id(app_id, user_id, filter={}):
    request_address = f"{APPS_API}/{user_id}/{app_id}"
    return make_request(get, request_address, params=filter)


def create_app(user_id, data):
    request_address = f"{APPS_API}/{user_id}"
    return make_request(post, request_address, json=data)


def update_app(app_id, user_id, data):
    request_address = f"{APPS_API}/{user_id}/{app_id}"
    return make_request(patch, request_address, json=data)


def delete_app(app_id, user_id):
    request_address = f"{APPS_API}/{user_id}/{app_id}"
    return make_request(delete, request_address)
