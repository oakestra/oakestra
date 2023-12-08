from flask_smorest import Blueprint
from flask.views import MethodView
from marshmallow import Schema, fields
from bson.objectid import ObjectId
from werkzeug import exceptions

from db import *

resourcesblp = Blueprint(
    'Resource Info', 'resource info', url_prefix='/resources'
)

class ResourceSchema(Schema):
    id = fields.String(attribute='_id')
    cluster_name = fields.String()
    cluster_location = fields.String()
    ip = fields.String()
    port = fields.String()
    active_nodes = fields.Integer()

    memory_in_mb = fields.Integer()
    total_cpu_cores = fields.Integer()
    total_gpu_cores = fields.Integer()
    virtualization = fields.List(fields.String())
    last_modified_timestamp = fields.Float()

class ResourceFilterSchema(Schema):
    active = fields.Boolean()
    job_id = fields.String()

@resourcesblp.route('/')
class AllResourcesController(MethodView):

    # TODO: support pagination
    @resourcesblp.arguments(ResourceFilterSchema, location='query')
    @resourcesblp.response(200, ResourceSchema(many=True), content_type="application/json")
    def get(self, args, *kwargs):

        # TODO: need better way to handle query params
        active = args.get('active')
        job_id = args.get('job_id')
        if job_id:
            job = find_job_by_id(job_id)
            cluster_id = job.get('cluster_id')
            cluster = find_cluster_by_id(cluster_id)
            return cluster;
        if active:
            return list(find_active_clusters())
        
        return list(find_all_clusters())
    
@resourcesblp.route('/<resourceId>')
class ResourceController(MethodView):

    @resourcesblp.response(200, ResourceSchema(), content_type="application/json")
    def get(self, resourceId, args, *kwargs):
        if ObjectId.is_valid(resourceId) is False:
            raise exceptions.BadRequest()
        
        cluster = find_cluster_by_id(resourceId)
        if cluster is None:
            raise exceptions.NotFound()

        return cluster
    