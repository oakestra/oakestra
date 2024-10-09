from ext_requests.organization_db import mongo_add_user_role_to_organization
from ext_requests.user_db import (
    mongo_delete_user,
    mongo_get_user,
    mongo_get_user_by_name,
    mongo_get_user_by_organization_id,
    mongo_update_user,
)


def user_get_by_name(username, oragnization_id):
    return mongo_get_user_by_name(username, oragnization_id)


def user_delete(username):
    return mongo_delete_user(username)


def user_add(username, data, organization_id):
    user_id = str(mongo_get_user_by_name(username)["_id"])
    if "roles" in data:
        mongo_add_user_role_to_organization(user_id, organization_id, data["roles"])
        del data["roles"]

    return mongo_update_user(user_id, data)


def user_get_all():
    return mongo_get_user()


def user_get_all_from_Organization(organization_id):
    return mongo_get_user_by_organization_id(organization_id)
