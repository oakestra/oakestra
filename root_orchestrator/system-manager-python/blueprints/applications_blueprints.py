from blueprints.schema_wrapper import SchemaWrapper
from bson import json_util
from ext_requests.user_db import mongo_get_user_by_name
from flask import request
from flask.views import MethodView
from flask_jwt_extended import get_jwt_identity, jwt_required
from flask_restful import Resource
from flask_smorest import Blueprint, abort
from roles.securityUtils import Role, get_jwt_organization, require_role

# ........ Functions for user management ...............#
# ......................................................#
from services.application_management import (
    delete_app,
    get_all_applications,
    get_user_app,
    get_user_apps,
    register_app,
    update_app,
)
from sla.schema import sla_schema

applicationblp = Blueprint(
    "Application operations",
    "applications",
    url_prefix="/api/application",
    description="Operations on single application",
)

applicationsblp = Blueprint(
    "Applications operations",
    "applications",
    url_prefix="/api/applications",
    description="Operations on multiple applications",
)

applications_schema = {"type": "array", "items": sla_schema}

application_update_schema = login_schema = {
    "type": "object",
    "properties": {
        "application_name": {"type": "string"},
        "application_namespace": {"type": "string"},
        "microservices": {"type": "string"},
    },
}


@applicationblp.route("/<appid>")
class ApplicationController(MethodView):
    @applicationblp.response(200, SchemaWrapper(sla_schema), content_type="application/json")
    @jwt_required()
    def get(self, appid, *args, **kwargs):
        try:
            current_user = get_jwt_identity()
            result, code = get_user_app(current_user, appid)
            if code != 200:
                abort(code, result)
                # TODO Frontend should be able to handle the _id being a string and not an object.
                return {**result, "_id": {"$oid": str(result["_id"])}}
        except Exception as e:
            return abort(404, {"message": e})

    @jwt_required()
    def delete(self, appid, *args, **kwargs):
        try:
            current_user = get_jwt_identity()
            res = delete_app(appid, current_user)
            if res:
                return {"message": "Application Deleted"}
            else:
                abort(501, {"message": "User could not be deleted"})
        except ConnectionError as e:
            abort(404, {"message": e})

    @jwt_required()
    def put(self, appid, *args, **kwargs):
        print(request.get_json())
        try:
            current_user = get_jwt_identity()
            update_app(appid, current_user, request.get_json())
            return {"message": "Application is updated"}
        except ConnectionError as e:
            abort(404, {"message": e})


@applicationblp.route("/")
class CreateApplicationController(Resource):
    @applicationblp.arguments(schema=sla_schema, location="json", validate=False, unknown=True)
    @applicationblp.response(
        200, SchemaWrapper(applications_schema), content_type="application/json"
    )
    @jwt_required()
    def post(self, *args, **kwargs):
        data = request.get_json()
        current_user = get_jwt_identity()
        result, code = register_app(data, current_user)
        if code != 200:
            abort(code, description=result)

        # TODO Frontend should be able to handle the _id being a string and not an object.
        for i in range(len(result)):
            result[i]["_id"] = {"$oid": result[i]["_id"]}

        return json_util.dumps(result)


@applicationsblp.route("/<userid>")
class MultipleApplicationControllerUser(Resource):
    @applicationblp.response(
        200, SchemaWrapper(applications_schema), content_type="application/json"
    )
    @jwt_required()
    def get(self, userid):
        current_user = get_jwt_identity()
        organization_id = get_jwt_organization()
        user = mongo_get_user_by_name(current_user, organization_id)
        if userid != str(user["_id"]):
            abort(401, {"message": "Unauthorized"})
        result, code = get_user_apps(current_user)
        if code != 200:
            abort(code, result)

        # TODO Frontend should be able to handle the _id being a string and not an object.
        for i in range(len(result)):
            result[i]["_id"] = {"$oid": result[i]["_id"]}

        return json_util.dumps(result)


# For the Admin to get all applications
@applicationsblp.route("/")
class MultipleApplicationController(Resource):
    @applicationblp.response(
        200, SchemaWrapper(applications_schema), content_type="application/json"
    )
    @jwt_required()
    @require_role(Role.ADMIN)
    def get(self, *args, **kwargs):
        result, code = get_all_applications()
        if code != 200:
            abort(code, result)

        # TODO Frontend should be able to handle the _id being a string and not an object.
        for i in range(len(result)):
            result[i]["_id"] = {"$oid": result[i]["_id"]}

        return json_util.dumps(result)
