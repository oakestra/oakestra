import ext_requests.mongodb_client as db
from bson import ObjectId
from rasclient import cluster_operations, job_operations

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
    new_job = job_operations.create_job(job_content)
    db.app.logger.info("MONGODB - job {} inserted".format(str(new_job.get("_id"))))
    return str(new_job.get("_id"))


def mongo_get_all_jobs():
    return job_operations.get_jobs()


def mongo_find_job_by_id(job_id):
    return job_operations.get_job_by_id(job_id)


def mongo_update_job_status(job_id, status, status_detail, instances=None):
    job = mongo_find_job_by_id(job_id)

    if job is None:
        return None

    if instances is not None:
        for instance in instances:
            job_operations.update_job_instance(job_id, instance["instance_number"], instance)

    return job_operations.update_job_status(job_id, status, status_detail)


def mongo_set_microservice_id(job_id):
    return job_operations.update_job(job_id, {"microserviceID": job_id})


def mongo_update_job_status_and_instances(
    job_id, status, next_instance_progressive_number, instance_list
):
    print("Updating Job Status and assigning a cluster for this job...")
    job_operations.update_job(
        job_id,
        {
            "status": status,
            "next_instance_progressive_number": next_instance_progressive_number,
            "instance_list": instance_list,
        },
    )


def mongo_get_jobs_of_application(app_id):
    return job_operations.get_jobs({"applicationID": app_id})


def mongo_update_job(job_id, job):
    db.app.logger.info("MONGODB - update job...")
    job = job_operations.update_job(job_id, job)
    db.app.logger.info("MONGODB - job {} updated")
    return job


def mongo_delete_job(job_id):
    db.app.logger.info("MONGODB - delete job...")
    job_operations.delete_job(job_id)
    db.app.logger.info("MONGODB - job {} deleted")
    # return mongo_frontend_jobs.find()


def mongo_find_cluster_of_job(job_id, instance_num):
    db.app.logger.info("Find job by Id and return cluster...")
    query = {}

    if instance_num != -1:
        query["instance_list"] = int(instance_num)

    job_obj = mongo_find_job_by_id(job_id)

    if job_obj is None:
        return None

    instances = job_obj.get("instance_list")
    if instances and len(instances) > 0:
        instance = job_obj["instance_list"][0]
        return cluster_operations.get_resource_by_id(instance["cluster_id"])


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
