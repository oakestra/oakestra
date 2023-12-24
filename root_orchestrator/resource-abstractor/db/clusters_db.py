from bson.objectid import ObjectId
from .clusters_helper import is_cluster_active

import db.mongodb_client as db


CLUSTERS_FRESHNESS_INTERVAL = 30

def find_clusters(filter):
    clusters = db.mongo_clusters.db.clusters.find(filter)
    for cluster in clusters:
        cluster['active'] = is_cluster_active(cluster)

    return clusters;

def find_cluster_by_id(cluster_id):
    cluster = find_clusters({"_id": ObjectId(cluster_id)})
    return cluster[0] if cluster else None

def find_cluster_by_name(name):
    cluster = find_clusters({"cluster_name": name})
    return cluster[0] if cluster else None
