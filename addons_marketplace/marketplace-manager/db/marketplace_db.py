from enum import Enum

import db.mongodb_client as db
from bson.objectid import ObjectId


class StatusEnum(Enum):
    UNDER_REVIEW = "under_review"
    APPROVED = "approved"
    VERIFICATION_FAILED = "verification_failed"


def find_addons(filter={}):
    return db.mongo_marketplace.find(filter)


def find_approved_addons(filter={}):
    return find_addons({**filter, "status": "approved"})


def find_addon_by_id(addon_id):
    addon = list(find_addons({"_id": ObjectId(addon_id)}))
    return addon[0] if addon else None


def create_addon(addon):
    inserted = db.mongo_marketplace.insert_one(addon)

    return db.mongo_marketplace.find_one({"_id": inserted.inserted_id})


def update_addon(addon_id, addon_data):
    return db.mongo_marketplace.find_one_and_update(
        {"_id": ObjectId(addon_id)}, {"$set": addon_data}, return_document=True
    )
