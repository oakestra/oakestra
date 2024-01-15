import db.mongodb_client as db
from bson.objectid import ObjectId


def find_all_jobs():
    return db.mongo_jobs.db.jobs.find()


def find_job_by_id(job_id):
    return db.mongo_jobs.db.jobs.find_one({"_id": ObjectId(job_id)})


def update_job(job_id, job_data):
    return db.mongo_jobs.db.jobs.update_one({"_id": ObjectId(job_id)}, {"$set": job_data})
