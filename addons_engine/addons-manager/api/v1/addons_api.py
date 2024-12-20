import json

from db import addons_db
from flask.views import MethodView
from flask_smorest import Blueprint, abort
from marshmallow import INCLUDE, Schema, fields, validate
from services import addons_service

addonsblp = Blueprint("Addons Api", "addons_api", url_prefix="/api/v1/addons")


class AddonPOSTSchema(Schema):
    marketplace_id = fields.String(required=True)


class AddonPATCHSchema(Schema):
    status = fields.String(
        validate=validate.OneOf(
            [str(status) for status in addons_service.AddonStatusEnum],
        ),
        required=True,
    )


class AddonFilterSchema(Schema):
    status = fields.String()


@addonsblp.route("/")
class AllAddonsController(MethodView):
    @addonsblp.arguments(AddonFilterSchema, location="query")
    def get(self, query={}):
        return json.dumps(list(addons_db.find_addons(query)), default=str), 200

    @addonsblp.arguments(AddonPOSTSchema, location="json")
    def post(self, addon_data):
        marketplace_id = addon_data.get("marketplace_id")
        exisiting_addon = addons_db.find_addon_by_marketplace_id(marketplace_id)
        if exisiting_addon:
            abort(400, message=f"Addon with marketplace-id - {marketplace_id} already exists.")

        result = addons_service.install_addon(addon_data)

        if result is None:
            abort(400, message="Failed to install addon")

        return json.dumps(result, default=str), 201


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

    def delete(self, addon_id, *args, **kwargs):
        result = addons_db.update_addon(
            addon_id, {"status": str(addons_service.AddonStatusEnum.DISABLING)}
        )

        if result is None:
            abort(400, message="Failed to disable addon")

        return json.dumps(result, default=str), 200
