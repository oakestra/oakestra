from _datetime import datetime
from bson.objectid import ObjectId

CLUSTERS_FRESHNESS_INTERVAL = 30


def get_freshness_threshhold():
    now_timestamp = datetime.now().timestamp()
    return now_timestamp - CLUSTERS_FRESHNESS_INTERVAL


def build_filter(query):
    filter = query
    if filter.get("active"):
        filter["last_modified_timestamp"] = {"$gt": get_freshness_threshhold()}
    if filter.get("cluster_id"):
        filter["_id"] = ObjectId(filter.get("cluster_id"))

    filter.pop("cluster_id", None)
    filter.pop("job_id", None)
    filter.pop("active", None)
    return filter
