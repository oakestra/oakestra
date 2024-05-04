from datetime import datetime

import db.mongodb_client as db
from bson.objectid import ObjectId


def find_apps(filter={}):
    return db.mongo_apps.find(filter)


def find_app_by_id(app_id, extra_filter={}):
    filter = {**extra_filter, "_id": ObjectId(app_id)}
    app = list(find_apps(filter=filter))

    return app[0] if app else None


def delete_app(app_id):
    filter = {"_id": ObjectId(app_id)}
    return db.mongo_apps.find_one_and_delete(filter, return_document=True)


def update_app(app_id, data):
    data.pop("_id", None)

    return db.mongo_apps.find_one_and_update(
        {"_id": ObjectId(app_id)},
        {"$set": data},
        return_document=True,
    )


def create_app(app_data):
    inserted = db.mongo_apps.insert_one(app_data)

    return update_app(str(inserted.inserted_id), {"applicationID": str(inserted.inserted_id)})


# Job operations ##############################################################
def find_jobs(filter={}):
    return db.mongo_jobs.find(filter)


def find_job_by_id(job_id, filter={}):
    filter["_id"] = ObjectId(job_id)
    job = list(find_jobs(filter=filter))

    return job[0] if job else None


def find_job_by_name(job_name):
    jobs = list(find_jobs(filter={"job_name": job_name}))

    return jobs[0] if jobs else None


def delete_job(job_id):
    return db.mongo_jobs.find_one_and_delete({"_id": ObjectId(job_id)})


def update_job(job_id, job_data):
    job_data.pop("_id", None)

    return db.mongo_jobs.find_one_and_update(
        {"_id": ObjectId(job_id)}, {"$set": job_data}, return_document=True
    )


def update_job_instance(job_id, instance_number, job_data):
    job_data.pop("_id", None)

    current_time = datetime.now().isoformat()
    cpu_update = {"value": job_data.get("cpu"), "timestamp": current_time}
    memory_update = {"value": job_data.get("memory"), "timestamp": current_time}

    return db.mongo_jobs.update_one(
        {
            "_id": ObjectId(job_id),
            "instance_list": {"$elemMatch": {"instance_number": instance_number}},
        },
        {
            "$push": {
                "instance_list.$.cpu_history": {
                    "$each": [cpu_update],
                    "$slice": -100,
                },
                "instance_list.$.memory_history": {
                    "$each": [memory_update],
                    "$slice": -100,
                },
            },
            "$set": {
                "instance_list.$.cpu": job_data.get("cpu"),
                "instance_list.$.memory": job_data.get("memory"),
                "instance_list.$.publicip": job_data.get("publicip"),
                "instance_list.$.disk": job_data.get("disk"),
                "instance_list.$.status": job_data.get("status"),
                "instance_list.$.status_detail": job_data.get(
                    "status_detail", "No extra information"
                ),
                "instance_list.$.logs": job_data.get("logs", ""),
            },
        },
    )


def create_job(job_data):
    inserted = db.mongo_jobs.insert_one(job_data)

    return db.mongo_jobs.find_one({"_id": inserted.inserted_id})
