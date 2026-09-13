import logging
import os

from flask_pymongo import PyMongo

MONGO_URL = os.environ.get("ROOT_MONGO_URL", "localhost")
MONGO_PORT = os.environ.get("ROOT_MONGO_PORT", 10007)

MONGO_ADDR_USERS = f"mongodb://{MONGO_URL}:{MONGO_PORT}/users"

mongo_users = None
mongo_organization = None
mongo_registration_tokens = None
mongo_revoked_certs = None

app = None

CLUSTERS_FRESHNESS_INTERVAL = 45

logger = logging.getLogger("system_manager")


def mongo_init(flask_app):
    global app, mongo_users
    global mongo_organization
    global mongo_registration_tokens
    global mongo_revoked_certs

    app = flask_app

    mongo_users = PyMongo(app, uri=MONGO_ADDR_USERS).db["user"]
    mongo_organization = PyMongo(app, uri=MONGO_ADDR_USERS).db["organization"]
    mongo_registration_tokens = PyMongo(app, uri=MONGO_ADDR_USERS).db["registration_tokens"]
    # Mongo TTL monitor garbage-collects expired one-time registration tokens.
    mongo_registration_tokens.create_index("expiry_date", expireAfterSeconds=0)

    mongo_revoked_certs = PyMongo(app, uri=MONGO_ADDR_USERS).db["revoked_certs"]
    # TTL: keep revocation records for twice the max cert validity so
    # recently expired certs are still blocked during their potential reuse window.
    mongo_revoked_certs.create_index("revoked_at", expireAfterSeconds=63072000)  # 730 days

    logger.info("MONGODB - init mongo")
    logger.info(mongo_users)
