from datetime import datetime

import ext_requests.mongodb_client as db
from bson import ObjectId


def mongo_upsert_cluster(cluster_ip, message):
    db.app.logger.info("MONGODB - upserting cluster...")
    clusters = db.mongo_clusters.db.clusters
    cluster_info = message["cluster_info"]
    cluster_name = message["cluster_name"]
    cluster_location = message["cluster_location"]
    cluster_port = message["manager_port"]
    clusters.update_one(
        {"cluster_name": cluster_name},
        {
            "$set": {
                "ip": cluster_ip,
                "clusterinfo": cluster_info,
                "port": cluster_port,
                "cluster_location": cluster_location,
            }
        },
        upsert=True,
    )

    cluster_obj = clusters.find_one({"cluster_name": cluster_name})

    db.app.logger.info("MONGODB - cluster_id: {0}".format(cluster_obj["_id"]))
    return cluster_obj["_id"]


def mongo_find_cluster_by_id(cluster_id):
    return db.mongo_clusters.db.clusters.find_one(ObjectId(cluster_id))


def mongo_find_cluster_by_ip(cluster_ip):
    return db.mongo_clusters.db.clusters.find_one({"ip": cluster_ip})


def mongo_get_all_clusters():
    return db.mongo_clusters.db.clusters.find()


def mongo_find_one_cluster():
    """Finds first cluster occurrence"""
    return db.mongo_clusters.db.clusters.find_one()


def mongo_find_all_active_clusters():
    db.app.logger.info("Finding the active cluster orchestrators...")
    now_timestamp = datetime.now().timestamp()
    return db.mongo_clusters.db.clusters.find(
        {"last_modified_timestamp": {"$gt": now_timestamp - db.CLUSTERS_FRESHNESS_INTERVAL}}
    )


def mongo_find_cluster_by_id_and_incr_node(c_id):
    return db.mongo_clusters.db.clusters.update_one(
        {"_id": c_id}, {"$inc": {"nodes": 1}}, upsert=True
    )


def mongo_find_cluster_by_id_and_set_number_of_nodes(c_id, number_of_nodes):
    return db.mongo_clusters.db.clusters.update_one(
        {"_id": c_id}, {"$inc": {"nodes": number_of_nodes}}, upsert=True
    )


def mongo_find_cluster_by_id_and_decr_node(c_id):
    return db.mongo_clusters.db.clusters.update_one(
        {"_id": c_id}, {"$inc": {"nodes": -1}}, upsert=True
    )


def mongo_find_cluster_by_location(location):
    try:
        return db.mongo_clusters.db.clusters.find_one({"cluster_location": location})
    except Exception:
        return "Error"


def mongo_update_cluster_information(cluster_id, data):
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

    db.mongo_clusters.db.clusters.find_one_and_update(
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
        upsert=True,
    )
