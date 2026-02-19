from _datetime import datetime

from bson.objectid import ObjectId

CANDIDATES_FRESHNESS_INTERVAL = 30


def get_freshness_threshold():
    now_timestamp = datetime.now().timestamp()
    return now_timestamp - CANDIDATES_FRESHNESS_INTERVAL


def build_filter(query):
    filter = query
    if filter.get("active"):
        filter["last_modified_timestamp"] = {"$gt": get_freshness_threshold()}
    if filter.get("candidate_id"):
        filter["_id"] = ObjectId(filter.get("candidate_id"))

    filter.pop("candidate_id", None)
    filter.pop("job_id", None)
    filter.pop("active", None)
    return filter
