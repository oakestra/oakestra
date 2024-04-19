import db.mongodb_client as db
from bson.objectid import ObjectId

from .clusters_helper import get_freshness_threshhold


def find_clusters(filter):
    pipeline = [
        {"$match": filter},
        {
            "$addFields": {
                "active": {"$gt": ["$last_modified_timestamp", get_freshness_threshhold()]}
            }
        },
    ]
    return db.mongo_clusters.db.clusters.aggregate(pipeline)


def find_cluster_by_id(cluster_id):
    cluster = list(find_clusters({"_id": ObjectId(cluster_id)}))
    return cluster[0] if cluster else None


def find_cluster_by_name(name):
    cluster = list(find_clusters({"cluster_name": name}))
    return cluster[0] if cluster else None
