import logging

from ext_requests.net_plugin_requests import net_inform_service_deploy, net_inform_service_undeploy
from resource_abstractor_client import app_operations, job_operations
from services.instance_management import request_scale_down_instance
from sla.versioned_sla_parser import parse_sla_json


def insert_job(microservice):
    logging.log(logging.INFO, "MONGODB - insert job...")
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
    if new_job is None:
        logging.error(f"job not inserted - {job_name}")
        return None

    logging.info("job {} inserted".format(str(new_job.get("_id"))))
    return str(new_job.get("_id"))


def create_services_of_app(username, sla, force=False):
    data = parse_sla_json(sla)
    logging.log(logging.INFO, sla)
    app_id = data.get("applications")[0]["applicationID"]
    last_service_id = ""
    application = app_operations.get_app_by_id(app_id, username)

    if application is None:
        return {"message": "application not found"}, 404

    deployed_services = []
    failed_services = []
    for microservice in data.get("applications")[0].get("microservices"):
        if not valid_service(microservice):
            return {"message": "invalid service name or namespace"}, 403
        # Insert job into database
        service = generate_db_structure(application, microservice)
        last_service_id = insert_job(service)
        if last_service_id is None:
            logging.warning(
                f"service not inserted for app-{app_id}, service-{service['service_name']}"
            )
            # TODO: add a reason why it failed.
            failed_services.append({"service_name": service["service_name"], "status": 500})
            continue

        # Insert job into app's services list
        # TODO what should be done if updating job or application fails?
        job_operations.update_job(last_service_id, {"microserviceID": last_service_id})
        add_service_to_app(app_id, last_service_id, username)
        try:
            # Inform network plugin about the new service
            net_inform_service_deploy(service, str(last_service_id))
            deployed_services.append({"service_name": service["service_name"], "status": 200})
        except Exception:
            delete_service(username, str(last_service_id))
            failed_services.append(
                {
                    "service_name": service["service_name"],
                    "message": "failed to deploy service",
                    "status": 500,
                }
            )

        # TODO: check if service deployed already etc. force=True must force the insertion anyway
    return {
        "job_id": str(last_service_id),
        "deployed_services": deployed_services,
        "failed_services": failed_services,
    }, 200


def delete_job(job_id):
    logging.info("delete job...")
    job_operations.delete_job(job_id)


def delete_service(username, serviceid):
    apps = app_operations.get_user_apps(username)
    for application in apps:
        if serviceid in application["microservices"]:
            request_scale_down_instance(serviceid, username)
            remove_service_from_app(application["applicationID"], serviceid, username)
            delete_job(serviceid)
            net_inform_service_undeploy(serviceid)
            return True
    return False


def update_service(username, sla, serviceid):
    # TODO Check fields and redeploy service
    # TODO this function is currently causing a lof of issues as such it is commented it out.
    # https://github.com/oakestra/oakestra/pull/282#discussion_r1526433174

    # apps = app_operations.get_user_apps(username)
    # for application in apps:
    #     if serviceid in application["microservices"]:
    #         logging.log(logging.INFO, f"update job - {serviceid}...")

    #         job = job_operations.update_job(serviceid, sla)
    #         if job is None:
    #             logging.log(logging.ERROR, "job not updated")
    #             continue

    #         logging.log(logging.INFO, "job {} updated")
    #         return job, 200

    return {"message": "service not found"}, 404


def user_services(appid, username):
    application = app_operations.get_app_by_id(appid, username)
    if application is None:
        return {"message": "app not found"}, 404

    return job_operations.get_jobs_of_application(appid), 200


def get_service(serviceid, username):
    apps = app_operations.get_user_apps(username)
    for application in apps:
        if serviceid in application["microservices"]:
            return job_operations.get_job_by_id(serviceid)


def get_all_services():
    return job_operations.get_jobs()


def generate_db_structure(application, microservice):
    microservice["applicationID"] = application["applicationID"]
    microservice["app_name"] = application["application_name"]
    microservice["app_ns"] = application["application_namespace"]
    microservice["service_name"] = microservice["microservice_name"]
    microservice["service_ns"] = microservice["microservice_namespace"]
    microservice["image"] = microservice["code"]
    microservice["next_instance_progressive_number"] = 0
    microservice["instance_list"] = []
    if microservice["virtualization"] == "container":
        microservice["virtualization"] = "docker"
    addresses = microservice.get("addresses")
    if addresses is not None:
        microservice["RR_ip"] = addresses.get(
            "rr_ip"
        )  # compatibility with older netmanager versions
        microservice["RR_ip_v6"] = addresses.get("rr_ip_v6")
    if microservice["virtualization"] == "unikernel":
        microservice["arch"] = microservice["arch"]
    return microservice


def add_service_to_app(app_id, service_id, userid):
    application = app_operations.get_app_by_id(app_id, userid)
    application["microservices"].append(service_id)
    app_operations.update_app(app_id, userid, {"microservices": application["microservices"]})


def remove_service_from_app(app_id, service_id, userid):
    application = app_operations.get_app_by_id(app_id, userid)
    application["microservices"].remove(service_id)
    app_operations.update_app(app_id, userid, {"microservices": application["microservices"]})


def valid_service(service):
    if len(service["microservice_name"]) > 10 or len(service["microservice_name"]) < 1:
        return False
    if len(service["microservice_namespace"]) > 10 or len(service["microservice_namespace"]) < 1:
        return False
    return True
