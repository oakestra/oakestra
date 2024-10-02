from enum import Enum

from bson.objectid import ObjectId
from db import mongodb_client as db


class HookEventsEnum(Enum):
    POST_CREATE = "post_create"
    PRE_CREATE = "pre_create"

    POST_UPDATE = "post_update"
    PRE_UPDATE = "pre_update"

    POST_DELETE = "post_delete"
    PRE_DELETE = "pre_delete"


ASYNC_EVENTS = [
    HookEventsEnum.POST_CREATE.value,
    HookEventsEnum.POST_UPDATE.value,
    HookEventsEnum.POST_DELETE.value,
]

SYNC_EVENTS = [
    HookEventsEnum.PRE_CREATE.value,
    HookEventsEnum.PRE_UPDATE.value,
    HookEventsEnum.PRE_DELETE.value,
]


def find_hooks(filter={}):
    return db.mongo_hooks.find(filter) or []


def find_hook_by_id(hook_id):
    hooks = find_hooks({"_id": hook_id})
    return hooks[0] if hooks else None


def update_hook(hook_id, data):
    data.pop("_id", None)

    return db.mongo_hooks.find_one_and_update(
        {"_id": ObjectId(hook_id)}, {"$set": data}, return_document=True
    )


def create_hook(data):
    data.pop("_id", None)
    res = db.mongo_hooks.insert_one(data)

    return db.mongo_hooks.find_one({"_id": res.inserted_id})


def delete_hook(hook_id):
    return db.mongo_hooks.delete_one({"_id": ObjectId(hook_id)})
