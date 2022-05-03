from flask_restful import Resource

from roles.securityUtils import jwt_auth_required, identity_is_username
from users.auth import user_get_roles


class UserPermissionController(Resource):

    @staticmethod
    @jwt_auth_required()
    @identity_is_username()
    def get(username):
        user = user_get_roles(username)
        print(user)
        if user is not None:
            return {"roles": user['roles']}, 200
        else:
            return {"message": "User does not exist."}, 404
