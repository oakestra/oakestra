from enum import Enum

from bson.objectid import ObjectId
from db import mongodb_client as db


class HookEventsEnum(Enum):
    AFTER_CREATE = "afterCreate"
    BEFORE_CREATE = "beforeCreate"

    AFTER_UPDATE = "afterUpdate"
    BEFORE_UPDATE = "beforeUpdate"

    AFTER_DELETE = "afterDelete"
    BEFORE_DELETE = "beforeDelete"


ASYNC_EVENTS = [
    HookEventsEnum.AFTER_CREATE.value,
    HookEventsEnum.AFTER_UPDATE.value,
    HookEventsEnum.AFTER_DELETE.value,
]

SYNC_EVENTS = [
    HookEventsEnum.BEFORE_CREATE.value,
    HookEventsEnum.BEFORE_UPDATE.value,
    HookEventsEnum.BEFORE_DELETE.value,
]


def find_hooks(filter={}):
    return db.mongo_hooks.find(filter) or []


def find_hook_by_id(hook_id):
    hooks = find_hooks({"_id": hook_id})
    return hooks[0] if hooks else None


def update_hook(hook_id, data):
    return db.mongo_hooks.find_one_and_update(
        {"_id": ObjectId(hook_id)}, {"$set": data}, return_document=True
    )


def create_hook(data):
    res = db.mongo_hooks.insert_one(data)

    return db.mongo_hooks.find_one({"_id": res.inserted_id})


def create_update_hook(hook_data):
    hook_name = hook_data.get("hook_name")
    hook = db.mongo_hooks.find_one({"hook_name": hook_name})

    if hook:
        return update_hook(str(hook.get("_id")), hook_data)

    return create_hook(hook_data)


def delete_hook(hook_id):
    return db.mongo_hooks.delete_one({"_id": hook_id})
