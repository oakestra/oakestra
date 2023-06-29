import json
import traceback

from bson import json_util
from flask import request, Response, jsonify
from flask.views import MethodView
from flask_restful import Resource
from flask_jwt_extended import jwt_required
from flask_smorest import Blueprint, Api, abort

from blueprints.schema_wrapper import SchemaWrapper
from services.application_management import *
from roles.securityUtils import *

from sla.schema import sla_schema

# ........ Functions for user management ...............#
# ......................................................#
from services.application_management import register_app

applicationblp = Blueprint(
    'Application operations', 'applications', url_prefix='/api/application',
    description='Operations on single application'
)

applicationsblp = Blueprint(
    'Applications operations', 'applications', url_prefix='/api/applications',
    description='Operations on multiple applications'
)

applications_schema = {
    "type": "array",
    "items": sla_schema
}

application_update_schema = login_schema = {
    "type": "object",
    "properties": {
        "application_name": {"type": "string"},
        "application_namespace": {"type": "string"},
        "microservices": {"type": "string"},
    }
}


@applicationblp.route('/<appid>')
class ApplicationController(MethodView):

    @applicationblp.response(200, SchemaWrapper(sla_schema), content_type="application/json")
    @jwt_required()
    def get(self, appid, *args, **kwargs):
        try:
            current_user = get_jwt_identity()
            return json_util.dumps(get_user_app(current_user, appid))
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


@applicationblp.route('/')
class ApplicationController(Resource):
    @applicationblp.arguments(schema=sla_schema, location="json", validate=False, unknown=True)
    @applicationblp.response(200, SchemaWrapper(applications_schema), content_type="application/json")
    @jwt_required()
    def post(self, *args, **kwargs):
        data = request.get_json()
        current_user = get_jwt_identity()
        result, code = register_app(data, current_user)
        if code != 200:
            abort(code, result)
        return json_util.dumps(result)


@applicationsblp.route('/<userid>')
class MultipleApplicationControllerUser(Resource):

    @applicationblp.response(200, SchemaWrapper(applications_schema), content_type="application/json")
    @jwt_required()
    def get(self, userid):
        current_user = get_jwt_identity()
        organization_id = get_jwt_organization()
        user = mongo_get_user_by_name(current_user, organization_id)
        if userid != str(user['_id']):
            abort(401, {"message": "Unauthorized"})
        return json_util.dumps(users_apps(current_user))


# For the Admin to get all applications
@applicationsblp.route('/')
class MultipleApplicationController(Resource):

    @applicationblp.response(200, SchemaWrapper(applications_schema), content_type="application/json")
    @jwt_required()
    @require_role(Role.ADMIN)
    def get(self, *args, **kwargs):
        return json_util.dumps(mongo_get_all_applications())
