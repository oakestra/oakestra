import db.mongodb_client as db
from bson.objectid import ObjectId


def find_apps(filter={}):
    return db.mongo_apps.find(filter)


def find_app_by_id(app_id, extra_filter={}):
    filter = {**extra_filter, "_id": ObjectId(app_id)}
    app = list(find_apps(filter=filter))

    return app[0] if app else None


def delete_app(app_id):
    filter = {"_id": ObjectId(app_id)}
    return db.mongo_apps.find_one_and_delete(filter, return_document=True)


def update_app(app_id, data):
    return db.mongo_apps.find_one_and_update(
        {"_id": ObjectId(app_id)},
        {"$set": data},
        return_document=True,
    )


def _create_app(app_data):
    res = db.mongo_apps.insert_one(app_data)

    return res.inserted_id


def create_app(app_data):
    inserted_id = _create_app(app_data)

    return update_app(str(inserted_id), {"applicationID": str(inserted_id)})
