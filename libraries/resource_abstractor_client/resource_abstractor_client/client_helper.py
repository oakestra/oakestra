import json
import logging
import os
from typing import Any, Optional

import httpx

from resource_abstractor_client.openapi.client import Client

RESOURCE_ABSTRACTOR_ADDR = (
    f"http://{os.environ.get('RESOURCE_ABSTRACTOR_URL')}:"
    f"{os.environ.get('RESOURCE_ABSTRACTOR_PORT')}"
)

# The Go client (go_resource_abstractor/client) picked a 10s default for the same
# reason: there was none before, so a hung abstractor hung the caller.
DEFAULT_TIMEOUT = 10.0

# One httpx.Client (and its connection pool) per address, so patching
# RESOURCE_ABSTRACTOR_ADDR mid-process - as services_test.py does - builds a fresh
# client rather than reusing one pointed at the wrong host, while repeated calls
# against the same address still share a connection.
_clients: dict[str, Client] = {}


def _client() -> Client:
    addr = RESOURCE_ABSTRACTOR_ADDR
    client = _clients.get(addr)
    if client is None:
        client = Client(base_url=addr, timeout=DEFAULT_TIMEOUT)
        _clients[addr] = client
    return client


def make_request(kwargs: dict[str, Any]) -> Optional[Any]:
    """Send a request built by a generated `_get_kwargs()` and return its decoded body.

    kwargs is dispatched as-is - unlike the generated `sync_detailed()` functions,
    which parse the response into the models declared in openapi.yaml, this hands back
    the raw decoded JSON. The abstractor stores schemaless Mongo documents that often
    carry fields the spec doesn't describe (`microserviceID`, `next_instance_progressive_number`,
    ...), and every call site in this repo already expects those fields to survive a
    round trip - which parsing into a typed model, then throwing the model away, would
    not preserve reliably beyond `additional_properties`. See
    go_resource_abstractor/client/response.go for the Go client's identical reasoning
    for using the bare generated methods over the `*WithResponse` ones.

    Returns None for a 404, a connection failure, or any other non-2xx response alike,
    matching every existing call site's `is None` check. A successful response with no
    body (a 204) also returns None, which every 204-returning call site already
    discards.
    """
    url = f"{RESOURCE_ABSTRACTOR_ADDR}{kwargs.get('url', '')}"
    try:
        response = _client().get_httpx_client().request(**kwargs)
        response.raise_for_status()
        if not response.content:
            return None
        return response.json()
    except (httpx.HTTPError, json.JSONDecodeError):
        logging.warning(f"Calling {url} not successful.")

    return None


def translate_kwargs(kwargs: dict[str, Any], wire_to_python: dict[str, str]) -> dict[str, Any]:
    """Map a caller's wire-named query kwargs onto a generated `_get_kwargs()`'s
    Python parameter names, dropping (and warning about) anything the spec doesn't
    declare.

    This is what lets get_apps(**kwargs)/get_candidates(**kwargs)/get_jobs(**kwargs)
    keep accepting arbitrary kwargs: the spec only declares a handful of query
    parameters per endpoint, and openapi-python-client renames the ones that aren't
    already snake_case (userId -> user_id, applicationID -> application_id) to valid
    Python identifiers, so a caller's wire-named kwarg has to be translated before it
    can be passed to the generated function.

    An argument the spec doesn't declare - a Mongo filter document, say - is dropped
    here with a warning instead of silently vanishing on the wire the way it did
    before: see job_management.py's mark_inactive_as_failed and
    get_jobs_with_failed_instances, which pass **{"instance_list": {"$elemMatch": ...}}
    and **{"$or": [...]} to get_jobs(). Neither was ever a real filter - the resource
    abstractor has never had a query parameter for a raw Mongo filter - so this changes
    nothing about what jobs come back, only how loudly the no-op is reported.
    """
    translated = {}
    for key, value in kwargs.items():
        python_name = wire_to_python.get(key)
        if python_name is None:
            logging.warning(f"Ignoring unsupported resource abstractor filter: {key}")
            continue
        translated[python_name] = value
    return translated
