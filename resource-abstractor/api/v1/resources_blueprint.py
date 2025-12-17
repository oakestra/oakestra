from bson import ObjectId
from db import candidates_db
from db.candidates_helper import build_filter
from db.jobs_db import find_job_by_id
from flask import jsonify, request
from flask.views import MethodView
from flask_smorest import Blueprint
from marshmallow import INCLUDE, Schema, fields
from services.hook_service import perform_create, perform_update, pre_post_hook
from werkzeug import exceptions

resourcesblp = Blueprint("Resources", "resources", url_prefix="/api/v1/resources")


class ResourceSchema(Schema):
    _id = fields.String()
    candidate_name = fields.String()
    candidate_location = fields.String()
    ip = fields.String()
    port = fields.String()
    active_nodes = fields.Integer()
    active = fields.Boolean()

    memory = fields.Integer()
    vcpus = fields.Integer()
    vgpus = fields.Integer()
    cpu_percent = fields.Float()
    aggregation_per_architecture = fields.Dict()
    memory_percent = fields.Float()
    gpu_percent = fields.Integer()
    virtualization = fields.List(fields.String())
    supported_addons = fields.List(fields.String())
    architecture = fields.String()
    last_modified_timestamp = fields.Float()


class ResourceFilterSchema(Schema):
    active = fields.Boolean()
    job_id = fields.String()
    candidate_name = fields.String()
    ip = fields.String()

@resourcesblp.errorhandler(422)
def handle_unprocessable_entity(err):
    details = {}
    if hasattr(err, "data") and err.data:
        details = err.data.get("messages", {})

    return {
        "message": "Invalid input",
        "details": details
    }, 422

@resourcesblp.route("/")
class AllResourcesController(MethodView):
    @resourcesblp.arguments(ResourceFilterSchema, location="query")
    def get(self, query={}):
        filter = query
        job_id = filter.get("job_id")
        if job_id:
            if ObjectId.is_valid(job_id) is False:
                raise exceptions.BadRequest()

            job = find_job_by_id(job_id)
            if job is None:
                raise exceptions.NotFound()

            candidate_id = job.get("candidate")
            if candidate_id is None:
                raise exceptions.NotFound()

            filter["candidate_id"] = candidate_id
        filter = build_filter(query)

        if request.args.get("resources"):
            print("Resources: ", request.args.get("resources"), flush=True)
            res = list(candidates_db.find_candidates(filter, request.args.get("resources")))
        else:
            res = list(candidates_db.find_candidates(filter))

        for candidate in res:
            if "_id" in candidate:
                candidate["_id"] = str(candidate["_id"])

        return jsonify(res)

    @resourcesblp.arguments(ResourceSchema(unknown=INCLUDE), location="json")
    @resourcesblp.response(201, ResourceSchema, content_type="application/json")
    @pre_post_hook("resources")
    def post(self, data, **kwargs):
        return candidates_db.create_candidate(data)

    @resourcesblp.arguments(ResourceSchema(unknown=INCLUDE), location="json")
    @resourcesblp.response(200, ResourceSchema, content_type="application/json")
    def put(self, data, **kwargs):
        resource_name = data.get("candidate_name")

        candidate = candidates_db.find_candidate_by_name(resource_name)
        if candidate:
            return perform_update(
                "resources", candidates_db.update_candidate, str(candidate["_id"]), data
            )

        return perform_create("resources", candidates_db.create_candidate, data)


@resourcesblp.route("/<resource_id>")
class ResourceController(MethodView):
    @resourcesblp.response(200, ResourceSchema, content_type="application/json")
    def get(self, resource_id):
        if ObjectId.is_valid(resource_id) is False:
            raise exceptions.BadRequest()

        candidate = candidates_db.find_candidate_by_id(resource_id)
        if candidate is None:
            raise exceptions.NotFound()

        return candidate

    @resourcesblp.arguments(ResourceSchema(unknown=INCLUDE), location="json")
    @resourcesblp.response(200, ResourceSchema, content_type="application/json")
    @pre_post_hook("resources", with_param_id="resource_id")
    def patch(self, data, **kwargs):
        resource_id = kwargs.get("resource_id")

        if not ObjectId.is_valid(resource_id):
            raise exceptions.NotFound

        return candidates_db.update_candidate_information(resource_id, data)

    @resourcesblp.response(204, ResourceSchema, content_type="application/json")
    @pre_post_hook("resources", with_param_id="resource_id")
    def delete(self, *args, **kwargs):
        resource_id = kwargs.get("resource_id")

        candidates_db.delete_candidate(resource_id)
