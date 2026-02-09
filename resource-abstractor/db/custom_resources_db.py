from bson.objectid import ObjectId

from db import mongodb_client as db


def find_custom_resources(filter={}):
    return db.mongo_meta_data.find(filter)


def find_custom_resource_by_type(resource_type):
    return db.mongo_meta_data.find_one({"resource_type": resource_type})


def delete_custom_resource_by_type(resource_type):
    return db.mongo_meta_data.find_one_and_delete({"resource_type": resource_type})


def create_custom_resource(data):
    data.pop("_id", None)

    inserted = db.mongo_meta_data.insert_one(data)

    return db.mongo_meta_data.find_one({"_id": inserted.inserted_id})


def check_custom_resource_exists(resource_type):
    custom_resource = find_custom_resource_by_type(resource_type)

    return custom_resource is not None


def _get_collection(resource_type):
    return db.db_custom_resources.db[resource_type]


def find_resources(resource_type, filter={}):
    collection = _get_collection(resource_type)

    return collection.find(filter)


def find_resource_by_id(resource_type, id):
    collection = _get_collection(resource_type)

    return collection.find_one({"_id": ObjectId(id)})


def create_resource(resource_type, data):
    collection = _get_collection(resource_type)

    inserted = collection.insert_one(data)

    return collection.find_one({"_id": inserted.inserted_id})


def update_resource(resource_type, id, data):
    collection = _get_collection(resource_type)

    # Remove _id from data to avoid MongoDB immutable field error
    update_data = {k: v for k, v in data.items() if k != "_id"}

    return collection.find_one_and_update(
        {"_id": ObjectId(id)}, {"$set": update_data}, return_document=True
    )


def delete_resource(resource_type, id):
    collection = _get_collection(resource_type)

    return collection.find_one_and_delete({"_id": ObjectId(id)})


def delete_all_resources(resource_type):
    """Delete all instances of a specific resource type."""
    collection = _get_collection(resource_type)
    result = collection.delete_many({})
    return result.deleted_count
