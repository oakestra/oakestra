import os

from flask_pymongo import PyMongo

MONGO_URL = os.environ.get("CLOUD_MONGO_URL")
MONGO_PORT = os.environ.get("CLOUD_MONGO_PORT")

MONGO_ADDR_CLUSTERS = f"mongodb://{MONGO_URL}:{MONGO_PORT}/clusters"
MONGO_ADDR_JOBS = f"mongodb://{MONGO_URL}:{MONGO_PORT}/jobs"

mongo_clusers = None
mongo_jobs = None
app = None


def mongo_init(flask_app):
    global mongo_clusters, mongo_jobs
    global app

    app = flask_app
    app.config["MONGO_URI"] = MONGO_ADDR_CLUSTERS

    mongo_clusters = PyMongo(app, uri=MONGO_ADDR_CLUSTERS)
    mongo_jobs = PyMongo(app, uri=MONGO_ADDR_JOBS)

    app.logger.info("init mongo")
