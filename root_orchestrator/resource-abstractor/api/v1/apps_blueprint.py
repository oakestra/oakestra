import json

from db import apps_db
from flask import request
from flask.views import MethodView
from flask_smorest import Blueprint
from marshmallow import Schema, fields

applicationsblp = Blueprint(
    "Applications operations",
    "applications",
    url_prefix="/api/v1/applications",
    description="Operations on applications",
)


class ApplicationFilterSchema(Schema):
    application_name = fields.String()
    application_namespace = fields.String()
    userId = fields.String()


@applicationsblp.route("/")
class ApplicationsController(MethodView):
    @applicationsblp.arguments(ApplicationFilterSchema, location="query")
    def get(self, query={}):
        return json.dumps(list(apps_db.find_apps(query)), default=str)

    def post(self, *args, **kwargs):
        data = request.get_json()
        return json.dumps(apps_db.create_app(data), default=str)


@applicationsblp.route("/<appId>")
class ApplicationController(MethodView):
    @applicationsblp.arguments(ApplicationFilterSchema, location="query")
    def get(self, query, **kwargs):
        app_id = kwargs.get("appId")
        return json.dumps(apps_db.find_app_by_id(app_id, query), default=str)

    def delete(self, appId, *args, **kwargs):
        return json.dumps(apps_db.delete_app(appId), default=str)

    def patch(self, appId, *args, **kwargs):
        data = request.get_json()
        return json.dumps(apps_db.update_app(appId, data), default=str)
