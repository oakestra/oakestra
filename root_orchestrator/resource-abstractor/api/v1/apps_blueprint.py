import json

from db import jobs_db as apps_db
from flask.views import MethodView
from flask_smorest import Blueprint
from marshmallow import Schema, fields
from services.hook_service import before_after_hook

applicationsblp = Blueprint(
    "Applications operations",
    "applications",
    url_prefix="/api/v1/applications",
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

    @before_after_hook("applications")
    def post(self, data, *args, **kwargs):
        result = apps_db.create_app(data)

        return json.dumps(result, default=str)


@applicationsblp.route("/<app_id>")
class ApplicationController(MethodView):
    @applicationsblp.arguments(ApplicationFilterSchema, location="query")
    def get(self, query, *args, **kwargs):
        app_id = kwargs.get("app_id")

        return json.dumps(apps_db.find_app_by_id(app_id, query), default=str)

    @before_after_hook("applications", with_param_id="app_id")
    def delete(self, *args, **kwargs):
        app_id = kwargs.get("app_id")
        result = apps_db.delete_app(app_id)

        return json.dumps(result, default=str)

    @before_after_hook("applications", with_param_id="app_id")
    def patch(self, data, *args, **kwargs):
        app_id = kwargs.get("app_id")
        result = apps_db.update_app(app_id, data)

        return json.dumps(result, default=str)
