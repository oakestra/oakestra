from datetime import datetime

from bson.objectid import ObjectId

import db.mongodb_client as db
from db.candidates_helper import get_freshness_threshold

HISTORY_SLICE_SIZE = -100
CANONICAL_RESOURCES = [
    "_id",
    "cpu_percent",
    "vcpus",
    "memory_percent",
    "vram",
    "vram_percent",
    "gpu_temp",
    "gpu_drivers",
    "gpu_percent",
    "vgpus",
    "memory",
    "virtualization",
    "supported_addons",
    "active_nodes",
    "ip",
    "port",
    "candidate_location",
    "candidate_name",
    "cpu_history",
    "memory_history",
    "csi_drivers",
]


def create_candidate(data):
    inserted = db.mongo_candidates.insert_one(data)

    return db.mongo_candidates.find_one({"_id": inserted.inserted_id})


def update_candidate(candidate_id, data):
    data.pop("_id", None)

    return db.mongo_candidates.find_one_and_update(
        {"_id": ObjectId(candidate_id)},
        {"$set": data},
        return_document=True,
    )


def find_candidates(filter, resources=None):
    pipeline = [
        {"$match": filter},
        {
            "$addFields": {
                "active": {"$gt": ["$last_modified_timestamp", get_freshness_threshold()]}
            }
        },
    ]

    request = set(CANONICAL_RESOURCES)
    if resources:
        resources = [r.strip() for r in resources.split(",") if r.strip()]
        request.update(resources)

    print("Request: ", request, flush=True)

    projection = {field: 1 for field in request}
    pipeline.append({"$project": projection})

    return db.mongo_candidates.aggregate(pipeline)


def find_candidate_by_id(candidate_id):
    candidate = list(find_candidates({"_id": ObjectId(candidate_id)}))
    return candidate[0] if candidate else None


def find_candidate_by_name(candidate_name):
    return db.mongo_candidates.find_one({"candidate_name": candidate_name})


def update_timestamp(data):
    datetime_now = datetime.now()

    if "_id" in data:
        data.pop("_id")

    data["last_modified"] = datetime_now
    data["last_modified_timestamp"] = datetime.timestamp(datetime_now)
    return data


def update_candidate_information(candidate_id, data):
    """Save aggregated Candidate Information"""

    update_dict = update_timestamp(data)

    # remove duplicate history values
    if "cpu_history" in update_dict:
        del update_dict["cpu_history"]
    if "memory_history" in update_dict:
        del update_dict["memory_history"]

    cpu_update = {
        "value": data.get("cpu_percent"),
        "timestamp": update_dict["last_modified_timestamp"],
    }
    memory_update = {
        "value": data.get("memory_percent"),
        "timestamp": update_dict["last_modified_timestamp"],
    }

    return db.mongo_candidates.find_one_and_update(
        {"_id": ObjectId(candidate_id)},
        {
            "$push": {
                "cpu_history": {"$each": [cpu_update], "$slice": HISTORY_SLICE_SIZE},
                "memory_history": {"$each": [memory_update], "$slice": HISTORY_SLICE_SIZE},
            },
            "$set": update_dict,
        },
        return_document=True,
    )


def delete_candidate(candidate_id):
    return db.mongo_candidates.delete_one({"_id": ObjectId(candidate_id)})
