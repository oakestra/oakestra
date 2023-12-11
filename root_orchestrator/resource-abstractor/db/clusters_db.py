from bson.objectid import ObjectId

from .mongodb_client import mongo_clusters


def find_clusters(filter):
    return mongo_clusters.db.clusters.find(filter)

def find_cluster_by_id(cluster_id):
    return mongo_clusters.db.clusters.find_one({"_id": ObjectId(cluster_id)})

def find_cluster_by_name(name):
    return mongo_clusters.db.clusters.find_one({"cluster_name": name})

