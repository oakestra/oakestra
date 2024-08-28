import json
import logging
import os
import threading
from functools import wraps

from db import hooks_db
from flask import request
from requests import exceptions, post

RESPONSE_TIMEOUT = os.environ.get("HOOK_REQUEST_TIMEOUT", 5)
CONNECT_TIMEOUT = os.environ.get("HOOK_CONNECT_TIMEOUT", 10)


def call_webhook(url, data):
    try:
        logging.info(f"Calling webhook with data: {data}")
        response = post(url, json=data, timeout=(CONNECT_TIMEOUT, RESPONSE_TIMEOUT))
        response.raise_for_status()
        data = response.json()
    except exceptions.ConnectTimeout:
        logging.warning(f"Request timed out while trying to connect to {url}")
    except exceptions.ReadTimeout:
        logging.warning(f"{url} failed to return response in the allotted amount of time")
    except exceptions.RequestException:
        logging.warning(f"Hook failed to send request to {url}")

    # if request fails the original data is returned
    return data


def process_async_hook(entity_name, event, entity_id):
    async_hooks = hooks_db.find_hooks({"entity": entity_name, "events": {"$in": [event]}})

    for hook in async_hooks:
        data = {
            "entity": entity_name,
            "entity_id": entity_id,
            "event": event,
        }
        threading.Thread(target=call_webhook, args=(hook["webhook_url"], data)).start()


def process_post_create(entity_name, entity_id):
    process_async_hook(entity_name, hooks_db.HookEventsEnum.POST_CREATE.value, entity_id)


def process_post_update(entity_name, entity_id):
    process_async_hook(entity_id, entity_name, hooks_db.HookEventsEnum.POST_UPDATE.value)


def process_post_delete(entity_name, entity_id):
    process_async_hook(entity_id, entity_name, hooks_db.HookEventsEnum.POST_DELETE.value)


def process_sync_hook(entity_name, event, data):
    sync_hooks = hooks_db.find_hooks({"entity": entity_name, "events": {"$in": [event]}})

    for hook in sync_hooks:
        data = call_webhook(hook["webhook_url"], data)

    return data


def process_pre_create(entity_name, data):
    return process_sync_hook(entity_name, hooks_db.HookEventsEnum.PRE_CREATE.value, data)


def process_pre_update(entity_name, data):
    return process_sync_hook(entity_name, hooks_db.HookEventsEnum.PRE_UPDATE.value, data)


def perform_create(entity_name, fn, *args):
    data = args[-1]
    data = process_pre_create(entity_name, data)

    args = list(args)
    args[-1] = data

    result = fn(*tuple(args))
    process_post_create(entity_name, str(result.get("_id")))

    return result


def perform_update(entity_name, fn, *args):
    data = args[-1]
    data = process_pre_update(entity_name, data)

    # Update the last argument with the new data
    args = list(args)
    args[-1] = data

    result = fn(*tuple(args))
    process_post_update(entity_name, str(result.get("_id")))

    return result


# put currently is not supported
api_hooks_map = {
    "post": (process_pre_create, process_post_create),
    "patch": (process_pre_update, process_post_update),
    "delete": (None, process_post_delete),
}


# doesn't work well with @arguments decorator
# this decorator has to be the closest to the method
def pre_post_hook(entity_name, with_param_id=None):
    def decorator(fn):
        @wraps(fn)
        def wrapper(*args, **kwargs):
            method_name = fn.__name__
            if method_name not in api_hooks_map.keys():
                raise ValueError(f"Method {method_name} not supported")

            # incase this was wrapped by @arguments
            data = args[1] if len(args) > 1 else request.json

            pre_fn = api_hooks_map[method_name][0]
            post_fn = api_hooks_map[method_name][1]

            entity_id = kwargs.get(with_param_id) if with_param_id else None

            if pre_fn:
                if entity_id:
                    data["_id"] = entity_id

                data = pre_fn(entity_name, data)

            args = list(args)
            if data and len(args) > 1:
                args[1] = data
            elif data:
                args.append(data)

            result = fn(*tuple(args), **kwargs)
            result_id = str(result["_id"]) if isinstance(result, dict) else None

            # incase result was json encoded
            if result and result_id is None:
                try:
                    result_id = json.loads(result).get("_id")
                except json.JSONDecodeError:
                    pass
            elif result_id is None:
                # fallback on passed id
                # This happens with delete methods that don't return the deleted entity
                result_id = entity_id

            post_fn(entity_name, result_id)

            return result

        return wrapper

    return decorator
