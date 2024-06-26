import json

from db import addons_db
from flask.views import MethodView
from flask_smorest import Blueprint, abort
from marshmallow import INCLUDE, Schema, fields, validate
from services import addons_service

addonsblp = Blueprint("Addons Api", "addons_api", url_prefix="/api/v1/addons")


# TODO add get schema
class AddonPOSTSchema(Schema):
    marketplace_id = fields.String(required=True)
    status = fields.String(
        validate=validate.OneOf(
            [str(status) for status in addons_service.AddonStatusEnum],
        ),
        required=True,
    )


class AddonPATCHSchema(Schema):
    status = fields.String()


class AddonFilterSchema(Schema):
    status = fields.String()


@addonsblp.route("/")
class AllAddonsController(MethodView):
    @addonsblp.arguments(AddonFilterSchema, location="query")
    def get(self, query={}):
        return json.dumps(list(addons_db.find_addons(query)), default=str), 200

    @addonsblp.arguments(AddonPOSTSchema, location="json")
    def post(self, addon_data):
        result = addons_service.install_addon(addon_data)

        if result is None:
            abort(400, message="Failed to install addon")

        return json.dupms(result, default=str), 201

    # TODO: only used for testing purposes.
    def delete(self):
        result = addons_db.delete_all_addons()
        return {"message": f"{result.deleted_count} addons deleted."}

    # TODO: only used for testing purposes.
    def delete(self):
        addons_service.stop_all_addons()
        result = addons_db.delete_all_addons()
        return {"message": f"{result.deleted_count} addons deleted."}


@addonsblp.route("/<addon_id>")
class AddonController(MethodView):
    def get(self, addon_id):
        addon = addons_db.find_addon_by_id(addon_id)
        if addon is None:
            abort(404, message="Addon not found")

        return json.dumps(addon, default=str), 200

    @addonsblp.arguments(AddonPATCHSchema(unknown=INCLUDE), location="json")
    def patch(self, addon_data, *args, **kwargs):
        addon_id = kwargs.get("addon_id")
        result = addons_db.update_addon(addon_id, addon_data)

        if result is None:
            abort(400, message="Failed to update addon")

        return json.dumps(result, default=str), 200
