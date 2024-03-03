import db.mongodb_client as db
from bson.objectid import ObjectId


def find_all_apps(filter={}):
    return db.mongo_apps.find(filter)


def find_app_by_id(app_id, filter={}):
    filter["_id"] = ObjectId(app_id)
    app = list(find_all_apps(filter=filter))

    return app[0] if app else None


def find_user_apps(user_id, filter={}):
    return find_all_apps(filter={"userId": user_id})


def find_user_app(user_id, app_id):
    return find_app_by_id(app_id, filter={"userId": user_id})


def delete_app(user_id, app_id):
    return db.mongo_apps.find_one_and_delete(
        {"_id": ObjectId(app_id), "userId": user_id}, return_document=True
    )


def update_app(user_id, app_id, data):
    return db.mongo_apps.find_one_and_update(
        {"_id": ObjectId(app_id), "userId": user_id},
        {"$set": data},
        return_document=True,
    )


def _create_app(app_data):
    res = db.mongo_apps.insert_one(app_data)

    return res.inserted_id


def create_app(user_id, app_data):
    app_data["userId"] = user_id
    inserted_id = _create_app(app_data)

    return update_app(app_data.get("userId"), str(inserted_id), {"applicationID": str(inserted_id)})


def create_update_app(app_data):
    app_name = app_data.get("app_name")
    app = db.mongo_apps.find_one({"app_name": app_name})

    if app:
        return update_app(str(app.get("_id")), app_data)
    else:
        return create_app(app_data)

    return app


def find_app_by_name_and_namespace(app_name, app_ns):
    return db.mongo_apps.find_one({"application_name": app_name, "application_namespace": app_ns})
