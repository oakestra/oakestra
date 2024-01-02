import traceback

from ext_requests.apps_db import (
    mongo_add_application,
    mongo_delete_application,
    mongo_find_app_by_id,
    mongo_find_app_by_name_and_namespace,
    mongo_get_all_applications,
    mongo_get_applications_of_user,
    mongo_update_application,
)
from services.service_management import create_services_of_app, delete_service


def register_app(applications, userid):
    for application in applications["applications"]:
        if mongo_find_app_by_name_and_namespace(
            application.get("application_name"),
            application.get("application_namespace"),
        ):
            return {
                "message": "An application with the same name and namespace exists already"
            }, 409
        if not valid_app_requirements(application):
            return {"message": "Application name or namespace are not in the valid format"}, 422

        if "action" in application:
            del application["action"]
        if "_id" in application:
            del application["_id"]
        application["userId"] = userid
        microservices = application.get("microservices")
        application["microservices"] = []
        app_id = mongo_add_application(application)

        # register microservices as well if any
        if app_id:
            if len(microservices) > 0:
                try:
                    application["microservices"] = microservices
                    application["applicationID"] = app_id
                    result, status = create_services_of_app(
                        userid,
                        {
                            "sla_version": applications["sla_version"],
                            "customerID": userid,
                            "applications": [application],
                        },
                    )
                    if status != 200:
                        delete_app(app_id, userid)
                        return result, status
                except Exception:
                    print(traceback.format_exc())
                    delete_app(app_id, userid)
                    return {"message": "error during the registration of the microservices"}, 500

    return list(mongo_get_applications_of_user(userid)), 200


def update_app(appid, userid, fields):
    # TODO: fields validation before update
    return mongo_update_application(appid, userid, fields)


def delete_app(appid, userid):
    application = get_user_app(userid, appid)
    for service_id in application.get("microservices"):
        delete_service(userid, service_id)
    return mongo_delete_application(appid, userid)


def users_apps(userid):
    return mongo_get_applications_of_user(userid)


def all_apps():
    return mongo_get_all_applications()


def get_user_app(userid, appid):
    return mongo_find_app_by_id(appid, userid)


def valid_app_requirements(app):
    if len(app["application_name"]) > 10 or len(app["application_name"]) < 1:
        return False
    if len(app["application_namespace"]) > 10 or len(app["application_namespace"]) < 1:
        return False
    return True
