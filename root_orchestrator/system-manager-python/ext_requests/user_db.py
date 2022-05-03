# ......... CREATE ADMIN USER ...............
#############################################
from datetime import datetime

from bson import ObjectId
from werkzeug.security import generate_password_hash
from ext_requests.mongodb_client import *


def create_admin():
    existing_user = mongo_get_user_by_name('Admin')
    if len(existing_user) == 0:
        d = datetime.now()
        user_roles = [{'name': 'Admin', 'description': 'This is the admin role'},
                      {'name': 'Application_Provider', 'description': 'This is the app role'},
                      {'name': 'Infrastructure_Provider', 'description': 'This is the infra role'}]

        user = {
            'name': 'Admin',
            'email': '',
            'password': 'Admin',
            'roles': user_roles,
            'created_at': d.strftime("%d/%m/%Y %H:%M")
        }

        password = 'Admin'
        user['password'] = generate_password_hash('Admin')
        mongo_save_user(user)
        user['password'] = password
        app.logger.info("MONGODB - created admin")


def mongo_save_user(data):
    mongo_users.insert_one(data)
    return mongo_users.find()


def mongo_get_user():
    return mongo_users.find()


def mongo_get_user_by_name(username):
    user = list(mongo_users.find({"name": username}))
    if len(user) == 0:
        return []
    return user[0]


def mongo_get_user_by_id(user_id):
    return mongo_users.find_one(ObjectId(user_id))


def mongo_delete_user(username):
    mongo_users.find_one_and_delete({'name': username})
    return mongo_users.find()


def mongo_update_user(user_id, user):
    print(user)
    if "_id" in user:
        del user['_id']
    mongo_users.find_one_and_update({'_id': ObjectId(user_id)},
                              {'$set': user})
    return "ok"


def mongo_create_password_reset_token(user_id, expiry_date, token_hash):
    data = {
        'user_id': user_id,
        'expiry_date': expiry_date,
        'token_hash': token_hash
    }
    mongo_users.insert_one(data)


def mongo_get_password_reset_token(token_hash):
    return mongo_users.find({"token_hash": token_hash})[0]


def mongo_delete_password_reset_token(token_id):
    mongo_users.find_one_and_delete({"_id": token_id})
