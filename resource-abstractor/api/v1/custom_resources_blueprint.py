import json

import jsonschema
from db import custom_resources_db
from flask import request, jsonify
from flask.views import MethodView
from flask_smorest import Blueprint, abort
from marshmallow import INCLUDE, Schema, fields
from services.hook_service import pre_post_hook

customblp = Blueprint("Custom Resources", "custom_resources", url_prefix="/api/v1/custom-resources")


class CustomResourceSchema(Schema):
    _id = fields.String()
    resource_type = fields.String(required=True)
    schema = fields.Dict()


@customblp.route("/")
class CustomResourceController(MethodView):
    @customblp.response(200, CustomResourceSchema(many=True), content_type="application/json")
    def get(self):
        return list(custom_resources_db.find_custom_resources())

    @customblp.arguments(CustomResourceSchema, location="json")
    @customblp.response(201, CustomResourceSchema, content_type="application/json")
    def post(self, data, *args, **kwargs):
        return custom_resources_db.create_custom_resource(data)


@customblp.route("/<resource_type>")
class CustomResourceDefinitionController(MethodView):
    def delete(self, *args, **kwargs):
        """
        Delete a custom resource definition and all its instances.
        This is a cascading delete operation.
        """
        resource_type = kwargs.get("resource_type")

        meta_data = custom_resources_db.find_custom_resource_by_type(resource_type)
        if meta_data is None:
            abort(404, message="Custom Resource definition not found")

        # Delete all instances of this resource type
        custom_resources_db.delete_all_resources(resource_type)

        # Delete the resource definition
        custom_resources_db.delete_custom_resource_by_type(resource_type)

        return jsonify({"message": f"Resource type '{resource_type}' and all its instances deleted"})


@customblp.route("/<resource>")
class ResourcesController(MethodView):
    def get(self, *args, **kwargs):
        """
        Get all resources of a specific type with optional filtering.

        Query parameters are passed as MongoDB filter criteria:
        - Simple equality: ?field=value
        - Multiple conditions: ?field1=value1&field2=value2
        - Nested fields: ?parent.child=value

        Example: GET /api/v1/custom-resources/database?status=active&region=us-east
        """
        resource_type = kwargs.get("resource")
        filter = request.args.to_dict()

        meta_data = custom_resources_db.find_custom_resource_by_type(resource_type)
        if meta_data is None:
            abort(404, message="Custom Resource not registered")

        result = list(custom_resources_db.find_resources(resource_type, filter))

        return jsonify(result)

    @customblp.arguments(Schema(unknown=INCLUDE), location="json")
    @pre_post_hook()
    def post(self, data, *args, **kwargs):
        resource_type = kwargs.get("resource")

        meta_data = custom_resources_db.find_custom_resource_by_type(resource_type)
        if meta_data is None:
            abort(404, message="Custom Resource not registered")

        try:
            jsonschema.validate(data, meta_data.get("schema", {}))
        except jsonschema.ValidationError as e:
            abort(400, message=e.message)

        result = custom_resources_db.create_resource(resource_type, data)

        return jsonify(result)


@customblp.route("/<resource>/<resource_id>")
class ResourceController(MethodView):
    def get(self, *args, **kwargs):
        resource_type = kwargs.get("resource")
        resource_id = kwargs.get("resource_id")

        meta_data = custom_resources_db.find_custom_resource_by_type(resource_type)
        if meta_data is None:
            abort(404, message="Custom Resource not registered")

        result = custom_resources_db.find_resource_by_id(resource_type, resource_id)

        return jsonify(result)

    @customblp.arguments(Schema(unknown=INCLUDE), location="json")
    @pre_post_hook(with_param_id="resource_id")
    def patch(self, data, *args, **kwargs):
        resource_type = kwargs.get("resource")
        resource_id = kwargs.get("resource_id")

        meta_data = custom_resources_db.find_custom_resource_by_type(resource_type)
        if meta_data is None:
            abort(404, message="Custom Resource not found")

        try:
            jsonschema.validate(data, meta_data.get("schema", {}))
        except jsonschema.ValidationError as e:
            abort(400, message=e.message)

        result = custom_resources_db.update_resource(resource_type, resource_id, data)

        return jsonify(result)

    @pre_post_hook(with_param_id="resource_id")
    def delete(self, *args, **kwargs):
        resource_type = kwargs.get("resource")
        resource_id = kwargs.get("resource_id")

        meta_data = custom_resources_db.find_custom_resource_by_type(resource_type)
        if meta_data is None:
            abort(404, message="Custom Resource not found")

        custom_resources_db.delete_resource(resource_type, resource_id)

        return jsonify({"_id": resource_id})
