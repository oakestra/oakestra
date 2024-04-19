# ......... CREATE ADMIN USER ...............
#############################################
from datetime import datetime

import ext_requests.mongodb_client as db
from bson import ObjectId
from ext_requests.organization_db import (
    mongo_add_organization,
    mongo_add_user_role_to_organization,
    mongo_delete_all_role_entrys_of_user,
    mongo_get_organization_by_name,
    mongo_get_roles_of_user_in_organization,
)
from werkzeug.security import generate_password_hash


def create_admin():
    user_id = None
    existing_user = mongo_get_user_by_name("Admin")
    if existing_user is None:
        d = datetime.now()
        admin = {
            "name": "Admin",
            "email": "",
            "password": "Admin",
            "created_at": d.strftime("%d/%m/%Y %H:%M"),
        }
        admin["password"] = generate_password_hash("Admin")
        user_id = str(mongo_save_user_without_roles(admin))
    else:
        user_id = str(existing_user["_id"])

    existing_organization = mongo_get_organization_by_name("root")
    if existing_organization is None:
        user_roles = [
            "Admin",
            "Organization_Admin",
            "Application_Provider",
            "Infrastructure_Provider",
        ]

        member = [{"user_id": user_id, "roles": user_roles}]

        organization = {"name": "root", "member": member}
        mongo_add_organization(organization)
    db.app.logger.info("MONGODB - created root organization with admin")


def mongo_save_user(data, organization_id):
    u = {
        "name": data["name"],
        "email": data["email"],
        "password": data["password"],
        "created_at": data["created_at"],
    }
    roles = data["roles"]
    user = db.mongo_users.insert_one(u)
    mongo_add_user_role_to_organization(str(user.inserted_id), organization_id, roles)
    return mongo_get_single_user_of_organization(str(user.inserted_id), organization_id)


def mongo_get_single_user_of_organization(user_id, organization_id):
    user = mongo_get_user_by_id(user_id)
    user["roles"] = mongo_get_roles_of_user_in_organization(user_id, organization_id)
    return user


def mongo_save_user_without_roles(data):
    user = db.mongo_users.insert_one(data)
    return user.inserted_id


def mongo_get_user():
    return db.mongo_users.find()


def mongo_get_user_by_name(username, organization_id=None):
    user = db.mongo_users.find_one({"name": username})
    return mongo_add_roles_to_user(user, organization_id)


def mongo_get_user_by_id(user_id, organization_id=None):
    user = db.mongo_users.find_one(ObjectId(user_id))
    return mongo_add_roles_to_user(user, organization_id)


def mongo_get_user_by_organization_id(organization_id):
    organization = db.mongo_organization.find_one({"_id": ObjectId(organization_id)})
    if organization is None:
        return []

    user = []
    for m in organization["member"]:
        u = db.mongo_users.find_one({"_id": ObjectId(m.get("user_id"))})
        if u is not None:
            u["roles"] = m.get("roles")
            user.append(u)
    return user


def mongo_delete_user(username):
    user = db.mongo_users.find_one_and_delete({"name": username})
    mongo_delete_all_role_entrys_of_user(str(user["_id"]))
    return db.mongo_users.find()


def mongo_update_user(user_id, user):
    print(user)
    if "_id" in user:
        del user["_id"]
    db.mongo_users.find_one_and_update({"_id": ObjectId(user_id)}, {"$set": user})
    return "ok"


def mongo_add_roles_to_user(user, organization_id):
    if organization_id is None:
        return user

    organization = db.mongo_organization.find_one({"_id": ObjectId(organization_id)})
    if organization is None:
        return user

    for m in organization["member"]:
        if m.get("user_id") == str(user["_id"]):
            user["roles"] = m.get("roles")
    return user


def mongo_create_password_reset_token(user_id, expiry_date, token_hash):
    data = {"user_id": user_id, "expiry_date": expiry_date, "token_hash": token_hash}
    db.mongo_users.insert_one(data)


def mongo_get_password_reset_token(token_hash):
    return db.mongo_users.find({"token_hash": token_hash})[0]


def mongo_delete_password_reset_token(token_id):
    db.mongo_users.find_one_and_delete({"_id": token_id})
