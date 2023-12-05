import os
from flask_pymongo import PyMongo
from bson.objectid import ObjectId
from _datetime import datetime

CLUSTERS_FRESHNESS_INTERVAL = 30

MONGO_URL = os.environ.get('CLOUD_MONGO_URL')
MONGO_PORT = os.environ.get('CLOUD_MONGO_PORT')

MONGO_ADDR_CLUSTERS = 'mongodb://' + str(MONGO_URL) + ':' + str(MONGO_PORT) + '/clusters'

mongo_clusers = None
app = None

def mongo_init(flask_app):
    global mongo_clusters
    global app

    app = flask_app
    app.config["MONGO_URI"] = MONGO_ADDR_CLUSTERS

    mongo_clusters = PyMongo(app, uri=MONGO_ADDR_CLUSTERS)

    app.logger.info("init mongo")


# .................... Cluster operations ................#
###########################################################

def find_clusters_by_filter(filter):
    return mongo_clusters.db.clusters.find(filter)

def find_all_clusters():
    return mongo_clusters.db.clusters.find()

def find_cluster_by_id(cluster_id):
    return mongo_clusters.db.clusters.find_one({"_id": ObjectId(cluster_id)})

def find_active_clusters():
    now_timestamp = datetime.now().timestamp()
    return find_clusters_by_filter(
        {'last_modified_timestamp': {'$gt': now_timestamp - CLUSTERS_FRESHNESS_INTERVAL}}
    )

def find_cluster_by_name(name):
    return mongo_clusters.db.clusters.find_one({"name": name})
