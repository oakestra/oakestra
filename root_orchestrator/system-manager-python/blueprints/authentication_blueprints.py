from flask import request
from flask_restful import Resource
from users.auth import user_register
from users.auth import user_login, user_token_refresh
from roles.securityUtils import *
from flask_smorest import Api, abort
from blueprints.jwt_wrapper import BlueprintExt
from flask.views import MethodView

loginbp = BlueprintExt(
    'Login', 'auth', url_prefix='/api/auth'
)

login_schema = {
    "type": "object",
    "properties": {
        "username": {"type": "string"},
        "password": {"type": "string"},
    }
}
register_schema = {
    "type": "object",
    "properties": {
        "name": {"type": "string"},
        "password": {"type": "string"},
        "roles": {"type": "array", "items": {"type": "string"}},
    }
}


# ......... Functions fot the Authentication ...........#
# ......................................................#

@loginbp.route("/login")
class UserLoginController(MethodView):

    @loginbp.arguments(schema=login_schema, location="json", validate=False, unknown=True)
    def post(self,*args,**kwargs):
        content = request.get_json()
        print(content)
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
    def post(self):
        content = request.get_json()
        return user_register(content)


@loginbp.route("/refresh")
class TokenRefreshController(Resource):

    @refresh_token_required()
    def post(self):
        identity = get_jwt_auth_identity()
        token = user_token_refresh(identity)
        if token == {}:
            abort(404, {"message": "User does not exists"})
        return token
