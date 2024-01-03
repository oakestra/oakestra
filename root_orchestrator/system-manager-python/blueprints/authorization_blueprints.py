from blueprints.schema_wrapper import SchemaWrapper
from flask.views import MethodView
from flask_smorest import Blueprint, abort
from roles.securityUtils import get_jwt_organization, identity_is_username, jwt_auth_required
from users.auth import user_get_roles

permissionbp = Blueprint("Permissions", "permissions", url_prefix="/api/permission")

auth_schema = {
    "type": "object",
    "properties": {
        "roles": {"type": "array", "items": {"type": "string"}},
    },
}


@permissionbp.route("/<username>")
class UserPermissionController(MethodView):
    @permissionbp.response(200, schema=SchemaWrapper(auth_schema), content_type="application/json")
    @jwt_auth_required()
    @identity_is_username()
    def get(self, username):
        organization_id = get_jwt_organization()
        user = user_get_roles(username, organization_id)
        print(user)
        if user is not None:
            return {"roles": user["roles"]}
        else:
            return abort(404, {"message": "User does not exist."})
