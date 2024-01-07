from datetime import datetime

import ext_requests.mongodb_client as db
from bson import ObjectId

# ....... Job operations .........
##################################


def mongo_insert_job(microservice):
    db.app.logger.info("MONGODB - insert job...")
    # jobname and details generation
    job_name = (
        microservice["app_name"]
        + "."
        + microservice["app_ns"]
        + "."
        + microservice["microservice_name"]
        + "."
        + microservice["microservice_namespace"]
    )
    microservice["job_name"] = job_name
    job_content = {
        "job_name": job_name,
        **microservice,  # The content of the input file
    }

    # job insertion
    new_job = db.mongo_services.find_one_and_update(
        {"job_name": job_name}, {"$set": job_content}, upsert=True, return_document=True
    )
    db.app.logger.info("MONGODB - job {} inserted".format(str(new_job.get("_id"))))
    return str(new_job.get("_id"))


def mongo_get_all_jobs():
    return db.mongo_services.find()


def mongo_get_job_status(job_id):
    return db.mongo_services.find_one({"_id": ObjectId(job_id)}, {"status": 1})["status"] + "\n"


def mongo_update_job_status(job_id, status, status_detail, instances=None):
    job = db.mongo_services.find_one({"_id": ObjectId(job_id)})
    if job is None:
        return None
    job.get("instance_list")
    if instances is not None:
        for instance in instances:
            current_time = datetime.now().isoformat()
            cpu_update = {"value": instance.get("cpu"), "timestamp": current_time}
            memory_update = {"value": instance.get("memory"), "timestamp": current_time}
            db.mongo_services.update_one(
                {
                    "_id": ObjectId(job_id),
                    "instance_list": {
                        "$elemMatch": {"instance_number": instance["instance_number"]}
                    },
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
                        "instance_list.$.cpu": instance.get("cpu"),
                        "instance_list.$.memory": instance.get("memory"),
                        "instance_list.$.publicip": instance.get("publicip"),
                        "instance_list.$.disk": instance.get("disk"),
                        "instance_list.$.status": instance.get("status"),
                        "instance_list.$.status_detail": instance.get(
                            "status_detail", "No extra information"
                        ),
                        "instance_list.$.logs": instance.get("logs", ""),
                    },
                },
            )

    return db.mongo_services.find_one_and_update(
        {"_id": ObjectId(job_id)},
        {"$set": {"status": status, "status_detail": status_detail}},
        return_document=True,
    )


def mongo_set_microservice_id(job_id):
    return db.mongo_services.update_one(
        {"_id": ObjectId(job_id)}, {"$set": {"microserviceID": job_id}}
    )


def mongo_update_job_net_status(job_id, instances):
    job = db.mongo_services.find_one({"_id": ObjectId(job_id)})
    instance_list = job["instance_list"]
    for instance in instances:
        instance_num = instance["instance_number"]
        elem = instance_list[instance_num]
        elem["namespace_ip"] = instance["namespace_ip"]
        elem["host_ip"] = instance["host_ip"]
        elem["host_port"] = instance["host_port"]
        instance_list[instance_num] = elem
    db.mongo_services.update_one(
        {"_id": ObjectId(job_id)}, {"$set": {"instance_list": instance_list}}
    )


def mongo_find_job_by_id(job_id):
    return db.mongo_services.find_one(ObjectId(job_id))


def mongo_find_job_by_name(job_name):
    return db.mongo_services.find_one({"job_name": job_name})


def mongo_find_job_by_ip(ip):
    # Search by Service Ip
    job = db.mongo_services.find_one({"service_ip_list.Address": ip})
    if job is None:
        # Search by instance ip
        job = db.mongo_services.find_one({"instance_list.instance_ip": ip})
    return job


def mongo_update_job_status_and_instances(
    job_id, status, next_instance_progressive_number, instance_list
):
    print("Updating Job Status and assigning a cluster for this job...")
    db.mongo_services.update_one(
        {"_id": ObjectId(job_id)},
        {
            "$set": {
                "status": status,
                "next_instance_progressive_number": next_instance_progressive_number,
                "instance_list": instance_list,
            }
        },
    )


def mongo_get_jobs_of_application(app_id):
    return db.mongo_services.aggregate([{"$match": {"applicationID": app_id}}])


def mongo_update_job(job_id, job):
    db.app.logger.info("MONGODB - update job...")
    job = db.mongo_services.find_one_and_update(
        {"_id": ObjectId(job_id)}, {"$set": job}, return_document=True
    )
    db.app.logger.info("MONGODB - job {} updated")
    return job


def mongo_delete_job(job_id):
    db.app.logger.info("MONGODB - delete job...")
    db.mongo_services.find_one_and_delete({"_id": ObjectId(job_id)})
    db.app.logger.info("MONGODB - job {} deleted")
    # return mongo_frontend_jobs.find()


def mongo_get_job_usage(job_id):
    db.app.logger.info("MONGODB - get usage...")
    job = db.mongo_services.find_one(ObjectId(job_id))
    if "usage" in job:
        return job["usage"]
    else:
        return None


def mongo_find_cluster_of_job(job_id, instance_num):
    db.app.logger.info("Find job by Id and return cluster...")
    job_obj = db.mongo_services.find_one(
        {"_id": ObjectId(job_id)}, {"instance_list": 1}
    )  # return just the assgined cluster of the job
    for instance in job_obj.get("instance_list"):
        if instance["instance_number"] == int(instance_num) or instance_num == -1:
            return db.mongo_clusters.db.clusters.find_one(ObjectId(instance["cluster_id"]))


# ......... APPLICATIONS .........
##################################


def mongo_add_application(application):
    db.app.logger.info("MONGODB - insert application...")
    application.get("userId")
    new_job = db.mongo_applications.insert_one(application)
    inserted_id = new_job.inserted_id
    db.app.logger.info("MONGODB - app {} inserted".format(str(inserted_id)))
    db.mongo_applications.find_one_and_update(
        {"_id": inserted_id}, {"$set": {"applicationID": str(inserted_id)}}
    )
    return str(inserted_id)


def mongo_get_all_applications():
    return db.mongo_applications.find()


def mongo_find_app_by_id(app_id, userid):
    app = db.mongo_applications.find_one(ObjectId(app_id))
    if app:
        if app.get("userId") == userid:
            return app


def mongo_find_app_by_name_and_namespace(app_name, app_ns):
    return db.mongo_applications.find_one(
        {"application_name": app_name, "application_namespace": app_ns}
    )


def mongo_update_application(app_id, userid, data):
    db.app.logger.info("MONGODB - update data...")
    db.mongo_applications.find_one_and_update(
        {"_id": ObjectId(app_id), "userId": userid},
        {
            "$set": {
                "application_name": data.get("application_name"),
                "application_namespace": data.get("application_namespace"),
                "application_desc": data.get("application_desc", ""),
                "microservices": data.get("microservices"),
            }
        },
    )

    db.app.logger.info("MONGODB - application {} updated")
    return db.mongo_applications.find()  # return the application list


def mongo_update_application_microservices(app_id, microservices):
    db.mongo_applications.find_one_and_update(
        {"_id": ObjectId(app_id)}, {"$set": {"microservices": microservices}}
    )


def mongo_delete_application(app_id, userid):
    db.mongo_applications.find_one_and_delete({"_id": ObjectId(app_id), "userId": userid})
    return db.mongo_applications.find()  # return the application list


def mongo_get_applications_of_user(user_id):
    return db.mongo_applications.aggregate([{"$match": {"userId": user_id}}])
