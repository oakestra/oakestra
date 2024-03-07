import json

from db import apps_db
from flask import request
from flask.views import MethodView
from flask_restful import Resource
from flask_smorest import Blueprint

applicationblp = Blueprint(
    "Application operations",
    "applications",
    url_prefix="/api/v1/application",
    description="Operations on single application",
)


@applicationblp.route("/")
class ApplicationsController(Resource):
    def get(self, *args, **kwargs):
        return json.dumps(list(apps_db.find_all_apps(), default=str))


@applicationblp.route("/<userid>")
class CreateApplicationController(Resource):
    def get(self, userid, *args, **kwargs):
        return json.dumps(list(apps_db.find_user_apps(userid)), default=str)

    def post(self, userid, *args, **kwargs):
        data = request.get_json()
        return json.dump(apps_db.create_app(userid, data), default=str)


@applicationblp.route("/<userid>/<appid>")
class ApplicationController(MethodView):
    def get(self, userid, appid, *args, **kwargs):
        return json.dump(apps_db.find_user_app(userid, appid), default=str)

    def delete(self, userid, appid, *args, **kwargs):
        return json.dump(apps_db.delete_app(userid, appid), default=str)

    def put(self, userid, appid, *args, **kwargs):
        data = request.get_json()
        return json.dump(apps_db.update_app(userid, appid, data), default=str)

    def patch(self, userid, appid, *args, **kwargs):
        data = request.get_json()
        return json.dump(apps_db.update_app(userid, appid, data), default=str)
