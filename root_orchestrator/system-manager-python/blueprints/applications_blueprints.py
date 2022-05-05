import traceback

from bson import json_util
from flask import request, Response, jsonify
from flask_restful import Resource
from flask_jwt_extended import jwt_required

from services.application_management import *
from roles.securityUtils import *


# ........ Functions for user management ...............#
# ......................................................#
from services.application_management import register_app


class ApplicationController(Resource):

    @staticmethod
    @jwt_required()
    def get(appid):
        try:
            current_user = get_jwt_identity()
            application = json_util.dumps(get_user_app(appid, current_user))
            return Response(
                response=application,
                status=200,
                mimetype="application/json"
            )
        except Exception as e:
            tb = traceback.format_exc()
            print(tb)
            return {"message": e.message}, 404

    @staticmethod
    @jwt_required()
    def delete(appid):
        try:
            current_user = get_jwt_identity()
            res = delete_app(appid, current_user)
            if res:
                resp = jsonify(
                    {"message": "Application Deleted"})
                resp.status_code = 200
                return resp
            else:
                resp = jsonify(
                    {"message": "User could not be deleted"})
                resp.status_code = 501
                return resp
        except ConnectionError as e:
            resp = jsonify({"message": e.message})
            resp.status_code = 404
            return resp

    @staticmethod
    @jwt_required()
    def put(appid):
        try:
            current_user = get_jwt_identity()
            update_app(appid, current_user, request.get_json())
            return Response(
                response=json_util.dumps({"message": "Application is updated"}),
                status=200
            )
        except ConnectionError as e:
            resp = json_util.dumps({"message": e.message})
            resp.status_code = 404
            return resp

    @staticmethod
    @jwt_required()
    def post():
        data = request.get_json()
        if "action" in data:
            del data['action']
        if "_id" in data:
            del data['_id']
        current_user = get_jwt_identity()
        return str(json_util.dumps(register_app(data, current_user))), 200


class MultipleApplicationControllerUser(Resource):

    @staticmethod
    @jwt_required()
    def get(userid):
        current_user = get_jwt_identity()
        if userid != current_user:
            resp = json_util.dumps({"message": "Unauthorized"})
            resp.status_code = 401
            return resp
        return Response(
            response=json_util.dumps(users_apps(current_user)),
            status=200,
            mimetype="application/json"
        )


# For the Admin to get all applications
class MultipleApplicationController(Resource):

    @staticmethod
    @jwt_required()
    @require_role(Role.ADMIN)
    def get():
        return Response(
            response=json_util.dumps(mongo_get_all_applications()),
            status=200,
            mimetype="application/json"
        )
