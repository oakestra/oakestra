import os

from flask_pymongo import PyMongo

MONGO_URL = os.environ.get("CLOUD_MONGO_URL", "localhost")
MONGO_PORT = os.environ.get("CLOUD_MONGO_PORT", 10007)

MONGO_ADDR_CLUSTERS = "mongodb://" + str(MONGO_URL) + ":" + str(MONGO_PORT) + "/clusters"
MONGO_ADDR_JOBS = "mongodb://" + str(MONGO_URL) + ":" + str(MONGO_PORT) + "/jobs"
MONGO_ADDR_USERS = "mongodb://" + str(MONGO_URL) + ":" + str(MONGO_PORT) + "/users"
MONGO_ADDR_GATEWAYS = "mongodb://" + str(MONGO_URL) + ":" + str(MONGO_PORT) + "/gateways"

mongo_clusters = None
mongo_jobs = None
mongo_users = None
mongo_applications = None
mongo_services = None
mongo_organization = None
mongo_gateways = None
mongo_gateway_services = None

app = None

CLUSTERS_FRESHNESS_INTERVAL = 45


def mongo_init(flask_app):
    global app, mongo_clusters, mongo_jobs, mongo_users
    global mongo_applications, mongo_services, mongo_organization
    global mongo_gateways, mongo_gateway_services

    app = flask_app

    mongo_clusters = PyMongo(app, uri=MONGO_ADDR_CLUSTERS)
    mongo_jobs = PyMongo(app, uri=MONGO_ADDR_JOBS)
    mongo_users = PyMongo(app, uri=MONGO_ADDR_USERS).db["user"]
    mongo_organization = PyMongo(app, uri=MONGO_ADDR_USERS).db["organization"]
    mongo_applications = mongo_jobs.db["apps"]
    mongo_services = mongo_jobs.db["jobs"]
    mongo_gateways = PyMongo(app, uri=MONGO_ADDR_GATEWAYS)
    mongo_gateway_services = mongo_gateways.db["services"]

    app.logger.info("MONGODB - init mongo")
    app.logger.info(mongo_users)
