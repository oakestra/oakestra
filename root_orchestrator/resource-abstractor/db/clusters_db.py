from datetime import datetime

import db.mongodb_client as db
from bson.objectid import ObjectId
from clusters_helper import get_freshness_threshhold


def create_cluster(data):
    db.mongo_clusters.update_one(
        {"cluster_name": data["cluster_name"]},
        {
            "$set": {
                "ip": data["cluster_ip"],
                "clusterinfo": data["cluster_info"],
                "port": data["manager_port"],
                "cluster_location": data["cluster_location"],
            }
        },
        upsert=True,
    )

    return data


def find_clusters(filter):
    pipeline = [
        {"$match": filter},
        {
            "$addFields": {
                "active": {"$gt": ["$last_modified_timestamp", get_freshness_threshhold()]}
            }
        },
    ]
    return db.mongo_clusters.aggregate(pipeline)


def find_cluster_by_id(cluster_id):
    cluster = list(find_clusters({"_id": ObjectId(cluster_id)}))
    return cluster[0] if cluster else None


def update_cluster_information(cluster_id, data):
    """Save aggregated Cluster Information"""

    datetime_now = datetime.now()
    datetime_now_timestamp = datetime.timestamp(datetime_now)

    cpu_percent = data.get("cpu_percent")
    cpu_cores = data.get("cpu_cores")
    memory_percent = data.get("memory_percent")
    memory_in_mb = data.get("cumulative_memory_in_mb")
    nodes = data.get("number_of_nodes")
    gpu_cores = data.get("gpu_cores")
    gpu_percent = data.get("gpu_percent")
    # technology = data.get('technology')
    virtualization = data.get("virtualization")
    more = data.get("more")
    worker_groups = data.get("worker_groups")
    cpu_update = {"value": cpu_percent, "timestamp": datetime_now_timestamp}
    memory_update = {"value": memory_percent, "timestamp": datetime_now_timestamp}

    aggregation_per_architecture = data.get("aggregation_per_architecture", {})

    return db.mongo_clusters.find_one_and_update(
        {"_id": ObjectId(cluster_id)},
        {
            "$push": {
                "cpu_history": {"$each": [cpu_update], "$slice": -100},
                "memory_history": {"$each": [memory_update], "$slice": -100},
            },
            "$set": {
                "aggregated_cpu_percent": cpu_percent,
                "total_cpu_cores": cpu_cores,
                "total_gpu_cores": gpu_cores,
                "total_gpu_percent": gpu_percent,
                "aggregated_memory_percent": memory_percent,
                "memory_in_mb": memory_in_mb,
                "active_nodes": nodes,
                "aggregation_per_architecture": aggregation_per_architecture,
                "virtualization": virtualization,
                "more": more,
                "last_modified": datetime_now,
                "last_modified_timestamp": datetime_now_timestamp,
                "worker_groups": worker_groups,
            },
        },
        return_document=True,
    )
