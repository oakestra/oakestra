import logging
import os

from flask_pymongo import PyMongo

MONGO_URL = os.environ.get("ROOT_MONGO_URL", "localhost")
MONGO_PORT = os.environ.get("ROOT_MONGO_PORT", 10007)

MONGO_ADDR_USERS = f"mongodb://{MONGO_URL}:{MONGO_PORT}/users"

mongo_users = None
mongo_organization = None
mongo_cluster_tokens = None

app = None

CLUSTERS_FRESHNESS_INTERVAL = 45

logger = logging.getLogger("system_manager")


def mongo_init(flask_app):
    global app, mongo_users
    global mongo_organization
    global mongo_cluster_tokens

    app = flask_app

    mongo_users = PyMongo(app, uri=MONGO_ADDR_USERS).db["user"]
    mongo_organization = PyMongo(app, uri=MONGO_ADDR_USERS).db["organization"]
    mongo_cluster_tokens = PyMongo(app, uri=MONGO_ADDR_USERS).db["cluster_tokens"]

    logger.info("MONGODB - init mongo")
    logger.info(mongo_users)
