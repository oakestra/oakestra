from db import marketplace_db
from flask.views import MethodView
from flask_smorest import Blueprint, abort
from services import marketplace_service

from .schema import MarketplaceAddonSchema, MarketplaceFilterSchema

marketplaceblp = Blueprint(
    "Marketplace Api", "marketplace_api", url_prefix="/api/v1/marketplace/addons"
)


@marketplaceblp.route("/")
class MarketplaceAddonsController(MethodView):
    @marketplaceblp.arguments(MarketplaceFilterSchema, location="query")
    @marketplaceblp.response(
        200, MarketplaceAddonSchema(many=True), content_type="application/json"
    )
    def get(self, query={}):
        return list(marketplace_db.find_approved_addons(query))

    @marketplaceblp.arguments(MarketplaceAddonSchema, location="json")
    @marketplaceblp.response(201, MarketplaceAddonSchema, content_type="application/json")
    def post(self, data):
        return marketplace_service.register_addon(data)


@marketplaceblp.route("/<marketplace_addon_id>")
class MarketPlaceAddonController(MethodView):
    @marketplaceblp.response(200, MarketplaceAddonSchema, content_type="application/json")
    def get(self, marketplace_addon_id):
        addon = marketplace_db.find_addon_by_id(marketplace_addon_id)
        if addon is None:
            abort(404, {"message": "Addon not found"})

        return addon
