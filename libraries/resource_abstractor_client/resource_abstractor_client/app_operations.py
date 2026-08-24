from resource_abstractor_client.client_helper import make_request, translate_kwargs
from resource_abstractor_client.openapi.api.applications import (
    create_application,
    delete_application,
    get_application,
    list_applications,
    patch_application,
)
from resource_abstractor_client.openapi.models.application import Application

# Wire query parameter name -> the generated _get_kwargs()'s Python parameter name.
# See client_helper.translate_kwargs.
_LIST_PARAMS = {
    "application_name": "application_name",
    "application_namespace": "application_namespace",
    "userId": "user_id",
}


def get_apps(**kwargs):
    params = translate_kwargs(kwargs, _LIST_PARAMS)
    return make_request(list_applications._get_kwargs(**params))


def get_user_apps(user_id, filter={}):
    params = translate_kwargs(filter, _LIST_PARAMS)
    params["user_id"] = user_id
    return make_request(list_applications._get_kwargs(**params))


def get_app_by_name_and_namespace(app_name, app_ns, user_id, filter={}):
    params = translate_kwargs(filter, _LIST_PARAMS)
    params.update(user_id=user_id, application_name=app_name, application_namespace=app_ns)
    result = make_request(list_applications._get_kwargs(**params))
    return result[0] if result else None


def get_app_by_id(app_id, user_id, filter={}):
    params = translate_kwargs(filter, _LIST_PARAMS)
    params["user_id"] = user_id
    return make_request(get_application._get_kwargs(app_id, **params))


def create_app(user_id, data):
    data["userId"] = user_id
    return make_request(create_application._get_kwargs(body=Application.from_dict(data)))


def update_app(app_id, user_id, data):
    data["userId"] = user_id
    return make_request(patch_application._get_kwargs(app_id, body=Application.from_dict(data)))


def delete_app(app_id):
    return make_request(delete_application._get_kwargs(app_id))
