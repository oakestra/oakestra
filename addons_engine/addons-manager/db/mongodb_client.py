import os

from flask_pymongo import PyMongo

MONGO_URL = os.environ.get("ADDONS_ENGINE_MONGO_URL")
MONGO_PORT = os.environ.get("ADDONS_ENGINE_MONGO_PORT")

MONGO_BASE_ADDR = f"mongodb://{MONGO_URL}:{MONGO_PORT}"

# contains installed addons
MONGO_ADDR_ADDONS = f"{MONGO_BASE_ADDR}/addons"


mongo_addons = None
app = None


def mongo_init(flask_app):
    global app, mongo_addons

    app = flask_app

    mongo_addons = PyMongo(app, uri=MONGO_ADDR_ADDONS).db["addons"]
    mongo_addons.create_index("marketplace_id", unique=True)
