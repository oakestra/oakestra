# ........ Functions for organization management ...............#
# ......................................................#
import logging

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

from blueprints.schema_wrapper import SchemaWrapper

logger = logging.getLogger("system_manager")


organizationblp = Blueprint(
    "Organization operations",
    "organizations",
    url_prefix="/api/organization",
    description="Operations on single organization",
)

organization_schema = {
    "type": "object",
    "properties": {
        "_id": {"type": "string"},
        "name": {"type": "string"},
        "member": {"type": "array", "items": {"type": "object"}},
    },
}


@organizationblp.route("/<organizationid>")
class OrganizationControllerDelete(MethodView):
    @jwt_required()
    @require_role(Role.ADMIN)
    def delete(self, organizationid, *args, **kwargs):
        if organizationid == "undefined" or not organizationid:
            abort(400, {"message": "Invalid organization ID provided."})
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
        if organizationid == "undefined" or not organizationid:
            abort(400, {"message": "Invalid organization ID provided."})
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
            return get_all_organizations()
        except Exception as e:
            logger.error(e)
            return abort(404, {"message": e})

    @organizationblp.response(
        200, SchemaWrapper(organization_schema), content_type="application/json"
    )
    @jwt_required()
    @require_role(Role.ADMIN)
    def post(self, *args, **kwargs):
        try:
            new_org = request.get_json()
            new_id = add_organization(new_org)
            new_org["_id"] = new_id
            
            response_org = {
                "_id": new_id,
                "name": new_org.get("name"),
                "member": new_org.get("member", [])
            }
            logger.info(f"Organization created with ID: {new_id}")
            return response_org 
        except ConnectionError as e:
            abort(404, {"message": e})