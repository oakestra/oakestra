from bson.objectid import ObjectId

import db.mongodb_client as db


def find_clusters(filter):
    return db.mongo_clusters.db.clusters.find(filter)

def find_cluster_by_id(cluster_id):
    return db.mongo_clusters.db.clusters.find_one({"_id": ObjectId(cluster_id)})

def find_cluster_by_name(name):
    return db.mongo_clusters.db.clusters.find_one({"cluster_name": name})

