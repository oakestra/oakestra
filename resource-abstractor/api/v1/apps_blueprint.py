from db import jobs_db as apps_db
from flask import jsonify
from flask.views import MethodView
from flask_smorest import Blueprint
from marshmallow import Schema, fields
from services.hook_service import pre_post_hook

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
        return jsonify(list(apps_db.find_apps(query)))

    @pre_post_hook("applications")
    def post(self, data, *args, **kwargs):
        result = apps_db.create_app(data)

        return jsonify(result)


@applicationsblp.route("/<app_id>")
class ApplicationController(MethodView):
    @applicationsblp.arguments(ApplicationFilterSchema, location="query")
    def get(self, query, *args, **kwargs):
        app_id = kwargs.get("app_id")

        return jsonify(apps_db.find_app_by_id(app_id, query))

    @pre_post_hook("applications", with_param_id="app_id")
    def delete(self, *args, **kwargs):
        app_id = kwargs.get("app_id")
        result = apps_db.delete_app(app_id)

        return jsonify(result)

    @pre_post_hook("applications", with_param_id="app_id")
    def patch(self, data, *args, **kwargs):
        app_id = kwargs.get("app_id")
        result = apps_db.update_app(app_id, data)

        return jsonify(result)
