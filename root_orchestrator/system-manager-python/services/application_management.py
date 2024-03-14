import traceback

from rasclient import app_operations
from services.service_management import create_services_of_app, delete_service


def register_app(applications, userid):
    for application in applications["applications"]:
        if app_operations.get_app_by_name_and_namespace(
            application.get("application_name"), application.get("application_namespace"), userid
        ):
            return {
                "message": "An application with the same name and namespace exists already"
            }, 409
        if not valid_app_requirements(application):
            return {"message": "Application name or namespace are not in the valid format"}, 422

        application.pop("action", None)
        application.pop("_id", None)

        application["userId"] = userid
        microservices = application.get("microservices")
        application["microservices"] = []
        app = app_operations.create_app(userid, application)

        app_id = app.get("_id")
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

    return app_operations.get_user_apps(userid), 200


def update_app(appid, userid, fields):
    # TODO: fields validation before update
    app_data = {
        "application_name": fields.get("application_name"),
        "application_namespace": fields.get("application_namespace"),
        "application_desc": fields.get("application_desc", ""),
        "microservices": fields.get("microservices"),
    }
    return app_operations.update_app(appid, userid, app_data)


def delete_app(appid, userid):
    application = get_user_app(userid, appid)
    for service_id in application.get("microservices"):
        delete_service(userid, service_id)

    return app_operations.delete_app(appid)


def users_apps(userid):
    return app_operations.get_user_apps(userid)


def get_user_app(userid, appid):
    return app_operations.get_app_by_id(appid, userid)


def valid_app_requirements(app):
    if len(app["application_name"]) > 10 or len(app["application_name"]) < 1:
        return False
    if len(app["application_namespace"]) > 10 or len(app["application_namespace"]) < 1:
        return False
    return True


def get_all_applications():
    return app_operations.get_apps()
