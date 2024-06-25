from db import addons_db
from flask.views import MethodView
from flask_smorest import Blueprint, abort
from marshmallow import Schema, fields, validate
from services import addons_service

addonsblp = Blueprint("Addons Api", "addons_api", url_prefix="/api/v1/addons")


class AddonSchema(Schema):
    _id = fields.String()
    marketplace_id = fields.String(required=True)
    status = fields.String(
        validate=validate.OneOf(
            [status.value for status in addons_service.AddonStatusEnum],
        )
    )


class AddonFilterSchema(Schema):
    status = fields.String()


@addonsblp.route("/")
class AllAddonsController(MethodView):
    @addonsblp.arguments(AddonFilterSchema, location="query")
    @addonsblp.response(200, AddonSchema(many=True), content_type="application/json")
    def get(self, query={}):
        return list(addons_db.find_addons(query))

    @addonsblp.arguments(AddonSchema, location="json")
    @addonsblp.response(201, AddonSchema, content_type="application/json")
    def post(self, addon_data):
        result = addons_service.install_addon(addon_data)

        if result is None:
            abort(400, message="Failed to install addon")

        return result

    # TODO: only used for testing purposes.
    def delete(self):
        result = addons_db.delete_all_addons()
        return {"message": f"{result.deleted_count} addons deleted."}


@addonsblp.route("/<addon_id>")
class AddonController(MethodView):
    @addonsblp.response(200, AddonSchema, content_type="application/json")
    def get(self, addon_id):
        addon = addons_db.find_addon_by_id(addon_id)
        if addon is None:
            abort(404, message="Addon not found")

        return addon

    @addonsblp.arguments(AddonSchema, location="json")
    @addonsblp.response(200, AddonSchema, content_type="application/json")
    def patch(self, addon_data):
        result = addons_db.update_addon(addon_data)

        if result is None:
            abort(400, message="Failed to update addon")

        return result
