from ext_requests.user_db import mongo_get_user_by_name, mongo_delete_user, mongo_update_user, mongo_get_user


def user_get_by_name(username):
    return mongo_get_user_by_name(username)


def user_delete(username):
    return mongo_delete_user(username)


def user_add(username, data):
    # TODO check updated fields
    return mongo_update_user((mongo_get_user_by_name(username))['_id'], data)


def user_get_all():
    return mongo_get_user()
