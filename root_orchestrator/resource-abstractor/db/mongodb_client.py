import os

from flask_pymongo import PyMongo

MONGO_URL = os.environ.get("CLOUD_MONGO_URL")
MONGO_PORT = os.environ.get("CLOUD_MONGO_PORT")

MONGO_BASE_ADDR = f"mongodb://{MONGO_URL}:{MONGO_PORT}"
MONGO_ADDR_CLUSTERS = f"{MONGO_BASE_ADDR}/clusters"
MONGO_ADDR_JOBS = f"{MONGO_BASE_ADDR}/jobs"

mongo_clusers = None
mongo_apps = None
mongo_jobs = None
app = None


def mongo_init(flask_app):
    global mongo_clusters, mongo_jobs, mongo_apps
    global app

    app = flask_app

    mongo_clusters = PyMongo(app, uri=MONGO_ADDR_CLUSTERS).db["clusters"]
    mongo_apps = PyMongo(app, uri=MONGO_ADDR_JOBS).db["apps"]
    mongo_jobs = PyMongo(app, uri=MONGO_ADDR_JOBS).db["jobs"]

    app.logger.info("init mongo")
