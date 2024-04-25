from datetime import datetime

import db.mongodb_client as db
from bson.objectid import ObjectId
from db.clusters_helper import get_freshness_threshold

HISTORY_SLICE_SIZE = -100


def create_cluster(data):
    inserted = db.mongo_clusters.insert_one(data)

    return db.mongo_clusters.find_one({"_id": inserted.inserted_id})


def update_cluster(cluster_id, data):
    return db.mongo_clusters.find_one_and_update(
        {"_id": ObjectId(cluster_id)},
        {"$set": data},
        return_document=True,
    )


def find_clusters(filter):
    pipeline = [
        {"$match": filter},
        {
            "$addFields": {
                "active": {"$gt": ["$last_modified_timestamp", get_freshness_threshold()]}
            }
        },
    ]
    return db.mongo_clusters.aggregate(pipeline)


def find_cluster_by_id(cluster_id):
    cluster = list(find_clusters({"_id": ObjectId(cluster_id)}))
    return cluster[0] if cluster else None


def find_cluster_by_name(cluster_name):
    return db.mongo_clusters.find_one({"cluster_name": cluster_name})


def create_update_dict_from_mapping(data):
    # Define a mapping from data keys to database keys
    datetime_now = datetime.now()

    key_mapping = {
        "cpu_percent": "aggregated_cpu_percent",
        "cpu_cores": "total_cpu_cores",
        "memory_percent": "aggregated_memory_percent",
        "cumulative_memory_in_mb": "memory_in_mb",
        "number_of_nodes": "active_nodes",
        "gpu_cores": "total_gpu_cores",
        "gpu_percent": "total_gpu_percent",
        "virtualization": "virtualization",
        "more": "more",
        "worker_groups": "worker_groups",
        "aggregation_per_architecture": "aggregation_per_architecture",
    }

    # Use the mapping to create the update dictionary
    update_dict = {db_key: data.get(data_key) for data_key, db_key in key_mapping.items()}
    update_dict["last_modified"] = datetime_now
    update_dict["last_modified_timestamp"] = datetime.timestamp(datetime_now)

    return update_dict


def update_cluster_information(cluster_id, data):
    """Save aggregated Cluster Information"""

    update_dict = create_update_dict_from_mapping(data)

    cpu_update = {
        "value": data.get("cpu_percent"),
        "timestamp": update_dict["last_modified_timestamp"],
    }
    memory_update = {
        "value": data.get("memory_percent"),
        "timestamp": update_dict["last_modified_timestamp"],
    }

    return db.mongo_clusters.find_one_and_update(
        {"_id": ObjectId(cluster_id)},
        {
            "$push": {
                "cpu_history": {"$each": [cpu_update], "$slice": HISTORY_SLICE_SIZE},
                "memory_history": {"$each": [memory_update], "$slice": HISTORY_SLICE_SIZE},
            },
            "$set": update_dict,
        },
        return_document=True,
    )
