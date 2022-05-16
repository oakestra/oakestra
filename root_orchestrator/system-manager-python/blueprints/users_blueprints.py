import secrets
from datetime import datetime
from bson import json_util
from flask import request, Response, current_app
from flask.views import MethodView
from flask_restful import Resource
from roles.securityUtils import jwt_auth_required, identity_is_username, require_role, Role
from flask_smorest import Blueprint, Api, abort

# ........ Functions for user management ...............#
# ......................................................#
from users.auth import user_change_password, user_create_password_reset_request, user_change_password_with_reset_request
from users.user_management import user_get_by_name, user_delete, user_add, user_get_all

userbp = Blueprint(
    'User Operations', 'user', url_prefix='/api/user',
    description='Operations on single user'
)

usersbp = Blueprint(
    'Multiple Users Operations', 'users', url_prefix='/api/users',
    description='Operations on multiple users'
)


@userbp.route('/<username>')
class UserController(MethodView):

    @jwt_auth_required()
    @identity_is_username()
    def get(self, username, *args, **kwargs):
        return json_util.dumps(user_get_by_name(username))

    @jwt_auth_required()
    @require_role(Role.ADMIN)
    def delete(self, username, *args, **kwargs):
        return json_util.dumps(user_delete(username))

    @jwt_auth_required()
    @require_role(Role.ADMIN)
    def put(self, username, *args, **kwargs):
        return json_util.dumps(user_add(username, request.get_json()))


@usersbp.route('/')
class AllUserController(MethodView):

    @jwt_auth_required()
    @require_role(Role.ADMIN)
    def get(self, *args, **kwargs):
        return json_util.dumps(user_get_all())


@userbp.route('/<username>')
class UserChangePasswordController(MethodView):

    @jwt_auth_required()
    @identity_is_username()
    def post(self, username, *args, **kwargs):
        content = request.get_json()
        return user_change_password(username, content['oldPassword'], content['newPassword'])


@userbp.route('/')
class UserResetPasswordController(MethodView):

    def post(self, *args, **kwargs):
        content = request.get_json()
        username = content['username']
        domain = content['domain']
        expires = current_app.config['RESET_TOKEN_EXPIRES']
        expiry_date = datetime.now() + expires
        reset_token = secrets.token_urlsafe()

        return user_create_password_reset_request(username, domain, reset_token, expiry_date)

    def put(self, *args, **kwargs):
        content = request.get_json()
        return user_change_password_with_reset_request(content["token"], content["password"])
