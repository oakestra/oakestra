import secrets
from datetime import datetime
from bson import json_util
from flask import request, Response, current_app
from flask_restful import Resource
from roles.securityUtils import jwt_auth_required, identity_is_username, require_role, Role


# ........ Functions for user management ...............#
# ......................................................#
from users.auth import user_change_password, user_create_password_reset_request, user_change_password_with_reset_request
from users.user_management import user_get_by_name, user_delete, user_add, user_get_all


class UserController(Resource):

    @staticmethod
    @jwt_auth_required()
    @identity_is_username()
    def get(username):
        return Response(
            response=json_util.dumps(user_get_by_name(username)),
            status=200,
            mimetype="application/json"
        )

    @staticmethod
    @jwt_auth_required()
    @require_role(Role.ADMIN)
    def delete(username):
        return Response(
            response=json_util.dumps(user_delete(username)),
            status=200,
            mimetype="application/json"
        )

    @staticmethod
    @jwt_auth_required()
    @require_role(Role.ADMIN)
    def put(username):
        return user_add(username, request.get_json())


class AllUserController(Resource):

    @staticmethod
    @jwt_auth_required()
    @require_role(Role.ADMIN)
    def get():
        return Response(
            response=json_util.dumps(user_get_all()),
            status=200,
            mimetype="application/json")


class UserChangePasswordController(Resource):

    @staticmethod
    @jwt_auth_required()
    @identity_is_username()
    def post(username):
        content = request.get_json()
        return user_change_password(username, content['oldPassword'], content['newPassword'])


class UserResetPasswordController(Resource):

    @staticmethod
    def post():
        content = request.get_json()
        username = content['username']
        domain = content['domain']
        expires = current_app.config['RESET_TOKEN_EXPIRES']
        expiry_date = datetime.now() + expires
        reset_token = secrets.token_urlsafe()

        return user_create_password_reset_request(username, domain, reset_token, expiry_date)

    @staticmethod
    def put():
        content = request.get_json()
        return user_change_password_with_reset_request(content["token"],content["password"])


class UserRolesController(Resource):
    @staticmethod
    def get():
        roles = [{"name": "Admin", "description": "This is the admin role"},
                 {"name": "Application_Provider", "description": "This is the app role"},
                 {"name": "Infrastructure_Provider", "description": "This is the infra role"}]
        return Response(
            response=json_util.dumps(roles),
            status=200,
            mimetype="application/json"
        )
