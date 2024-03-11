import json

from db import apps_db
from flask import request
from flask.views import MethodView
from flask_smorest import Blueprint
from marshmallow import Schema, fields

applicationblp = Blueprint(
    "Application operations",
    "applications",
    url_prefix="/api/v1/application",
    description="Operations on single application",
)


class ApplicationFilterSchema(Schema):
    application_name = fields.String()
    application_namespace = fields.String()
    userId = fields.String()


@applicationblp.route("/")
class ApplicationsController(MethodView):
    @applicationblp.arguments(ApplicationFilterSchema, location="query")
    def get(self, query={}):
        return json.dumps(list(apps_db.find_apps(query)), default=str)

    def post(self, *args, **kwargs):
        data = request.get_json()
        return json.dump(apps_db.create_app(data), default=str)


@applicationblp.route("/<appid>")
class ApplicationController(MethodView):
    @applicationblp.arguments(ApplicationFilterSchema, location="query")
    def get(self, appid, query={}):
        return json.dump(apps_db.find_app_by_id(appid), default=str)

    def delete(self, appid, *args, **kwargs):
        return json.dump(apps_db.delete_app(appid), default=str)

    def patch(self, appid, *args, **kwargs):
        data = request.get_json()
        return json.dump(apps_db.update_app(appid, data), default=str)
