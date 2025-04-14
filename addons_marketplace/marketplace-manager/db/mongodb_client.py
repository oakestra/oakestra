import os

from flask_pymongo import PyMongo

MONGO_URL = os.environ.get("ADDON_MARKETPLACE_MONGO_URL")
MONGO_PORT = os.environ.get("ADDON_MARKETPLACE_MONGO_PORT")

MONGO_BASE_ADDR = f"mongodb://{MONGO_URL}:{MONGO_PORT}"

# contains available addons
MONGO_ADDR_MARKETPLACE = f"{MONGO_BASE_ADDR}/marketplace"

mongo_marketplace = None
app = None


def mongo_init(flask_app):
    global app, mongo_marketplace

    app = flask_app

    mongo_marketplace = PyMongo(app, uri=MONGO_ADDR_MARKETPLACE).db["marketplace"]
