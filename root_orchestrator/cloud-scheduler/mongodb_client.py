import os

from _datetime import datetime
from bson.objectid import ObjectId
from flask_pymongo import PyMongo

MONGO_URL = os.environ.get("CLOUD_MONGO_URL")
MONGO_PORT = os.environ.get("CLOUD_MONGO_PORT")

MONGO_ADDR_CLUSTERS = "mongodb://" + str(MONGO_URL) + ":" + str(MONGO_PORT) + "/clusters"
MONGO_ADDR_JOBS = "mongodb://" + str(MONGO_URL) + ":" + str(MONGO_PORT) + "/jobs"

mongo_clusters = None
mongo_jobs = None

app = None

CLUSTERS_FRESHNESS_INTERVAL = 30


def mongo_init(flask_app):
    global mongo_clusters, mongo_jobs
    global app

    app = flask_app
    app.config["MONGO_URI"] = MONGO_ADDR_CLUSTERS

    mongo_clusters = PyMongo(app, uri=MONGO_ADDR_CLUSTERS)
    mongo_jobs = PyMongo(app, uri=MONGO_ADDR_JOBS)

    app.logger.info("init mongo")


# .................... Cluster operations ................#
###########################################################


def mongo_insert(obj):
    global mongo_clusters
    app.logger.info("inserting...")
    clusters = mongo_clusters.db.clusters
    cluster_id = clusters.insert_one(obj).inserted_id
    app.logger.info(cluster_id)
    return cluster_id


def mongo_find_cluster_by_id(id):
    global mongo_clusters
    return mongo_clusters.db.clusters.find_one(id).inserted_id


def mongo_find_any_cluster():
    """Finds first cluster occurrence"""
    global mongo_clusters
    return mongo_clusters.db.clusters.find_one()


def mongo_find_cluster_by_id_and_update(id, key, value):
    global mongo_clusters
    print("update..")
    o = mongo_clusters.db.clusters.find_one({"_id": id})
    print(o)

    mongo_clusters.db.clusters.find_one_and_update(
        {"_id": ObjectId(id)}, {"$set": {key: value}}, upsert=True
    )

    return 1


def mongo_find_cluster_by_name(name):
    global mongo_clusters
    cluster = mongo_clusters.db.clusters.find_one({"cluster_name": name})
    return cluster


def mongo_find_cluster_by_location(location):
    global mongo_clusters
    cluster = mongo_clusters.db.clusters.find_one({"cluster_location": location})
    return cluster


def is_cluster_active(cluster):
    print("check cluster activity...")
    timestamp_now = datetime.now().timestamp()
    last_modified_cluster = cluster.get("last_modified_timestamp")
    if last_modified_cluster >= timestamp_now - CLUSTERS_FRESHNESS_INTERVAL:
        return True
    else:
        return False


def mongo_find_all_active_clusters():
    global mongo_clusters
    app.logger.info("Finding the active cluster orchestrators...")
    now_timestamp = datetime.now().timestamp()
    return mongo_clusters.db.clusters.find(
        {"last_modified_timestamp": {"$gt": now_timestamp - CLUSTERS_FRESHNESS_INTERVAL}}
    )


# .................. JOB operations ....................
########################################################


def mongo_find_job_by_id(job_id):
    print("Find job by Id and return cluster.. and delete it...")
    # return just the assigned node of the job
    job_obj = mongo_jobs.db.jobs.find_one({"system_job_id": job_id})
    return job_obj


def find_cluster_by_job(job_id):
    job_obj = mongo_find_job_by_id(job_id)
    cluster_id = job_obj.get("cluster")
    return mongo_find_cluster_by_id(cluster_id)


def mongo_update_job_status(job_id, status):
    global mongo_jobs
    print("updating job status...")
    return mongo_jobs.db.jobs.update_one({"_id": ObjectId(job_id)}, {"$set": {"status": status}})


def mongo_update_job_status_and_cluster(job_id, status, cluster_id):
    global mongo_jobs
    print("Updating Job Status and assgined cluster for this job...")
    mongo_jobs.db.jobs.update_one(
        {"_id": ObjectId(job_id)},
        {"$set": {"status": status, "cluster": cluster_id, "replicas": 1}},
    )
