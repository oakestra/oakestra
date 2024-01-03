import logging

from ext_requests.apps_db import (
    mongo_delete_job,
    mongo_find_app_by_id,
    mongo_find_job_by_id,
    mongo_get_all_jobs,
    mongo_get_applications_of_user,
    mongo_get_jobs_of_application,
    mongo_insert_job,
    mongo_set_microservice_id,
    mongo_update_application_microservices,
    mongo_update_job,
)
from ext_requests.net_plugin_requests import net_inform_service_deploy, net_inform_service_undeploy
from services.instance_management import request_scale_down_instance
from sla.versioned_sla_parser import parse_sla_json


def create_services_of_app(username, sla, force=False):
    data = parse_sla_json(sla)
    logging.log(logging.INFO, sla)
    app_id = data.get("applications")[0]["applicationID"]
    last_service_id = ""
    application = mongo_find_app_by_id(app_id, username)
    if application is None:
        return {"message": "application not found"}, 404
    for microservice in data.get("applications")[0].get("microservices"):
        if not valid_service(microservice):
            return {"message": "invalid service name or namespace"}, 403
        # Insert job into database
        service = generate_db_structure(application, microservice)
        last_service_id = mongo_insert_job(service)
        # Insert job into app's services list
        mongo_set_microservice_id(last_service_id)
        add_service_to_app(app_id, last_service_id, username)
        # Inform network plugin about the new service
        try:
            net_inform_service_deploy(service, str(last_service_id))
        except Exception:
            delete_service(username, str(last_service_id))
            return {"message": "failed to deploy service"}, 500
        # TODO: check if service deployed already etc. force=True must force the insertion anyway
    return {"job_id": str(last_service_id)}, 200


def delete_service(username, serviceid):
    apps = mongo_get_applications_of_user(username)
    for application in apps:
        if serviceid in application["microservices"]:
            # undeploy instances
            request_scale_down_instance(serviceid, username)
            # remove service from app's services list
            remove_service_from_app(application["applicationID"], serviceid, username)
            # remove service from DB
            mongo_delete_job(serviceid)
            # inform network component
            net_inform_service_undeploy(serviceid)
            return True
    return False


def update_service(username, sla, serviceid):
    # TODO Check fields and redeploy service
    apps = mongo_get_applications_of_user(username)
    for application in apps:
        if serviceid in application["microservices"]:
            return mongo_update_job(serviceid, sla), 200
    return {"message": "service not found"}, 404


def user_services(appid, username):
    application = mongo_find_app_by_id(appid, username)
    if application is None:
        return {"message": "app not found"}, 404

    return mongo_get_jobs_of_application(appid), 200


def get_service(serviceid, username):
    apps = mongo_get_applications_of_user(username)
    for application in apps:
        if serviceid in application["microservices"]:
            return mongo_find_job_by_id(serviceid)


def get_all_services():
    return mongo_get_all_jobs()


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
    if microservice["virtualization"] == "unikernel":
        microservice["arch"] = microservice["arch"]
    return microservice


def add_service_to_app(app_id, service_id, userid):
    application = mongo_find_app_by_id(app_id, userid)
    application["microservices"].append(service_id)
    mongo_update_application_microservices(app_id, application["microservices"])


def remove_service_from_app(app_id, service_id, userid):
    application = mongo_find_app_by_id(app_id, userid)
    application["microservices"].remove(service_id)
    mongo_update_application_microservices(app_id, application["microservices"])


def valid_service(service):
    if len(service["microservice_name"]) > 10 or len(service["microservice_name"]) < 1:
        return False
    if len(service["microservice_namespace"]) > 10 or len(service["microservice_namespace"]) < 1:
        return False
    return True
