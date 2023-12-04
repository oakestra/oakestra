import os
from flask_pymongo import PyMongo
from bson.objectid import ObjectId

MONGO_URL = os.environ.get('CLOUD_MONGO_URL')
MONGO_PORT = os.environ.get('CLOUD_MONGO_PORT')

MONGO_ADDR_CLUSTERS = 'mongodb://' + str(MONGO_URL) + ':' + str(MONGO_PORT) + '/clusters'

mongo_clusers = None
app = None

def mongo_init(flask_app):
    global mongo_clusters, mongo_jobs
    global app

    app = flask_app
    app.config["MONGO_URI"] = MONGO_ADDR_CLUSTERS

    mongo_clusters = PyMongo(app, uri=MONGO_ADDR_CLUSTERS)

    app.logger.info("init mongo")


# .................... Cluster operations ................#
###########################################################

def mongo_get_all_clusters():
    global mongo_clusters
    return mongo_clusters.db.clusters.find()

def mongo_get_cluster_by_id(cluster_id):
    global mongo_clusters
    return mongo_clusters.db.clusters.find_one({"_id": ObjectId(cluster_id)})