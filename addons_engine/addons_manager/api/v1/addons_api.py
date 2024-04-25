import logging

from api.v1.schema import AddonFilterSchema, AddonSchema
from db import addons_db
from flask.views import MethodView
from flask_smorest import Blueprint, abort
from flask_socketio import emit
from services import addons_runner

addonsblp = Blueprint("Addons Api", "addons_api", url_prefix="/api/v1/addons")


@addonsblp.route("/")
class AllAddonsController(MethodView):
    @addonsblp.arguments(AddonFilterSchema, location="query")
    @addonsblp.response(200, AddonSchema(many=True), content_type="application/json")
    def get(self, query={}):
        return list(addons_db.find_addons(query))

    @addonsblp.arguments(AddonSchema, location="json")
    @addonsblp.response(201, AddonSchema, content_type="application/json")
    def post(self, addon_data):
        existing_addon = addons_db.find_addon_by_marketplace_id(addon_data["marketplace_id"])
        if existing_addon:
            abort(
                400,
                message=f"Addon with marketplace_id-{addon_data['marketplace_id']} already exists",
            )

        result = addons_runner.install_addon(addon_data)

        if result is None:
            abort(400, message="Failed to install addon")


@addonsblp.route("/<addon_id>")
class AddonController(MethodView):
    @addonsblp.response(200, AddonSchema, content_type="application/json")
    def get(self, addon_id):
        addon = addons_db.find_addon_by_id(addon_id)
        if addon is None:
            abort(404, message="Addon not found")

        return addon


def init_addons_socket(socketio, addons_manager_id):
    @socketio.event()
    def get_manager_id():
        emit("receive_manager_id", addons_manager_id)

    @socketio.event()
    def disable_addon(addon_id):
        logging.info(f"Disabling Addon-{addon_id}...")

        addon = addons_db.find_addon_by_id(addon_id)
        if not addon:
            logging.error(f"Addon-{addon_id} not found")
            return

        addons_runner.stop_addon(addon)
        addons_db.update_addon(addon_id, {"status": "disabled"})

    @socketio.event()
    def enable_addon(addon_id):
        logging.info(f"Enabling Addon-{addon_id}...")

        addon = addons_db.find_addon_by_id(addon_id)
        if addon is None:
            logging.error(f"Addon {addon_id} not found")
            return

        addons_runner.run_addon(addon)

    # TODO: complete implementation
    @socketio.event()
    def report_failure(addon_id, containers=[]):
        logging.info(f"Addon failing {addon_id}: {containers}")
