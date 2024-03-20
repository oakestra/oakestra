from requests import delete, get, patch, post
from resource_abstractor_client.client_helper import make_request

APPS_API = "/api/v1/applications"


def get_apps(**kwargs):
    return make_request(get, APPS_API, params=kwargs)


def get_user_apps(user_id, filter={}):
    filter = {**filter, "userId": user_id}
    return make_request(get, APPS_API, params=filter)


def get_app_by_name_and_namespace(app_name, app_ns, user_id, filter={}):
    filter = {
        **filter,
        "userId": user_id,
        "application_name": app_name,
        "application_namespace": app_ns,
    }
    result = make_request(get, APPS_API, params=filter)
    return result[0] if result else None


def get_app_by_id(app_id, user_id, filter={}):
    filter = {**filter, "userId": user_id}
    request_address = f"{APPS_API}/{app_id}"
    return make_request(get, request_address, params=filter)


def create_app(user_id, data):
    data["userId"] = user_id
    return make_request(post, APPS_API, json=data)


def update_app(app_id, user_id, data):
    request_address = f"{APPS_API}/{app_id}"
    data["userId"] = user_id
    return make_request(patch, request_address, json=data)


def delete_app(app_id):
    request_address = f"{APPS_API}/{app_id}"
    return make_request(delete, request_address)
