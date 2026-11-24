import logging
import os

from flask_pymongo import PyMongo

MONGO_URL = os.environ.get("ROOT_MONGO_URL", "localhost")
MONGO_PORT = os.environ.get("ROOT_MONGO_PORT", 10007)

MONGO_ADDR_USERS = f"mongodb://{MONGO_URL}:{MONGO_PORT}/users"

mongo_users = None
mongo_organization = None

app = None

CLUSTERS_FRESHNESS_INTERVAL = 45

logger = logging.getLogger("system_manager")


def mongo_init(flask_app):
    global app, mongo_users
    global mongo_organization

    app = flask_app

    mongo_users = PyMongo(app, uri=MONGO_ADDR_USERS).db["user"]
    mongo_organization = PyMongo(app, uri=MONGO_ADDR_USERS).db["organization"]

    logger.info("MONGODB - init mongo")
    logger.info(mongo_users)


def mongo_init_standalone():
    """Same collections without a Flask app, for command line tools (e.g. users.emergency)."""
    global mongo_users, mongo_organization
    from pymongo import MongoClient

    database = MongoClient(MONGO_ADDR_USERS, serverSelectionTimeoutMS=5000)["users"]
    mongo_users = database["user"]
    mongo_organization = database["organization"]
