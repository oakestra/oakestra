import logging

from addon_status import AddonStatusEnum
from db import addons_db
from flask import jsonify
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
            [str(status) for status in AddonStatusEnum],
        ),
        required=True,
    )


class AddonFilterSchema(Schema):
    status = fields.String()


@addonsblp.route("/")
class AllAddonsController(MethodView):
    @addonsblp.arguments(AddonFilterSchema, location="query")
    def get(self, query={}):
        return jsonify(list(addons_db.find_addons(query))), 200

    @addonsblp.arguments(AddonPOSTSchema, location="json")
    def post(self, addon_data):
        marketplace_id = addon_data.get("marketplace_id")
        exisiting_addon = addons_db.find_addon_by_marketplace_id(marketplace_id)
        if exisiting_addon:
            abort(400, message=f"Addon with marketplace-id - {marketplace_id} already exists.")

        try:
            result = addons_service.install_addon(addon_data)
        except addons_service.MarketplaceAddonNotFoundError:
            abort(404, message="Marketplace addon not found")
        except addons_service.MarketplaceUnavailableError as e:
            logging.warning(f"Marketplace lookup for {marketplace_id} failed: {e}")
            abort(502, message="Marketplace is unavailable")

        if result is None:
            abort(400, message="Failed to install addon")

        return jsonify(result), 201


@addonsblp.route("/<addon_id>")
class AddonController(MethodView):
    def get(self, addon_id):
        addon = addons_db.find_addon_by_id(addon_id)
        if addon is None:
            abort(404, message="Addon not found")

        return jsonify(addon), 200

    @addonsblp.arguments(AddonPATCHSchema(unknown=INCLUDE), location="json")
    def patch(self, addon_data, *args, **kwargs):
        addon_id = kwargs.get("addon_id")
        result = addons_db.update_addon(addon_id, addon_data)

        if result is None:
            abort(404, message="Addon not found")

        return jsonify(result), 200

    def delete(self, addon_id, *args, **kwargs):
        addon = addons_db.find_addon_by_id(addon_id)

        if addon is None:
            abort(404, message="Addon not found")

        if addon.get("status") != str(AddonStatusEnum.DISABLED):
            result = addons_db.update_addon(addon_id, {"status": str(AddonStatusEnum.DISABLING)})

            if result is None:
                abort(400, message="Failed to disable addon")

            return jsonify(result), 200
        addons_db.delete_addon_by_id(addon_id)
        return "", 204
