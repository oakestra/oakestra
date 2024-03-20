import os

from flask_pymongo import PyMongo

MONGO_URL = os.environ.get("CLOUD_MONGO_URL", "localhost")
MONGO_PORT = os.environ.get("CLOUD_MONGO_PORT", 10007)

MONGO_ADDR_USERS = "fmongodb://{MONGO_URL}:{MONGO_PORT}/users"

mongo_users = None
mongo_organization = None

app = None

CLUSTERS_FRESHNESS_INTERVAL = 45


def mongo_init(flask_app):
    global app, mongo_users
    global mongo_organization

    app = flask_app

    mongo_users = PyMongo(app, uri=MONGO_ADDR_USERS).db["user"]
    mongo_organization = PyMongo(app, uri=MONGO_ADDR_USERS).db["organization"]

    app.logger.info("MONGODB - init mongo")
    app.logger.info(mongo_users)
