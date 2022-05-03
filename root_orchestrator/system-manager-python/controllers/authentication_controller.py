from flask import request
from flask_restful import Resource
from users.auth import user_register
from users.auth import user_login, user_token_refresh
from roles.securityUtils import *


# ......... Functions fot the Authentication ...........#
# ......................................................#

class UserLoginController(Resource):

    @staticmethod
    def post():
        content = request.get_json()
        print(content)
        if content is None:
            return {"message": "No credentials provided"}, 403
        resp = user_login(content)
        if resp == {}:
            return {"message": "invalid username or password"}, 401
        return user_login(content)


class UserRegisterController(Resource):

    @staticmethod
    @jwt_required()
    @require_role(Role.ADMIN)
    def post():
        content = request.get_json()
        return user_register(content)


class TokenRefreshController(Resource):

    @staticmethod
    @refresh_token_required()
    def post():
        identity = get_jwt_auth_identity()
        token = user_token_refresh(identity)
        if token == {}:
            return {"message": "User does not exists"}, 404
        return token
