import logging

from bson import json_util
from flask import request
from flask.views import MethodView
from flask_jwt_extended import jwt_required
from flask_restful import Resource
from flask_smorest import abort
from ext_requests.token_db import create_cluster_token
from roles.securityUtils import (
    Role,
    create_jwt_auth_access_token,
    get_jwt_auth_identity,
    get_jwt_organization,
    refresh_token_required,
    require_role,
)
from users.auth import user_login, user_register, user_token_refresh

from blueprints.jwt_wrapper import BlueprintExt

logger = logging.getLogger("system_manager")


loginbp = BlueprintExt("Login", "auth", url_prefix="/api/auth")
tokenbp = BlueprintExt("Register", "register", url_prefix="/api/token")

login_schema = {
    "type": "object",
    "properties": {
        "username": {"type": "string"},
        "password": {"type": "string"},
        "organization": {"type": "string"},
    },
}
register_schema = {
    "type": "object",
    "properties": {
        "name": {"type": "string"},
        "password": {"type": "string"},
        "roles": {"type": "array", "items": {"type": "string"}},
    },
}

cluster_token_schema = {
    "type": "object",
    "properties": {
        "cluster_name": {"type": "string"},
    },
    "required": ["cluster_name"],
}


# ......................................................#


@loginbp.route("/login")
class UserLoginController(MethodView):
    @loginbp.arguments(schema=login_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        content = request.get_json()
        logger.debug(content)
        if content is None:
            abort(403, {"message": "No credentials provided"})
        resp = user_login(content)
        if resp == {}:
            abort(401, {"message": "invalid username or password"})
        return resp


@loginbp.route("/register")
class UserRegisterController(Resource):
    @loginbp.arguments(schema=register_schema, location="json", validate=False, unknown=True)
    @jwt_required()
    @require_role(Role.ADMIN)
    def post(self, *args, **kwargs):
        content = request.get_json()
        organization_id = get_jwt_organization()
        return json_util.dumps(user_register(content, organization_id))


@loginbp.route("/refresh")
class TokenRefreshController(Resource):
    @refresh_token_required()
    def post(self):
        identity = get_jwt_auth_identity()
        token = user_token_refresh(identity)
        if token == {}:
            abort(404, {"message": "User does not exists"})
        return token


@tokenbp.route("/cluster")
class ClusterTokenController(Resource):
    @tokenbp.arguments(schema=cluster_token_schema, location="json", validate=False, unknown=True)
    @jwt_required()
    @require_role(Role.ADMIN)
    def post(self, *args, **kwargs):
        content = request.get_json() or {}
        cluster_name = content.get("cluster_name")

        if not cluster_name:
            abort(400, {"message": "cluster_name is required"})

        token = create_jwt_auth_access_token(
            identity=cluster_name,
            additional_claims={
                "cluster_name": cluster_name,
                "token_type": "cluster_bootstrap",
            },
        )

        token_doc = create_cluster_token(cluster_name, token)
        if not token_doc:
            abort(500, {"message": "Failed to save cluster token"})

        return {"token": token}
