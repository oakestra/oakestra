import db.mongodb_client as db
from bson.objectid import ObjectId


def find_addons(filter={}):
    return db.mongo_addons.find(filter)


def find_addon_by_id(addon_id):
    addon = list(find_addons({"_id": ObjectId(addon_id)}))
    return addon[0] if addon else None


def find_active_addons():
    return find_addons({"status": "enabled"})


def create_addon(addon):
    addon.pop("_id", None)
    inserted = db.mongo_addons.insert_one(addon)

    return db.mongo_addons.find_one({"_id": inserted.inserted_id})


def update_addon(addon_id, addon_data):
    return db.mongo_addons.find_one_and_update(
        {"_id": ObjectId(addon_id)}, {"$set": addon_data}, return_document=True
    )
