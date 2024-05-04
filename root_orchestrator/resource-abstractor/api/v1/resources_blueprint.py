from bson import ObjectId
from db import clusters_db
from db.clusters_helper import build_filter
from db.jobs_db import find_job_by_id
from flask.views import MethodView
from flask_smorest import Blueprint
from marshmallow import INCLUDE, Schema, fields
from services.hook_service import before_after_hook, perform_create, perform_update
from werkzeug import exceptions

resourcesblp = Blueprint("Resources", "resources", url_prefix="/api/v1/resources")


class ResourceSchema(Schema):
    _id = fields.String()
    cluster_name = fields.String()
    cluster_location = fields.String()
    ip = fields.String()
    port = fields.String()
    active_nodes = fields.Integer()
    active = fields.Boolean()

    memory_in_mb = fields.Integer()
    total_cpu_cores = fields.Integer()
    total_gpu_cores = fields.Integer()
    aggregated_cpu_percent = fields.Float()
    aggregation_per_architecture = fields.Dict()
    available_memory = fields.Float()
    total_gpu_percent = fields.Integer()
    virtualization = fields.List(fields.String())
    last_modified_timestamp = fields.Float()


class ResourceFilterSchema(Schema):
    active = fields.Boolean()
    job_id = fields.String()
    cluster_name = fields.String()
    ip = fields.String()


@resourcesblp.route("/")
class AllResourcesController(MethodView):
    @resourcesblp.arguments(ResourceFilterSchema, location="query")
    @resourcesblp.response(200, ResourceSchema(many=True), content_type="application/json")
    def get(self, query={}):
        filter = query
        job_id = filter.get("job_id")
        if job_id:
            if ObjectId.is_valid(job_id) is False:
                raise exceptions.BadRequest()

            job = find_job_by_id(job_id)
            if job is None:
                raise exceptions.NotFound()

            cluster_id = job.get("cluster")
            if cluster_id is None:
                raise exceptions.NotFound()

            filter["cluster_id"] = cluster_id
        filter = build_filter(query)

        return list(clusters_db.find_clusters(filter))

    @resourcesblp.arguments(ResourceSchema(unknown=INCLUDE), location="json")
    @resourcesblp.response(200, ResourceSchema, content_type="application/json")
    def put(self, data, **kwargs):
        resource_name = data.get("cluster_name")

        cluster = clusters_db.find_cluster_by_name(resource_name)
        if cluster:
            return perform_update("resource", clusters_db.update_cluster, str(cluster["_id"]), data)

        return perform_create("resource", clusters_db.create_cluster, data)


@resourcesblp.route("/<resource_id>")
class ResourceController(MethodView):
    @resourcesblp.response(200, ResourceSchema, content_type="application/json")
    def get(self, resource_id):
        if ObjectId.is_valid(resource_id) is False:
            raise exceptions.BadRequest()

        cluster = clusters_db.find_cluster_by_id(resource_id)
        if cluster is None:
            raise exceptions.NotFound()

        return cluster

    @resourcesblp.arguments(ResourceSchema(unknown=INCLUDE), location="json")
    @resourcesblp.response(200, ResourceSchema, content_type="application/json")
    @before_after_hook("resources", with_param_id="resource_id")
    def patch(self, data, **kwargs):
        resource_id = kwargs.get("resource_id")

        if ObjectId.is_valid(resource_id) is False:
            raise exceptions.BadRequest()

        return clusters_db.update_cluster_information(resource_id, data)
