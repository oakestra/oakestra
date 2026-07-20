from flask.views import MethodView
from flask_smorest import Blueprint, abort
from oakestra_logging import get_logger
from roles.securityUtils import get_jwt_organization, identity_is_username, jwt_auth_required
from users.auth import user_get_roles

from blueprints.schema_wrapper import SchemaWrapper

logger = get_logger(__name__)

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
        logger.debug(
            "Resolved user permissions",
            event_name="authorization.roles.resolved",
            role_count=len(user.get("roles", [])) if user else 0,
        )
        if user is not None:
            return {"roles": user["roles"]}
        else:
            return abort(404, {"message": "User does not exist."})
