import ext_requests.mongodb_client as db
from rasclient import app_operations, job_operations

# ....... Job operations .........
##################################


# TODO: only kept for the testing purposes. Remove it once new tests are added.
def mongo_find_job_by_id(job_id):
    return job_operations.get_job_by_id(job_id)


# TODO: only kept for the testing purposes. Remove it once new tests are added.
def mongo_get_jobs_of_application(app_id):
    return job_operations.get_jobs({"applicationID": app_id})


# ......... APPLICATIONS .........
##################################


# TODO kept for testing purposes. Remove it once new tests are added.
def mongo_add_application(application):
    db.app.logger.info("MONGODB - insert application...")

    if application.get("userId") is None:
        return None

    new_job = app_operations.create_app(application.get("userId"), application)
    inserted_id = new_job.inserted_id
    db.app.logger.info("MONGODB - app {} inserted".format(str(inserted_id)))

    return str(inserted_id)


# TODO kept for testing purposes. Remove it once new tests are added.
def mongo_find_app_by_name_and_namespace(app_name, app_ns):
    return db.mongo_applications.find_one(
        {"application_name": app_name, "application_namespace": app_ns}
    )
