from resource_abstractor_client.client_helper import make_request, translate_kwargs
from resource_abstractor_client.openapi.api.resources import (
    get_resource,
    list_resources,
    patch_resource,
    upsert_resource,
)
from resource_abstractor_client.openapi.models.resource import Resource

# Wire query parameter name -> the generated _get_kwargs()'s Python parameter name.
# See client_helper.translate_kwargs. All five already match 1:1 - none of the
# resources filters are mixedCase - but the map still exists so an unsupported filter
# is dropped with a warning instead of silently vanishing on the wire.
_LIST_PARAMS = {
    "active": "active",
    "job_id": "job_id",
    "candidate_name": "candidate_name",
    "ip": "ip",
    "resources": "resources",
}


def get_candidates(**kwargs):
    # `resources` is style: form, explode: false in the spec - a single comma-joined
    # value, not the key repeated once per field. The server's query binder
    # (oapi-codegen's runtime.BindQueryParameterWithOptions) enforces that literally:
    # ?resources=a,b succeeds, ?resources=a&resources=b is a 400 Bad Request. httpx's
    # own default encoding of a Python list is the repeated form, so a list has to be
    # joined here before it reaches the generated request builder.
    resources = kwargs.get("resources")
    if isinstance(resources, (list, tuple)):
        kwargs["resources"] = ",".join(resources)
    params = translate_kwargs(kwargs, _LIST_PARAMS)
    return make_request(list_resources._get_kwargs(**params))


def get_candidate_by_id(candidate_id):
    return make_request(get_resource._get_kwargs(candidate_id))


def get_candidate_by_name(candidate_name):
    candidates = get_candidates(candidate_name=candidate_name)
    return candidates[0] if candidates else None


def get_candidate_by_ip(ip):
    candidates = get_candidates(ip=ip)
    return candidates[0] if candidates else None


def update_candidate_information(candidate_id, data):
    return make_request(patch_resource._get_kwargs(candidate_id, body=Resource.from_dict(data)))


def create_candidate(data):
    return make_request(upsert_resource._get_kwargs(body=Resource.from_dict(data)))
