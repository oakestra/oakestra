from bson import json_util
from db import apps_db
from flask import request
from flask.views import MethodView
from flask_restful import Resource
from flask_smorest import Blueprint
from marshmallow import INCLUDE, Schema, fields

applicationblp = Blueprint(
    "Application operations",
    "applications",
    url_prefix="/api/v1/application",
    description="Operations on single application",
)


class ApplicationSchema(Schema):
    _id = fields.String()


@applicationblp.route("/")
class ApplicationsController(Resource):
    @applicationblp.response(
        200, ApplicationSchema(unknown=INCLUDE), content_type="application/json"
    )
    def get(self, *args, **kwargs):
        return apps_db.find_all_apps()


@applicationblp.route("/<userid>")
class CreateApplicationController(Resource):
    @applicationblp.response(
        200, ApplicationSchema(unknown=INCLUDE), content_type="application/json"
    )
    def get(self, userid, *args, **kwargs):
        return apps_db.find_user_apps(userid)

    @applicationblp.response(
        200, ApplicationSchema(unknown=INCLUDE), content_type="application/json"
    )
    def post(self, userid, *args, **kwargs):
        data = request.get_json()
        return apps_db.create_app(userid, data)


@applicationblp.route("/<userid>/<appid>")
class ApplicationController(MethodView):
    @applicationblp.response(
        200, ApplicationSchema(unknown=INCLUDE), content_type="application/json"
    )
    def get(self, userid, appid, *args, **kwargs):
        return apps_db.find_user_app(userid, appid)

    @applicationblp.response(200, content_type="application/json")
    def delete(self, userid, appid, *args, **kwargs):
        return json_util.dumps(apps_db.delete_app(userid, appid))

    @applicationblp.response(
        200, ApplicationSchema(unknown=INCLUDE), content_type="application/json"
    )
    def put(self, userid, appid, *args, **kwargs):
        data = request.get_json()
        return apps_db.update_app(userid, appid, data)

    @applicationblp.response(
        200, ApplicationSchema(unknown=INCLUDE), content_type="application/json"
    )
    def patch(self, userid, appid, *args, **kwargs):
        data = request.get_json()
        return apps_db.update_app(userid, appid, data)
