# ......... CREATE ADMIN USER ...............
#############################################
from datetime import datetime

from bson import ObjectId
from werkzeug.security import generate_password_hash
import ext_requests.mongodb_client as db

def create_admin():
    existing_user = mongo_get_user_by_name('Admin')
    if existing_user is None:
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
        db.app.logger.info("MONGODB - created admin")


def mongo_save_user(data):
    db.mongo_users.insert_one(data)
    return db.mongo_users.find()


def mongo_get_user():
    return db.mongo_users.find()


def mongo_get_user_by_name(username):
    return db.mongo_users.find_one({"name": username})


def mongo_get_user_by_id(user_id):
    return db.mongo_users.find_one(ObjectId(user_id))


def mongo_delete_user(username):
    db.mongo_users.find_one_and_delete({'name': username})
    return db.mongo_users.find()


def mongo_update_user(user_id, user):
    print(user)
    if "_id" in user:
        del user['_id']
    db.mongo_users.find_one_and_update({'_id': ObjectId(user_id)},
                              {'$set': user})
    return "ok"


def mongo_create_password_reset_token(user_id, expiry_date, token_hash):
    data = {
        'user_id': user_id,
        'expiry_date': expiry_date,
        'token_hash': token_hash
    }
    db.mongo_users.insert_one(data)


def mongo_get_password_reset_token(token_hash):
    return db.mongo_users.find({"token_hash": token_hash})[0]


def mongo_delete_password_reset_token(token_id):
    db.mongo_users.find_one_and_delete({"_id": token_id})
