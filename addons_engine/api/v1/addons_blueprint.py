from db import addons_db
from flask.views import MethodView
from flask_smorest import Blueprint, abort
from services import addons_service

from .schema import AddonFilterSchema, AddonSchema

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
        return addons_service.install_addon(addon_data)


@addonsblp.route("/<addon_id>")
class AddonController(MethodView):
    @addonsblp.response(200, AddonSchema, content_type="application/json")
    def get(self, addon_id):
        addon = addons_db.find_addon_by_id(addon_id)
        if addon is None:
            abort(404, message="Addon not found")

        return addon
