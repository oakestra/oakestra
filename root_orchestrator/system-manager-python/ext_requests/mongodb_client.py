import os

from flask_pymongo import PyMongo
from oakestra_logging import get_logger

MONGO_URL = os.environ.get("ROOT_MONGO_URL", "localhost")
MONGO_PORT = os.environ.get("ROOT_MONGO_PORT", 10007)

MONGO_ADDR_USERS = f"mongodb://{MONGO_URL}:{MONGO_PORT}/users"

mongo_users = None
mongo_organization = None

app = None

CLUSTERS_FRESHNESS_INTERVAL = 45

logger = get_logger(__name__)


def mongo_init(flask_app):
    global app, mongo_users
    global mongo_organization

    app = flask_app

    mongo_users = PyMongo(app, uri=MONGO_ADDR_USERS).db["user"]
    mongo_organization = PyMongo(app, uri=MONGO_ADDR_USERS).db["organization"]

    logger.info("Initialized MongoDB clients", event_name="database.initialized")
