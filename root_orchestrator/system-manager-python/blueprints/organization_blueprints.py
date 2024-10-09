# ........ Functions for organization management ...............#
# ......................................................#
from blueprints.schema_wrapper import SchemaWrapper
from bson import json_util
from flask import request
from flask.views import MethodView
from flask_jwt_extended import jwt_required
from flask_smorest import Blueprint, abort
from organizations.organization_management import (
    add_organization,
    delete_organization,
    get_all_organizations,
    update_organization,
)
from roles.securityUtils import Role, require_role

organizationblp = Blueprint(
    "Organization operations",
    "organizations",
    url_prefix="/api/organization",
    description="Operations on single organization",
)

organization_schema = {
    "type": "object",
    "properties": {
        "name": {"type": "string"},
        "member": {"type": "array", "items": {"type": "string"}},
    },
}


@organizationblp.route("/<organizationid>")
class OrganizationControllerDelete(MethodView):
    @jwt_required()
    @require_role(Role.ADMIN)
    def delete(self, organizationid, *args, **kwargs):
        try:
            res = delete_organization(organizationid)
            if res:
                return {"message": "Organization deleted"}
            else:
                abort(501, {"message": "Organization could not be deleted"})
        except ConnectionError as e:
            abort(404, {"message": e})

    @jwt_required()
    @require_role(Role.ADMIN)
    def put(self, organizationid, *args, **kwargs):
        try:
            update_organization(organizationid, request.get_json())
            return {"message": "Organization is updated"}
        except ConnectionError as e:
            abort(404, {"message": e})


@organizationblp.route("/")
class OrganizationController(MethodView):
    @organizationblp.response(
        200, SchemaWrapper(organization_schema), content_type="application/json"
    )
    @jwt_required()
    @require_role(Role.ADMIN)
    def get(self, *args, **kwargs):
        try:
            return json_util.dumps(get_all_organizations())
        except Exception as e:
            print(e)
            return abort(404, {"message": e})

    @organizationblp.response(
        200, SchemaWrapper(organization_schema), content_type="application/json"
    )
    @jwt_required()
    @require_role(Role.ADMIN)
    def post(self, *args, **kwargs):
        try:
            return json_util.dumps(add_organization(request.get_json()))
        except ConnectionError as e:
            abort(404, {"message": e})
