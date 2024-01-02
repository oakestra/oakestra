import secrets
from datetime import datetime

from bson import json_util
from flask import current_app, request
from flask.views import MethodView
from flask_smorest import Blueprint
from roles.securityUtils import (
    Role,
    get_jwt_organization,
    identity_is_username,
    jwt_auth_required,
    require_role,
)

# ........ Functions for user management ...............#
# ......................................................#
from users.auth import (
    user_change_password,
    user_change_password_with_reset_request,
    user_create_password_reset_request,
)
from users.user_management import (
    user_add,
    user_delete,
    user_get_all,
    user_get_all_from_Organization,
    user_get_by_name,
)

userbp = Blueprint(
    "User Operations",
    "user",
    url_prefix="/api/user",
    description="Operations on single user",
)

usersbp = Blueprint(
    "Multiple Users Operations",
    "users",
    url_prefix="/api/users",
    description="Operations on multiple users",
)


@userbp.route("/<username>")
class UserController(MethodView):
    @jwt_auth_required()
    @identity_is_username()
    def get(self, username, *args, **kwargs):
        oragnization_id = get_jwt_organization()
        return json_util.dumps(user_get_by_name(username, oragnization_id))

    @jwt_auth_required()
    @require_role(Role.ADMIN)
    def delete(self, username, *args, **kwargs):
        return json_util.dumps(user_delete(username))

    @jwt_auth_required()
    @require_role(Role.ADMIN)
    def put(self, username, *args, **kwargs):
        organization_id = get_jwt_organization()
        return json_util.dumps(user_add(username, request.get_json(), organization_id))


@usersbp.route("/")
class AllUserController(MethodView):
    @jwt_auth_required()
    @require_role(Role.ADMIN)
    def get(self, *args, **kwargs):
        return json_util.dumps(user_get_all())


@usersbp.route("/<organization_id>")
class AllOrganizationUserController(MethodView):
    @jwt_auth_required()
    @require_role(Role.ADMIN)
    def get(self, organization_id, *args, **kwargs):
        return json_util.dumps(user_get_all_from_Organization(organization_id))


@userbp.route("/<username>")
class UserChangePasswordController(MethodView):
    @jwt_auth_required()
    @identity_is_username()
    def post(self, username, *args, **kwargs):
        content = request.get_json()
        return user_change_password(username, content["oldPassword"], content["newPassword"])


@userbp.route("/")
class UserResetPasswordController(MethodView):
    def post(self, *args, **kwargs):
        content = request.get_json()
        username = content["username"]
        domain = content["domain"]
        expires = current_app.config["RESET_TOKEN_EXPIRES"]
        expiry_date = datetime.now() + expires
        reset_token = secrets.token_urlsafe()

        return user_create_password_reset_request(username, domain, reset_token, expiry_date)

    def put(self, *args, **kwargs):
        content = request.get_json()
        return user_change_password_with_reset_request(content["token"], content["password"])
